package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
)

const (
	defaultGeminiLocation = "global"
	defaultGeminiModel    = "gemini-3.7-flash"
	defaultGeminiBaseURL  = "https://aiplatform.googleapis.com/v1"
	vertexADCScope        = "https://www.googleapis.com/auth/cloud-platform"
	maxGeminiErrorBytes   = 64 << 10
	maxGeminiBodyBytes    = 16 << 20
)

type geminiProvider struct {
	apiKey          string
	projectID       string
	location        string
	model           string
	baseURL         string
	maxRetries      int
	maxOutputTokens int
	client          *http.Client
	tokenResolver   func(context.Context) (string, error)
}

type geminiGenerateContentRequest struct {
	SystemInstruction *geminiContent         `json:"systemInstruction,omitempty"`
	Contents          []geminiContent        `json:"contents"`
	GenerationConfig  geminiGenerationConfig `json:"generationConfig"`
}

type geminiGenerationConfig struct {
	MaxOutputTokens int                   `json:"maxOutputTokens,omitempty"`
	Temperature     *float64              `json:"temperature,omitempty"`
	ThinkingConfig  *geminiThinkingConfig `json:"thinkingConfig,omitempty"`
}

type geminiThinkingConfig struct {
	ThinkingLevel string `json:"thinkingLevel,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text    string `json:"text,omitempty"`
	Thought bool   `json:"thought,omitempty"`
}

type geminiGenerateContentResponse struct {
	Candidates     []geminiCandidate     `json:"candidates"`
	PromptFeedback *geminiPromptFeedback `json:"promptFeedback,omitempty"`
}

type geminiCandidate struct {
	Content      geminiContent `json:"content"`
	FinishReason string        `json:"finishReason,omitempty"`
}

type geminiPromptFeedback struct {
	BlockReason        string `json:"blockReason"`
	BlockReasonMessage string `json:"blockReasonMessage"`
}

// NewGemini creates a Provider backed by Vertex AI or Google AI Studio generateContent.
func NewGemini(apiKey, projectID, location, model, baseURL string, maxRetries, maxOutputTokens int) Provider {
	if location == "" {
		location = defaultGeminiLocation
	}
	if model == "" {
		model = defaultGeminiModel
	}
	if baseURL == "" {
		baseURL = defaultGeminiBaseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")

	return &geminiProvider{
		apiKey:          geminiAPIKey(apiKey),
		projectID:       projectID,
		location:        location,
		model:           model,
		baseURL:         baseURL,
		maxRetries:      max(maxRetries, 0),
		maxOutputTokens: maxOutputTokens,
		client:          &http.Client{},
		tokenResolver:   googleADCAccessToken,
	}
}

func geminiAPIKey(apiKey string) string {
	if apiKey == "" {
		return "adc"
	}
	return apiKey
}

func (p *geminiProvider) Name() string   { return "gemini" }
func (p *geminiProvider) Model() string  { return p.model }
func (p *geminiProvider) APIKey() string { return p.apiKey }

func googleADCAccessToken(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "gcloud", "auth", "application-default", "print-access-token", "--scopes="+vertexADCScope, "--quiet")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("gemini authentication failed (Google ADC token resolution failed).\nTo use Gemini, do one of:\n  1. Provide an AI Studio key: export GEMINI_API_KEY=\"AIza...\"\n  2. Authenticate Google Cloud ADC: gcloud auth application-default login\n  3. Switch default provider: prompter configure (e.g. OpenAI, Groq)")
	}
	token := strings.TrimSpace(string(out))
	if token == "" {
		return "", errors.New("gemini authentication failed: access token was empty")
	}
	return token, nil
}

func isGenerativeLanguageEndpoint(baseURL string) bool {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return false
	}
	return parsed.Scheme == "https" && strings.EqualFold(parsed.Hostname(), "generativelanguage.googleapis.com")
}

func (p *geminiProvider) getAccessToken(ctx context.Context) (string, error) {
	if p.apiKey != "" && p.apiKey != "adc" && !strings.HasPrefix(p.apiKey, "AIza") {
		return p.apiKey, nil
	}
	return p.tokenResolver(ctx)
}

func (p *geminiProvider) buildURL(streaming bool) string {
	method := ":generateContent"
	if streaming {
		method = ":streamGenerateContent?alt=sse"
	}
	if isGenerativeLanguageEndpoint(p.baseURL) {
		model := p.model
		if !strings.HasPrefix(model, "models/") {
			model = "models/" + model
		}
		return fmt.Sprintf("%s/%s%s", p.baseURL, model, method)
	}
	return fmt.Sprintf("%s/projects/%s/locations/%s/publishers/google/models/%s%s",
		p.baseURL,
		url.PathEscape(p.projectID),
		url.PathEscape(p.location),
		url.PathEscape(p.model),
		method,
	)
}

func (p *geminiProvider) buildRequestBody(req CallRequest) ([]byte, error) {
	payload := geminiGenerateContentRequest{
		Contents: []geminiContent{
			{
				Role:  "user",
				Parts: []geminiPart{{Text: req.UserPrompt}},
			},
		},
		GenerationConfig: geminiGenerationConfig{
			MaxOutputTokens: p.maxOutputTokens,
		},
	}

	if req.SystemPrompt != "" {
		payload.SystemInstruction = &geminiContent{
			Parts: []geminiPart{{Text: req.SystemPrompt}},
		}
	}

	if req.Effort != "" {
		payload.GenerationConfig.ThinkingConfig = &geminiThinkingConfig{
			ThinkingLevel: strings.ToUpper(strings.TrimSpace(req.Effort)),
		}
	}

	return json.Marshal(payload)
}

func (p *geminiProvider) setHeaders(ctx context.Context, httpReq *http.Request) error {
	httpReq.Header.Set("Content-Type", "application/json")
	if isGenerativeLanguageEndpoint(p.baseURL) {
		if p.apiKey == "" || p.apiKey == "adc" {
			return fmt.Errorf("API key required for Google AI Studio endpoint (%s)", p.baseURL)
		}
		httpReq.Header.Set("x-goog-api-key", p.apiKey)
		return nil
	}
	if strings.TrimSpace(p.projectID) == "" {
		return errors.New("project ID required for Vertex AI (set gemini.project_id, PROMPTER_GEMINI_PROJECT_ID, GEMINI_PROJECT_ID, GOOGLE_CLOUD_PROJECT, or GCP_PROJECT)")
	}

	token, err := p.getAccessToken(ctx)
	if err != nil {
		return err
	}
	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Header.Set("X-Goog-User-Project", p.projectID)
	return nil
}

func (p *geminiProvider) do(ctx context.Context, endpoint string, body []byte) (*http.Response, error) {
	for attempt := 0; ; attempt++ {
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("gemini create request: %w", err)
		}
		if err := p.setHeaders(ctx, httpReq); err != nil {
			return nil, fmt.Errorf("gemini: %w", err)
		}

		resp, err := p.client.Do(httpReq)
		if err != nil {
			if attempt < p.maxRetries && ctx.Err() == nil {
				continue
			}
			return nil, fmt.Errorf("gemini: %w", err)
		}
		if attempt < p.maxRetries && retryableGeminiStatus(resp.StatusCode) {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxGeminiErrorBytes))
			_ = resp.Body.Close()
			continue
		}
		return resp, nil
	}
}

func retryableGeminiStatus(status int) bool {
	return status == http.StatusRequestTimeout || status == http.StatusConflict || status == http.StatusTooManyRequests || status >= 500
}

func (p *geminiProvider) Call(ctx context.Context, req CallRequest) (string, error) {
	body, err := p.buildRequestBody(req)
	if err != nil {
		return "", fmt.Errorf("gemini encode request: %w", err)
	}

	endpoint := p.buildURL(false)
	resp, err := p.do(ctx, endpoint, body)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, truncated, err := readGeminiBody(resp.Body, maxGeminiErrorBytes)
		if err != nil {
			return "", fmt.Errorf("gemini read error response: %w", err)
		}
		if truncated {
			respBody = append(respBody, []byte(" [truncated]")...)
		}
		return "", fmt.Errorf("gemini: POST %q: %s %s", endpoint, resp.Status, strings.TrimSpace(string(respBody)))
	}

	var parsed geminiGenerateContentResponse
	if err := decodeGeminiBody(resp.Body, &parsed); err != nil {
		return "", fmt.Errorf("gemini decode response: %w", err)
	}

	if len(parsed.Candidates) == 0 {
		if parsed.PromptFeedback != nil && parsed.PromptFeedback.BlockReason != "" {
			return "", fmt.Errorf("gemini: prompt blocked: %s: %s", parsed.PromptFeedback.BlockReason, parsed.PromptFeedback.BlockReasonMessage)
		}
		return "", fmt.Errorf("gemini: response contains no candidates")
	}
	if parsed.Candidates[0].FinishReason != "STOP" {
		return "", newCompletionError(p.Name(), parsed.Candidates[0].FinishReason, p.maxOutputTokens, false)
	}

	var textParts []string
	for _, part := range parsed.Candidates[0].Content.Parts {
		if !part.Thought && part.Text != "" {
			textParts = append(textParts, part.Text)
		}
	}
	content := strings.Join(textParts, "")
	if strings.TrimSpace(content) == "" {
		return "", fmt.Errorf("gemini: no text returned (finish_reason=%s)", parsed.Candidates[0].FinishReason)
	}

	return content, nil
}

func (p *geminiProvider) StreamCall(ctx context.Context, req CallRequest, w io.Writer) error {
	body, err := p.buildRequestBody(req)
	if err != nil {
		return fmt.Errorf("gemini encode request: %w", err)
	}

	endpoint := p.buildURL(true)
	resp, err := p.do(ctx, endpoint, body)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, truncated, err := readGeminiBody(resp.Body, maxGeminiErrorBytes)
		if err != nil {
			return fmt.Errorf("gemini read error response: %w", err)
		}
		if truncated {
			respBody = append(respBody, []byte(" [truncated]")...)
		}
		return fmt.Errorf("gemini: POST %q: %s %s", endpoint, resp.Status, strings.TrimSpace(string(respBody)))
	}

	scanner := bufio.NewScanner(resp.Body)
	var wrote, completed bool
	var terminalErr error
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}

		var chunk geminiGenerateContentResponse
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return fmt.Errorf("gemini stream decode event: %w", err)
		}
		if len(chunk.Candidates) > 0 {
			candidate := chunk.Candidates[0]
			for _, part := range candidate.Content.Parts {
				if !part.Thought && part.Text != "" {
					if _, err := io.WriteString(w, part.Text); err != nil {
						return fmt.Errorf("gemini stream write: %w", err)
					}
					wrote = true
				}
			}
			switch candidate.FinishReason {
			case "":
			case "STOP":
				completed = true
			default:
				terminalErr = newCompletionError(p.Name(), candidate.FinishReason, p.maxOutputTokens, wrote)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("gemini stream: %w", err)
	}
	if terminalErr != nil {
		return terminalErr
	}
	if !completed {
		return newCompletionError(p.Name(), "missing_terminal_status", p.maxOutputTokens, wrote)
	}
	if !wrote {
		return fmt.Errorf("gemini: stream produced no output")
	}

	return nil
}

func readGeminiBody(r io.Reader, limit int64) ([]byte, bool, error) {
	body, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, false, err
	}
	if int64(len(body)) > limit {
		return body[:limit], true, nil
	}
	return body, false, nil
}

func decodeGeminiBody(r io.Reader, dst any) error {
	return decodeBoundedGeminiJSON(r, dst, maxGeminiBodyBytes)
}

func decodeBoundedGeminiJSON(r io.Reader, dst any, limit int64) error {
	body, truncated, err := readGeminiBody(r, limit)
	if err != nil {
		return err
	}
	if truncated {
		return fmt.Errorf("response exceeds %d-byte limit", limit)
	}
	return json.Unmarshal(body, dst)
}
