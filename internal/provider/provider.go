package provider

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/responses"
	"github.com/openai/openai-go/shared"
)

// CallRequest bundles parameters for a provider API call.
type CallRequest struct {
	Model        string
	SystemPrompt string
	UserPrompt   string
	Effort       string
}

// Provider defines the contract for API providers.
type Provider interface {
	Name() string
	Model() string
	APIKey() string
	Call(ctx context.Context, req CallRequest) (string, error)
	StreamCall(ctx context.Context, req CallRequest, w io.Writer) error
}

// CompletionError reports a provider terminal state that did not complete successfully.
type CompletionError struct {
	provider        string
	reason          string
	maxOutputTokens int
	partial         bool
}

func (e *CompletionError) Error() string {
	output := "output is incomplete"
	if e.partial {
		output = "partial output may already have been emitted"
	}
	if isTokenLimitReason(e.reason) {
		if !e.partial {
			output = "output is truncated"
		}
		if e.maxOutputTokens > 0 {
			return fmt.Sprintf("%s: generation reached the configured max output token limit (%d); %s; increase max_output_tokens or shorten the requested output", e.provider, e.maxOutputTokens, output)
		}
		return fmt.Sprintf("%s: generation reached its output token limit; %s; increase max_output_tokens or shorten the requested output", e.provider, output)
	}
	return fmt.Sprintf("%s: generation ended without successful completion (reason=%s); %s", e.provider, e.reason, output)
}

func newCompletionError(providerName, reason string, maxOutputTokens int, partial bool) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "missing_terminal_status"
	}
	return &CompletionError{
		provider:        providerName,
		reason:          reason,
		maxOutputTokens: maxOutputTokens,
		partial:         partial,
	}
}

func isTokenLimitReason(reason string) bool {
	switch strings.ToLower(strings.TrimSpace(reason)) {
	case "length", "max_tokens", "max_output_tokens":
		return true
	default:
		return false
	}
}

// -----------------------------------------------------------------------------
// OpenAI Provider (Responses API)
// -----------------------------------------------------------------------------

type openAIProvider struct {
	apiKey          string
	model           string
	baseURL         string
	maxOutputTokens int
	client          *openai.Client
}

// NewOpenAI creates a Provider backed by the OpenAI Responses API.
// baseURL is optional; when empty the default api.openai.com endpoint is used.
func NewOpenAI(apiKey, model, baseURL string, maxRetries, maxOutputTokens int) Provider {
	opts := []option.RequestOption{
		option.WithAPIKey(apiKey),
		option.WithMaxRetries(maxRetries),
	}
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}
	c := openai.NewClient(opts...)
	return &openAIProvider{apiKey: apiKey, model: model, baseURL: baseURL, maxOutputTokens: maxOutputTokens, client: &c}
}

func (p *openAIProvider) Name() string   { return "openai" }
func (p *openAIProvider) Model() string  { return p.model }
func (p *openAIProvider) APIKey() string { return p.apiKey }

func (p *openAIProvider) buildParams(req CallRequest) responses.ResponseNewParams {
	params := responses.ResponseNewParams{
		Model:           shared.ResponsesModel(req.Model),
		Instructions:    openai.String(req.SystemPrompt),
		Input:           responses.ResponseNewParamsInputUnion{OfString: openai.String(req.UserPrompt)},
		MaxOutputTokens: openai.Int(int64(p.maxOutputTokens)),
	}
	if supportsOpenAIReasoning(req.Model) {
		params.Reasoning = shared.ReasoningParam{Effort: mapEffort(req.Effort)}
	}
	return params
}

func supportsOpenAIReasoning(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	if model == "gpt-5" || strings.HasPrefix(model, "gpt-5-") || strings.HasPrefix(model, "gpt-5.") {
		return true
	}
	for _, family := range []string{"o1", "o3", "o4"} {
		if model == family || strings.HasPrefix(model, family+"-") {
			return true
		}
	}
	return false
}

