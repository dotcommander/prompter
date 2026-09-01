package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

func assertCompletionError(t *testing.T, err error, reason string, partial bool) {
	t.Helper()
	if err == nil {
		t.Fatal("expected completion error, got nil")
	}
	var completionErr *CompletionError
	if !errors.As(err, &completionErr) {
		t.Fatalf("error = %T %v, want *CompletionError", err, err)
	}
	if completionErr.reason != reason {
		t.Fatalf("completion reason = %q, want %q", completionErr.reason, reason)
	}
	if completionErr.partial != partial {
		t.Fatalf("completion partial = %t, want %t", completionErr.partial, partial)
	}
}

func TestKnownNamesStable(t *testing.T) {
	t.Parallel()

	got := KnownNames()
	want := []string{"cerebras", "deepseek", "gemini", "groq", "omlx", "openai", "openrouter", "zai"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("KnownNames = %v, want %v", got, want)
	}
}

func TestRegistryUsesDescriptorTransports(t *testing.T) {
	t.Parallel()

	settings := make(map[string]ProviderSettings)
	for _, name := range KnownNames() {
		settings[name] = ProviderSettings{APIKey: "key", Model: "model", BaseURL: "http://test"}
	}
	registry := NewRegistry(RegistryConfig{Providers: settings})

	for name, prov := range registry {
		switch name {
		case "openai":
			if _, ok := prov.(*openAIProvider); !ok {
				t.Errorf("%s transport = %T, want *openAIProvider", name, prov)
			}
		case "gemini":
			if _, ok := prov.(*geminiProvider); !ok {
				t.Errorf("%s transport = %T, want *geminiProvider", name, prov)
			}
		default:
			if _, ok := prov.(*chatProvider); !ok {
				t.Errorf("%s transport = %T, want *chatProvider", name, prov)
			}
		}
	}
}

func TestChatProviderBuildParamsUsesMaxCompletionTokens(t *testing.T) {
	t.Parallel()

	prov := NewChat("groq", "key", "model", "http://test", 3, 123)
	chat, ok := prov.(*chatProvider)
	if !ok {
		t.Fatalf("NewChat returned %T, want *chatProvider", prov)
	}

	params := chat.buildParams(CallRequest{
		Model:        "model",
		SystemPrompt: "system",
		UserPrompt:   "user",
	})

	if !params.MaxCompletionTokens.Valid() {
		t.Fatal("MaxCompletionTokens omitted")
	}
	if params.MaxCompletionTokens.Value != 123 {
		t.Fatalf("MaxCompletionTokens = %d, want 123", params.MaxCompletionTokens.Value)
	}
}

