package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestKnownNamesStable(t *testing.T) {
	t.Parallel()

	got := KnownNames()
	want := []string{"cerebras", "gemini", "groq", "omlx", "openai", "openrouter", "synthetic", "wormhole", "zai"}
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

	prov := NewChat("synthetic", "key", "model", "http://test", 3, 123)
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

func TestWormholeProviderDoesNotRequireAPIKey(t *testing.T) {
	t.Parallel()

	registry := NewRegistry(RegistryConfig{
		Providers: map[string]ProviderSettings{
			"wormhole": {Model: "groq/openai/gpt-oss-120b", BaseURL: "http://127.0.0.1:8080/v1"},
		},
	})
	prov := registry["wormhole"]
	if prov.Name() != "wormhole" {
		t.Fatalf("Name = %q, want wormhole", prov.Name())
	}
	if prov.Model() != "groq/openai/gpt-oss-120b" {
		t.Fatalf("Model = %q, want groq/openai/gpt-oss-120b", prov.Model())
	}
	if prov.APIKey() == "" {
		t.Fatal("APIKey should be non-empty sentinel for local provider")
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

func TestWormholeProviderCallsOpenAICompatibleAPI(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.Model != "groq/openai/gpt-oss-120b" {
			t.Errorf("Model = %q, want groq/openai/gpt-oss-120b", req.Model)
		}
		if len(req.Messages) != 2 || req.Messages[0].Role != "system" || req.Messages[0].Content != "system" || req.Messages[1].Role != "user" || req.Messages[1].Content != "user" {
			t.Errorf("Messages = %+v, want system and user messages", req.Messages)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chatcmpl-test",
			"object":  "chat.completion",
			"created": 1,
			"model":   req.Model,
			"choices": []map[string]any{{
				"index": 0,
				"message": map[string]string{
					"role":    "assistant",
					"content": "from wormhole",
				},
				"finish_reason": "stop",
			}},
		})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	prov := NewChat("wormhole", "local", "groq/openai/gpt-oss-120b", server.URL+"/v1", 0)
	got, err := prov.Call(context.Background(), CallRequest{
		Model:        "groq/openai/gpt-oss-120b",
		SystemPrompt: "system",
		UserPrompt:   "user",
	})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got != "from wormhole" {
		t.Fatalf("Call = %q, want from wormhole", got)
	}
}

func TestChatProviderCallRejectsLengthTruncation(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"id":"chatcmpl-test",
			"object":"chat.completion",
			"created":1,
			"model":"model",
			"choices":[{
				"index":0,
				"message":{"role":"assistant","content":"partial response"},
				"finish_reason":"length"
			}]
		}`)
	}))
	t.Cleanup(server.Close)

	prov := NewChat("omlx", "local", "model", server.URL+"/v1", 0, 8192)
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

func TestChatProviderStreamCallReportsLengthTruncationAfterWritingPartialOutput(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"id\":\"chatcmpl-test\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"model\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"partial response\"},\"finish_reason\":null}]}\n\n")
		_, _ = io.WriteString(w, "data: {\"id\":\"chatcmpl-test\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"model\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"length\"}]}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(server.Close)

	prov := NewChat("omlx", "local", "model", server.URL+"/v1", 0, 8192)
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

func TestWormholeProviderReportsInvalidModel(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"unknown provider prefix","type":"invalid_request_error"}}`))
	}))
	t.Cleanup(server.Close)

	prov := NewChat("wormhole", "local", "missing/model", server.URL+"/v1", 0)
	_, err := prov.Call(context.Background(), CallRequest{
		Model:        "missing/model",
		SystemPrompt: "system",
		UserPrompt:   "user",
	})
	if err == nil {
		t.Fatal("Call expected invalid model error, got nil")
	}
	for _, want := range []string{"wormhole", "unknown provider prefix"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("Call error = %q, want %q", err, want)
		}
	}
}

func TestGeminiProviderCallsVertexAPI(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /projects/grimoire-2025/locations/global/publishers/google/models/gemini-3.7-flash:generateContent", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("Authorization = %q, want Bearer test-token", r.Header.Get("Authorization"))
		}
		if r.Header.Get("X-Goog-User-Project") != "grimoire-2025" {
			t.Errorf("X-Goog-User-Project = %q, want grimoire-2025", r.Header.Get("X-Goog-User-Project"))
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
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if req.SystemInstruction == nil || len(req.SystemInstruction.Parts) == 0 || req.SystemInstruction.Parts[0].Text != "system" {
			t.Errorf("SystemInstruction = %+v, want system text", req.SystemInstruction)
		}
		if len(req.Contents) == 0 || len(req.Contents[0].Parts) == 0 || req.Contents[0].Parts[0].Text != "user" {
			t.Errorf("Contents = %+v, want user text", req.Contents)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
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
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	prov := NewGemini("test-token", "grimoire-2025", "global", "gemini-3.7-flash", server.URL, 0, 1024)
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

func TestGeminiProviderUsesADCWhenAPIKeyIsAIza(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /projects/grimoire-2025/locations/global/publishers/google/models/gemini-3.7-flash:generateContent", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer adc-resolved-token" {
			t.Errorf("Authorization = %q, want Bearer adc-resolved-token", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
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
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	prov := NewGemini("AIzaSyExampleKey", "grimoire-2025", "global", "gemini-3.7-flash", server.URL, 0, 1024)
	gemini, ok := prov.(*geminiProvider)
	if !ok {
		t.Fatalf("expected *geminiProvider, got %T", prov)
	}
	gemini.tokenResolver = func(context.Context) (string, error) {
		return "adc-resolved-token", nil
	}

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

	prov := NewGemini("AIzaSyExampleKey", "", "", "gemini-2.5-flash", "https://generativelanguage.googleapis.com/v1beta", 0, 1024)
	gemini, ok := prov.(*geminiProvider)
	if !ok {
		t.Fatalf("expected *geminiProvider, got %T", prov)
	}

	gemini.client = &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.String() != "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:generateContent" {
				t.Errorf("URL = %q, want https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:generateContent", r.URL.String())
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
		Model:        "gemini-2.5-flash",
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

func TestGeminiStreamRejectsMalformedEvent(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"partial\"}]}}]}\n\n")
		_, _ = io.WriteString(w, "data: not-json\n\n")
	}))
	t.Cleanup(server.Close)

	prov := NewGemini("test-token", "project", "global", "model", server.URL, 0, 1024)
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