func (p *openAIProvider) Call(ctx context.Context, req CallRequest) (string, error) {
	resp, err := p.client.Responses.New(ctx, p.buildParams(req))
	if err != nil {
		return "", fmt.Errorf("openai: %w", err)
	}
	if resp.Status != responses.ResponseStatusCompleted {
		reason := resp.IncompleteDetails.Reason
		if reason == "" {
			reason = string(resp.Status)
		}
		return "", newCompletionError(p.Name(), reason, p.maxOutputTokens, false)
	}
	output := resp.OutputText()
	if strings.TrimSpace(output) == "" {
		return "", fmt.Errorf("openai: response completed without output text")
	}
	return output, nil
}

func (p *openAIProvider) StreamCall(ctx context.Context, req CallRequest, w io.Writer) (err error) {
	stream := p.client.Responses.NewStreaming(ctx, p.buildParams(req))
	defer func() {
		// Record close error only when no earlier write/stream error has been captured.
		if cerr := stream.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("openai stream close: %w", cerr)
		}
	}()

	var wrote, completed bool
	var terminalErr error
	for stream.Next() {
		event := stream.Current()
		switch event.Type {
		case "response.completed":
			completed = true
		case "response.incomplete":
			terminalErr = newCompletionError(p.Name(), event.Response.IncompleteDetails.Reason, p.maxOutputTokens, wrote)
		case "response.failed":
			terminalErr = newCompletionError(p.Name(), "failed", p.maxOutputTokens, wrote)
		}
		if event.Type == "response.output_text.delta" && event.Delta.OfString != "" {
			if _, err := io.WriteString(w, event.Delta.OfString); err != nil {
				return fmt.Errorf("openai stream write: %w", err)
			}
			wrote = true
		}
	}
	if err := stream.Err(); err != nil {
		return fmt.Errorf("openai stream: %w", err)
	}
	if terminalErr != nil {
		return terminalErr
	}
	if !completed {
		return newCompletionError(p.Name(), "missing_terminal_status", p.maxOutputTokens, wrote)
	}
	if !wrote {
		return fmt.Errorf("openai: stream produced no output")
	}
	return nil
}

// mapEffort converts a validated effort string to the SDK constant.
// config.Load guarantees the value is one of {low,medium,high}; the
// default arm is unreachable in normal operation.
func mapEffort(effort string) shared.ReasoningEffort {
	switch effort {
	case "medium":
		return shared.ReasoningEffortMedium
	case "high":
		return shared.ReasoningEffortHigh
	default: // "low" and unreachable values
		return shared.ReasoningEffortLow
	}
}

// -----------------------------------------------------------------------------
// Chat Provider (Chat Completions API — Cerebras, Groq, etc.)
// -----------------------------------------------------------------------------

type chatProvider struct {
	name            string
	apiKey          string
	model           string
	maxOutputTokens int
	client          *openai.Client
}

// NewChat creates a Provider backed by the OpenAI-compatible Chat Completions API.
func NewChat(name, apiKey, model, baseURL string, maxRetries int, maxOutputTokens ...int) Provider {
	c := openai.NewClient(
		option.WithAPIKey(apiKey),
		option.WithBaseURL(baseURL),
		option.WithMaxRetries(maxRetries),
	)
	limit := 0
	if len(maxOutputTokens) > 0 {
		limit = maxOutputTokens[0]
	}
	return &chatProvider{name: name, apiKey: apiKey, model: model, maxOutputTokens: limit, client: &c}
}

func (p *chatProvider) Name() string   { return p.name }
func (p *chatProvider) Model() string  { return p.model }
func (p *chatProvider) APIKey() string { return p.apiKey }

func (p *chatProvider) buildParams(req CallRequest) openai.ChatCompletionNewParams {
	params := openai.ChatCompletionNewParams{
		Model: req.Model,
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(req.SystemPrompt),
			openai.UserMessage(req.UserPrompt),
		},
	}
	if p.maxOutputTokens > 0 {
		params.MaxCompletionTokens = openai.Int(int64(p.maxOutputTokens))
	}
	return params
}

func (p *chatProvider) Call(ctx context.Context, req CallRequest) (string, error) {
	resp, err := p.client.Chat.Completions.New(ctx, p.buildParams(req))
	if err != nil {
		return "", fmt.Errorf("%s: %w", p.name, err)
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("%s: no response choices", p.name)
	}
	choice := resp.Choices[0]
	if choice.FinishReason != "stop" {
		return "", newCompletionError(p.name, choice.FinishReason, p.maxOutputTokens, false)
	}
	if strings.TrimSpace(choice.Message.Content) == "" {
		return "", fmt.Errorf("%s: response completed without output text", p.name)
	}
	return choice.Message.Content, nil
}