func TestOpenAIProviderBuildParamsIncludesReasoningOnlyForSupportedModels(t *testing.T) {
	t.Parallel()

	provider := NewOpenAI("key", "model", "", 0, 123).(*openAIProvider)
	for _, tt := range []struct {
		model string
		want  bool
	}{
		{model: "non-reasoning-model", want: false},
		{model: "gpt-5.6-luna", want: true},
	} {
		t.Run(tt.model, func(t *testing.T) {
			params := provider.buildParams(CallRequest{Model: tt.model, Effort: "high"})
			if got := params.Reasoning.Effort != ""; got != tt.want {
				t.Fatalf("Reasoning.Effort present = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestOmlxProviderDoesNotRequireAPIKey(t *testing.T) {
	t.Parallel()

	registry := NewRegistry(RegistryConfig{
		Providers: map[string]ProviderSettings{
			"omlx": {Model: "Ornith-1.5-35B-A3B-oQ4e-mtp", BaseURL: "http://127.0.0.1:8000/v1"},
		},
	})
	prov := registry["omlx"]
	if prov.Name() != "omlx" {
		t.Fatalf("Name = %q, want omlx", prov.Name())
	}
	if prov.Model() != "Ornith-1.5-35B-A3B-oQ4e-mtp" {
		t.Fatalf("Model = %q, want Ornith-1.5-35B-A3B-oQ4e-mtp", prov.Model())
	}
	if prov.APIKey() == "" {
		t.Fatal("APIKey should be non-empty sentinel for local provider")
	}
}

func TestChatProviderCallRejectsLengthTruncation(t *testing.T) {
	t.Parallel()

	prov := newChatTestProvider("omlx", "model", 8192, staticResponseTransport("application/json", `{
			"id":"chatcmpl-test",
			"object":"chat.completion",
			"created":1,
			"model":"model",
			"choices":[{
				"index":0,
				"message":{"role":"assistant","content":"partial response"},
				"finish_reason":"length"
			}]
		}`))

	got, err := prov.Call(context.Background(), CallRequest{Model: "model", UserPrompt: "input"})
	if got != "" {
		t.Fatalf("Call output = %q, want empty on truncation", got)
	}
	if err == nil {
		t.Fatal("Call expected truncation error, got nil")
	}
	for _, want := range []string{"omlx", "8192", "truncated", "max_output_tokens"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("Call error = %q, want %q", err, want)
		}
	}
}

func TestChatProviderCallRejectsNonSuccessTerminalStates(t *testing.T) {
	t.Parallel()

	for _, reason := range []string{"content_filter", "tool_calls", ""} {
		t.Run(reason, func(t *testing.T) {
			t.Parallel()
			body := fmt.Sprintf(`{
				"id":"chatcmpl-test","object":"chat.completion","created":1,"model":"model",
				"choices":[{"index":0,"message":{"role":"assistant","content":""},"finish_reason":%q}]
			}`, reason)
			prov := newChatTestProvider("omlx", "model", 8192, staticResponseTransport("application/json", body))
			got, err := prov.Call(context.Background(), CallRequest{Model: "model", UserPrompt: "input"})
			if got != "" {
				t.Fatalf("Call output = %q, want empty", got)
			}
			wantReason := reason
			if wantReason == "" {
				wantReason = "missing_terminal_status"
			}
			assertCompletionError(t, err, wantReason, false)
		})
	}
}

func TestChatProviderStreamCallReportsLengthTruncationAfterWritingPartialOutput(t *testing.T) {
	t.Parallel()

	body := "data: {\"id\":\"chatcmpl-test\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"model\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"partial response\"},\"finish_reason\":null}]}\n\n" +
		"data: {\"id\":\"chatcmpl-test\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"model\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"length\"}]}\n\n" +
		"data: [DONE]\n\n"
	prov := newChatTestProvider("omlx", "model", 8192, staticResponseTransport("text/event-stream", body))

	var output bytes.Buffer
	err := prov.StreamCall(context.Background(), CallRequest{Model: "model", UserPrompt: "input"}, &output)
	if output.String() != "partial response" {
		t.Fatalf("StreamCall output = %q, want partial response", output.String())
	}
	if err == nil {
		t.Fatal("StreamCall expected truncation error, got nil")
	}
	for _, want := range []string{"omlx", "8192", "partial output", "max_output_tokens"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("StreamCall error = %q, want %q", err, want)
		}
	}
}

func TestChatProviderStreamRequiresStopTerminalState(t *testing.T) {
	t.Parallel()

	for name, terminal := range map[string]string{
		"filtered": `data: {"id":"chatcmpl-test","object":"chat.completion.chunk","created":1,"model":"model","choices":[{"index":0,"delta":{},"finish_reason":"content_filter"}]}`,
		"missing":  "",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			body := "data: {\"id\":\"chatcmpl-test\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"model\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"partial response\"},\"finish_reason\":null}]}\n\n" + terminal + "\n\ndata: [DONE]\n\n"
			prov := newChatTestProvider("omlx", "model", 8192, staticResponseTransport("text/event-stream", body))
			var output bytes.Buffer
			err := prov.StreamCall(context.Background(), CallRequest{Model: "model", UserPrompt: "input"}, &output)
			if output.String() != "partial response" {
				t.Fatalf("StreamCall output = %q, want partial response", output.String())
			}
			if err == nil {
				t.Fatal("StreamCall expected terminal-state error")
			}
		})
	}
}

func TestGeminiProviderRetriesTransientResponses(t *testing.T) {
	t.Parallel()

	attempts := 0
	transport := roundTripperFunc(func(*http.Request) (*http.Response, error) {
		attempts++
		if attempts < 3 {
			return statusResponse(http.StatusInternalServerError, "text/plain", []byte("retry")), nil
		}
		return statusResponse(http.StatusOK, "application/json", []byte(`{"candidates":[{"content":{"parts":[{"text":"complete"}]},"finishReason":"STOP"}]}`)), nil
	})
	prov := NewGemini("test-token", "project", "global", "model", "http://test", 2, 1024)
	gemini := prov.(*geminiProvider)
	gemini.client = &http.Client{Transport: transport}

	got, err := prov.Call(context.Background(), CallRequest{Model: "model", UserPrompt: "input"})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got != "complete" || attempts != 3 {
		t.Fatalf("Call = %q after %d attempts, want complete after 3", got, attempts)
	}
}

func TestGeminiProviderUsesCallerContextForTimeout(t *testing.T) {
	t.Parallel()

	prov := NewGemini("test-token", "project", "global", "model", "http://test", 0, 1024)
	if timeout := prov.(*geminiProvider).client.Timeout; timeout != 0 {
		t.Fatalf("HTTP client timeout = %s, want zero so caller context owns timeout", timeout)
	}
}

func TestReadGeminiBodyReportsTruncation(t *testing.T) {
	t.Parallel()

	body, truncated, err := readGeminiBody(strings.NewReader("123456"), 5)
	if err != nil {
		t.Fatalf("readGeminiBody: %v", err)
	}
	if got := string(body); got != "12345" || !truncated {
		t.Fatalf("readGeminiBody = %q, %t; want %q, true", got, truncated, "12345")
	}
}

func TestDecodeGeminiBodyRejectsOversizeResponse(t *testing.T) {
	t.Parallel()

	var decoded geminiGenerateContentResponse
	err := decodeBoundedGeminiJSON(strings.NewReader("123456"), &decoded, 5)
	if err == nil || !strings.Contains(err.Error(), "exceeds 5-byte limit") {
		t.Fatalf("decodeBoundedGeminiJSON error = %v, want size-limit error", err)
	}
}

func TestOpenAIProviderAcceptsCompletedResponse(t *testing.T) {
	t.Parallel()

	prov := newOpenAITestProvider("application/json", `{
			"id":"resp-test","object":"response","created_at":1,"model":"model","status":"completed",
			"output":[{"id":"msg-test","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"complete response","annotations":[],"logprobs":[]}]}]
		}`)

	got, err := prov.Call(context.Background(), CallRequest{Model: "model", UserPrompt: "input"})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got != "complete response" {
		t.Fatalf("Call output = %q, want complete response", got)
	}
}

func TestOpenAIProviderRejectsIncompleteResponse(t *testing.T) {
	t.Parallel()

	prov := newOpenAITestProvider("application/json", `{
			"id":"resp-test","object":"response","created_at":1,"model":"model","status":"incomplete",
			"incomplete_details":{"reason":"max_output_tokens"},
			"output":[{"id":"msg-test","type":"message","role":"assistant","status":"incomplete","content":[{"type":"output_text","text":"partial response","annotations":[],"logprobs":[]}]}]
		}`)

	got, err := prov.Call(context.Background(), CallRequest{Model: "model", UserPrompt: "input"})
	if got != "" {
		t.Fatalf("Call output = %q, want empty", got)
	}
	assertCompletionError(t, err, "max_output_tokens", false)
}

func TestOpenAIProviderRejectsOtherNonSuccessAndEmptyOutput(t *testing.T) {
	t.Parallel()

	for name, body := range map[string]string{
		"failed": `{"id":"resp-test","object":"response","created_at":1,"model":"model","status":"failed","output":[]}`,
		"empty":  `{"id":"resp-test","object":"response","created_at":1,"model":"model","status":"completed","output":[]}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			prov := newOpenAITestProvider("application/json", body)
			got, err := prov.Call(context.Background(), CallRequest{Model: "model", UserPrompt: "input"})
			if got != "" || err == nil {
				t.Fatalf("Call = %q, %v; want empty output and error", got, err)
			}
			if strings.Contains(strings.ToLower(err.Error()), "token limit") {
				t.Fatalf("Call error mislabeled as token limit: %v", err)
			}
		})
	}
}

func TestOpenAIProviderStreamRequiresCompletedTerminalEvent(t *testing.T) {
	t.Parallel()

	body := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial response\",\"item_id\":\"msg-test\",\"output_index\":0,\"content_index\":0,\"sequence_number\":1}\n\n" +
		"data: {\"type\":\"response.incomplete\",\"sequence_number\":2,\"response\":{\"id\":\"resp-test\",\"object\":\"response\",\"created_at\":1,\"model\":\"model\",\"status\":\"incomplete\",\"incomplete_details\":{\"reason\":\"max_output_tokens\"},\"output\":[]}}\n\n" +
		"data: [DONE]\n\n"
	prov := newOpenAITestProvider("text/event-stream", body)

	var output bytes.Buffer
	err := prov.StreamCall(context.Background(), CallRequest{Model: "model", UserPrompt: "input"}, &output)
	if output.String() != "partial response" {
		t.Fatalf("StreamCall output = %q, want partial response", output.String())
	}
	assertCompletionError(t, err, "max_output_tokens", true)
}

func TestOpenAIProviderStreamAcceptsCompletedTerminalEvent(t *testing.T) {
	t.Parallel()

	body := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"complete response\",\"item_id\":\"msg-test\",\"output_index\":0,\"content_index\":0,\"sequence_number\":1}\n\n" +
		"data: {\"type\":\"response.completed\",\"sequence_number\":2,\"response\":{\"id\":\"resp-test\",\"object\":\"response\",\"created_at\":1,\"model\":\"model\",\"status\":\"completed\",\"output\":[]}}\n\n" +
		"data: [DONE]\n\n"
	prov := newOpenAITestProvider("text/event-stream", body)

	var output bytes.Buffer
	if err := prov.StreamCall(context.Background(), CallRequest{Model: "model", UserPrompt: "input"}, &output); err != nil {
		t.Fatalf("StreamCall: %v", err)
	}
	if output.String() != "complete response" {
		t.Fatalf("StreamCall output = %q, want complete response", output.String())
	}
}

func TestGeminiProviderCallsVertexAPI(t *testing.T) {
	t.Parallel()

	transport := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		wantPath := "/projects/test-project/locations/global/publishers/google/models/gemini-3.7-flash:generateContent"
		if r.Method != http.MethodPost || r.URL.Path != wantPath {
			t.Errorf("request = %s %s, want POST %s", r.Method, r.URL.Path, wantPath)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("Authorization = %q, want Bearer test-token", r.Header.Get("Authorization"))
		}
		if r.Header.Get("X-Goog-User-Project") != "test-project" {
			t.Errorf("X-Goog-User-Project = %q, want test-project", r.Header.Get("X-Goog-User-Project"))
		}

		var req struct {
			SystemInstruction *struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"systemInstruction"`
			Contents []struct {
				Role  string `json:"role"`
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"contents"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
			return nil, err
		}

		if req.SystemInstruction == nil || len(req.SystemInstruction.Parts) == 0 || req.SystemInstruction.Parts[0].Text != "system" {
			t.Errorf("SystemInstruction = %+v, want system text", req.SystemInstruction)
		}
		if len(req.Contents) == 0 || len(req.Contents[0].Parts) == 0 || req.Contents[0].Parts[0].Text != "user" {
			t.Errorf("Contents = %+v, want user text", req.Contents)
		}

		data, err := json.Marshal(map[string]any{
			"candidates": []map[string]any{
				{
					"content": map[string]any{
						"role": "model",
						"parts": []map[string]any{
							{"text": "from gemini vertex"},
						},
					},
					"finishReason": "STOP",
				},
			},
		})
		if err != nil {
			return nil, err
		}
		return jsonResponse(http.StatusOK, data), nil
	})

	prov := NewGemini("test-token", "test-project", "global", "gemini-3.7-flash", "http://test", 0, 1024)
	prov.(*geminiProvider).client = &http.Client{Transport: transport}
	if prov.Name() != "gemini" {
		t.Fatalf("Name = %q, want gemini", prov.Name())
	}
	if prov.Model() != "gemini-3.7-flash" {
		t.Fatalf("Model = %q, want gemini-3.7-flash", prov.Model())
	}

	got, err := prov.Call(context.Background(), CallRequest{
		Model:        "gemini-3.7-flash",
		SystemPrompt: "system",
		UserPrompt:   "user",
	})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got != "from gemini vertex" {
		t.Fatalf("Call = %q, want from gemini vertex", got)
	}
}

func TestGeminiProviderRequiresExplicitVertexProject(t *testing.T) {
	t.Parallel()
	prov := NewGemini("test-token", "", "global", "gemini-3.7-flash", "http://test", 0, 1024)
	gemini := prov.(*geminiProvider)
	gemini.tokenResolver = func(context.Context) (string, error) {
		t.Fatal("token resolver called without a project ID")
		return "", nil
	}
	_, err := prov.Call(context.Background(), CallRequest{UserPrompt: "input"})
	if err == nil || !strings.Contains(err.Error(), "project ID required for Vertex AI") {
		t.Fatalf("Call error = %v, want missing-project error", err)
	}
}

func TestGeminiProviderRejectsMaxTokensResponse(t *testing.T) {
	t.Parallel()

	prov := newGeminiTestProvider("application/json", `{"candidates":[{"content":{"parts":[{"text":"partial response"}]},"finishReason":"MAX_TOKENS"}]}`)
	got, err := prov.Call(context.Background(), CallRequest{Model: "model", UserPrompt: "input"})
	if got != "" {
		t.Fatalf("Call output = %q, want empty", got)
	}
	assertCompletionError(t, err, "MAX_TOKENS", false)
}

func TestGeminiProviderRejectsOtherNonSuccessReason(t *testing.T) {
	t.Parallel()

	prov := newGeminiTestProvider("application/json", `{"candidates":[{"content":{"parts":[{"text":"unsafe response"}]},"finishReason":"SAFETY"}]}`)
	got, err := prov.Call(context.Background(), CallRequest{Model: "model", UserPrompt: "input"})
	if got != "" {
		t.Fatalf("Call output = %q, want empty", got)
	}
	assertCompletionError(t, err, "SAFETY", false)
	if strings.Contains(strings.ToLower(err.Error()), "token limit") {
		t.Fatalf("Call error mislabeled as token limit: %v", err)
	}
}