func (p *chatProvider) StreamCall(ctx context.Context, req CallRequest, w io.Writer) (err error) {
	stream := p.client.Chat.Completions.NewStreaming(ctx, p.buildParams(req))
	defer func() {
		// Record close error only when no earlier write/stream error has been captured.
		if cerr := stream.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("%s stream close: %w", p.name, cerr)
		}
	}()

	var wrote, completed bool
	var terminalErr error
	for stream.Next() {
		chunk := stream.Current()
		for _, choice := range chunk.Choices {
			switch choice.FinishReason {
			case "":
			case "stop":
				completed = true
			default:
				terminalErr = newCompletionError(p.name, choice.FinishReason, p.maxOutputTokens, wrote)
			}
		}
		if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
			if _, err := io.WriteString(w, chunk.Choices[0].Delta.Content); err != nil {
				return fmt.Errorf("%s stream write: %w", p.name, err)
			}
			wrote = true
		}
	}
	if err := stream.Err(); err != nil {
		return fmt.Errorf("%s stream: %w", p.name, err)
	}
	if terminalErr != nil {
		return terminalErr
	}
	if !completed {
		return newCompletionError(p.name, "missing_terminal_status", p.maxOutputTokens, wrote)
	}
	if !wrote {
		return fmt.Errorf("%s: stream produced no output", p.name)
	}
	return nil
}

// -----------------------------------------------------------------------------
// Registry
// -----------------------------------------------------------------------------

// ProviderSettings holds the credentials and endpoint for a single provider.
type ProviderSettings struct {
	APIKey    string
	Model     string
	BaseURL   string
	ProjectID string
	Location  string
}

// RegistryConfig holds settings for all known providers.
type RegistryConfig struct {
	MaxOutputTokens int
	MaxRetries      int
	Providers       map[string]ProviderSettings
}

type providerTransport int

const (
	transportChat providerTransport = iota
	transportOpenAI
	transportGemini
)

type providerDescriptor struct {
	name      string
	transport providerTransport
	keyless   bool
}

var providerDescriptors = []providerDescriptor{
	{name: "openai", transport: transportOpenAI},
	{name: "cerebras", transport: transportChat},
	{name: "deepseek", transport: transportChat},
	{name: "groq", transport: transportChat},
	{name: "openrouter", transport: transportChat},
	{name: "zai", transport: transportChat},
	{name: "gemini", transport: transportGemini},
	{name: "omlx", transport: transportChat, keyless: true},
}

// NewRegistry builds a map of all providers keyed by name.
func NewRegistry(rc RegistryConfig) map[string]Provider {
	registry := make(map[string]Provider, len(providerDescriptors))
	for _, descriptor := range providerDescriptors {
		settings := rc.Providers[descriptor.name]
		apiKey := normalizedAPIKey(settings.APIKey, descriptor.keyless)
		switch descriptor.transport {
		case transportOpenAI:
			registry[descriptor.name] = NewOpenAI(apiKey, settings.Model, settings.BaseURL, rc.MaxRetries, rc.MaxOutputTokens)
		case transportGemini:
			registry[descriptor.name] = NewGemini(apiKey, settings.ProjectID, settings.Location, settings.Model, settings.BaseURL, rc.MaxRetries, rc.MaxOutputTokens)
		default:
			registry[descriptor.name] = NewChat(descriptor.name, apiKey, settings.Model, settings.BaseURL, rc.MaxRetries, rc.MaxOutputTokens)
		}
	}
	return registry
}

func normalizedAPIKey(apiKey string, keyless bool) string {
	if apiKey == "" && keyless {
		return "local"
	}
	return apiKey
}

// KnownNames returns the registered provider names in stable display order.
func KnownNames() []string {
	names := make([]string, 0, len(providerDescriptors))
	for _, descriptor := range providerDescriptors {
		names = append(names, descriptor.name)
	}
	sort.Strings(names)
	return names
}

func KnownNamesString() string {
	return strings.Join(KnownNames(), ", ")
}