func TestGeminiProviderStreamReportsMaxTokensAfterPartialOutput(t *testing.T) {
	t.Parallel()

	prov := newGeminiTestProvider("text/event-stream", "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"partial response\"}]},\"finishReason\":\"MAX_TOKENS\"}]}\n\n")
	var output bytes.Buffer
	err := prov.StreamCall(context.Background(), CallRequest{Model: "model", UserPrompt: "input"}, &output)
	if output.String() != "partial response" {
		t.Fatalf("StreamCall output = %q, want partial response", output.String())
	}
	assertCompletionError(t, err, "MAX_TOKENS", true)
}

func TestGeminiProviderStreamAcceptsStop(t *testing.T) {
	t.Parallel()

	prov := newGeminiTestProvider("text/event-stream", "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"complete response\"}]},\"finishReason\":\"STOP\"}]}\n\n")
	var output bytes.Buffer
	if err := prov.StreamCall(context.Background(), CallRequest{Model: "model", UserPrompt: "input"}, &output); err != nil {
		t.Fatalf("StreamCall: %v", err)
	}
	if output.String() != "complete response" {
		t.Fatalf("StreamCall output = %q, want complete response", output.String())
	}
}

func TestGeminiProviderUsesADCWhenAPIKeyIsAIza(t *testing.T) {
	t.Parallel()

	transport := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		wantPath := "/projects/test-project/locations/global/publishers/google/models/gemini-3.7-flash:generateContent"
		if r.Method != http.MethodPost || r.URL.Path != wantPath {
			t.Errorf("request = %s %s, want POST %s", r.Method, r.URL.Path, wantPath)
		}
		if r.Header.Get("Authorization") != "Bearer adc-resolved-token" {
			t.Errorf("Authorization = %q, want Bearer adc-resolved-token", r.Header.Get("Authorization"))
		}
		data, err := json.Marshal(map[string]any{
			"candidates": []map[string]any{
				{
					"content": map[string]any{
						"role": "model",
						"parts": []map[string]any{
							{"text": "from vertex adc"},
						},
					},
					"finishReason": "STOP",
				},
			},
		})
		if err != nil {
			return nil, err
		}
		return jsonResponse(http.StatusOK, data), nil
	})

	prov := NewGemini("AIzaSyExampleKey", "test-project", "global", "gemini-3.7-flash", "http://test", 0, 1024)
	gemini, ok := prov.(*geminiProvider)
	if !ok {
		t.Fatalf("expected *geminiProvider, got %T", prov)
	}
	gemini.tokenResolver = func(context.Context) (string, error) {
		return "adc-resolved-token", nil
	}
	gemini.client = &http.Client{Transport: transport}

	got, err := prov.Call(context.Background(), CallRequest{
		Model:        "gemini-3.7-flash",
		SystemPrompt: "system",
		UserPrompt:   "user",
	})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got != "from vertex adc" {
		t.Fatalf("Call = %q, want from vertex adc", got)
	}
}

func TestGeminiProviderCallsGoogleAIStudioAPI(t *testing.T) {
	t.Parallel()

	prov := NewGemini("AIzaSyExampleKey", "", "", "gemini-3.7-flash", "https://generativelanguage.googleapis.com/v1beta", 0, 1024)
	gemini, ok := prov.(*geminiProvider)
	if !ok {
		t.Fatalf("expected *geminiProvider, got %T", prov)
	}

	gemini.client = &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.String() != "https://generativelanguage.googleapis.com/v1beta/models/gemini-3.7-flash:generateContent" {
				t.Errorf("URL = %q, want https://generativelanguage.googleapis.com/v1beta/models/gemini-3.7-flash:generateContent", r.URL.String())
			}
			if r.Header.Get("x-goog-api-key") != "AIzaSyExampleKey" {
				t.Errorf("x-goog-api-key = %q, want AIzaSyExampleKey", r.Header.Get("x-goog-api-key"))
			}
			if r.Header.Get("Authorization") != "" {
				t.Errorf("Authorization should be empty, got %q", r.Header.Get("Authorization"))
			}

			resp := map[string]any{
				"candidates": []map[string]any{
					{
						"content": map[string]any{
							"role": "model",
							"parts": []map[string]any{
								{"text": "from ai studio"},
							},
						},
						"finishReason": "STOP",
					},
				},
			}
			data, _ := json.Marshal(resp)
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(bytes.NewReader(data)),
			}, nil
		}),
	}

	got, err := prov.Call(context.Background(), CallRequest{
		Model:        "gemini-3.7-flash",
		SystemPrompt: "system",
		UserPrompt:   "user",
	})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got != "from ai studio" {
		t.Fatalf("Call = %q, want from ai studio", got)
	}
}

func TestGenerativeLanguageEndpointRequiresTrustedHTTPSHost(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		baseURL string
		want    bool
	}{
		{name: "trusted host", baseURL: "https://generativelanguage.googleapis.com/v1beta", want: true},
		{name: "case insensitive host", baseURL: "https://GENERATIVELANGUAGE.GOOGLEAPIS.COM/v1beta", want: true},
		{name: "lookalike suffix", baseURL: "https://generativelanguage.googleapis.com.attacker.example/v1beta"},
		{name: "lookalike prefix", baseURL: "https://generativelanguage.googleapis.com@attacker.example/v1beta"},
		{name: "plaintext transport", baseURL: "http://generativelanguage.googleapis.com/v1beta"},
		{name: "malformed URL", baseURL: "://generativelanguage.googleapis.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := isGenerativeLanguageEndpoint(tt.baseURL); got != tt.want {
				t.Fatalf("isGenerativeLanguageEndpoint(%q) = %t, want %t", tt.baseURL, got, tt.want)
			}
		})
	}
}

func TestGeminiStreamRejectsMalformedEvent(t *testing.T) {
	t.Parallel()

	body := "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"partial\"}]}}]}\n\n" +
		"data: not-json\n\n"
	prov := newGeminiTestProvider("text/event-stream", body)
	var output bytes.Buffer
	err := prov.StreamCall(context.Background(), CallRequest{Model: "model", UserPrompt: "input"}, &output)
	if err == nil || !strings.Contains(err.Error(), "stream decode event") {
		t.Fatalf("StreamCall error = %v, want stream decode event", err)
	}
	if output.String() != "partial" {
		t.Fatalf("stream output = %q, want partial", output.String())
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func newOpenAITestProvider(contentType, body string) Provider {
	httpClient := &http.Client{Transport: staticResponseTransport(contentType, body)}
	client := openai.NewClient(
		option.WithAPIKey("key"),
		option.WithBaseURL("http://test/v1"),
		option.WithMaxRetries(0),
		option.WithHTTPClient(httpClient),
	)
	return &openAIProvider{
		apiKey:          "key",
		model:           "model",
		baseURL:         "http://test/v1",
		maxOutputTokens: 8192,
		client:          &client,
	}
}

func newChatTestProvider(name, model string, maxOutputTokens int, transport http.RoundTripper) Provider {
	client := openai.NewClient(
		option.WithAPIKey("local"),
		option.WithBaseURL("http://test/v1"),
		option.WithMaxRetries(0),
		option.WithHTTPClient(&http.Client{Transport: transport}),
	)
	return &chatProvider{
		name:            name,
		apiKey:          "local",
		model:           model,
		maxOutputTokens: maxOutputTokens,
		client:          &client,
	}
}

func newGeminiTestProvider(contentType, body string) Provider {
	prov := NewGemini("test-token", "project", "global", "model", "http://test", 0, 8192)
	prov.(*geminiProvider).client = &http.Client{Transport: staticResponseTransport(contentType, body)}
	return prov
}

func staticResponseTransport(contentType, body string) http.RoundTripper {
	return staticStatusResponseTransport(http.StatusOK, contentType, body)
}

func staticStatusResponseTransport(statusCode int, contentType, body string) http.RoundTripper {
	return roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return statusResponse(statusCode, contentType, []byte(body)), nil
	})
}

func jsonResponse(statusCode int, data []byte) *http.Response {
	return statusResponse(statusCode, "application/json", data)
}

func statusResponse(statusCode int, contentType string, body []byte) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Status:     fmt.Sprintf("%d %s", statusCode, http.StatusText(statusCode)),
		Header:     http.Header{"Content-Type": []string{contentType}},
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
}
