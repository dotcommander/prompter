package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGeminiRequestModelRouting(t *testing.T) {
	for _, base := range []string{"http://test", "https://generativelanguage.googleapis.com/v1"} {
		for _, streaming := range []bool{false, true} {
			for _, model := range []string{"override", ""} {
				p := NewGemini("test-token", "project", "global", "configured", base, 0, 32).(*geminiProvider)
				p.client = &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
					want := model
					if want == "" {
						want = "configured"
					}
					method := ":generateContent"
					if streaming {
						method = ":streamGenerateContent"
					}
					if !strings.HasSuffix(r.URL.Path, "/models/"+want+method) {
						t.Errorf("request path = %q, want model %q method %q", r.URL.Path, want, method)
					}
					body := `{"candidates":[{"content":{"parts":[{"text":"ok"}]},"finishReason":"STOP"}]}`
					if streaming {
						return statusResponse(200, "text/event-stream", []byte("data: "+body+"\n\n")), nil
					}
					return jsonResponse(200, []byte(body)), nil
				})}
				req := CallRequest{Model: model, UserPrompt: "input"}
				if streaming {
					if err := p.StreamCall(context.Background(), req, io.Discard); err != nil {
						t.Fatal(err)
					}
				} else {
					if _, err := p.Call(context.Background(), req); err != nil {
						t.Fatal(err)
					}
				}
			}
		}
	}
}

func TestGeminiStreamPreservesFailureEvents(t *testing.T) {
	for _, prefix := range []string{"", "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"partial\"}]}}]}\n\n"} {
		for _, event := range []string{`{"promptFeedback":{"blockReason":"SAFETY","blockReasonMessage":"blocked message"}}`, `{"error":{"code":429,"status":"RESOURCE_EXHAUSTED","message":"quota message"}}`} {
			p := newGeminiTestProvider("text/event-stream", prefix+"data: "+event+"\n\n")
			var out bytes.Buffer
			err := p.StreamCall(context.Background(), CallRequest{}, &out)
			var failure *GeminiStreamError
			if !errors.As(err, &failure) {
				t.Fatalf("error = %v, want structured event", err)
			}
			if failure.Partial != (prefix != "") {
				t.Fatalf("partial = %t", failure.Partial)
			}
			if failure.BlockReason == "" && (failure.Code != 429 || failure.Status != "RESOURCE_EXHAUSTED") {
				t.Fatalf("error event = %+v", failure)
			}
			if !strings.Contains(failure.Message, "message") {
				t.Fatalf("lost message: %+v", failure)
			}
		}
	}
}

type remediationFailWriter struct{ err error }

func (w remediationFailWriter) Write([]byte) (int, error) { return 0, w.err }

func TestStreamDeltaPrecedesTerminalError(t *testing.T) {
	chat := `data: {"id":"test","object":"chat.completion.chunk","created":1,"model":"model","choices":[{"index":0,"delta":{"content":"partial"},"finish_reason":"length"}]}` + "\n\ndata: [DONE]\n\n"
	gemini := "data: " + `{"candidates":[{"content":{"parts":[{"text":"partial"}]},"finishReason":"MAX_TOKENS"}],"error":{"code":500,"message":"failure"}}` + "\n\n"
	for _, p := range []Provider{newChatTestProvider("omlx", "model", 32, staticResponseTransport("text/event-stream", chat)), newGeminiTestProvider("text/event-stream", gemini)} {
		var out bytes.Buffer
		err := p.StreamCall(context.Background(), CallRequest{Model: "model"}, &out)
		if out.String() != "partial" {
			t.Fatalf("output = %q", out.String())
		}
		if p.Name() == "omlx" {
			assertCompletionError(t, err, "length", true)
		}
		writeErr := errors.New("writer failed")
		if err := p.StreamCall(context.Background(), CallRequest{Model: "model"}, remediationFailWriter{writeErr}); !errors.Is(err, writeErr) {
			t.Fatalf("writer priority: %v", err)
		}
	}
}

func TestGeminiStreamScannerCapacity(t *testing.T) {
	for _, size := range []int{128 << 10, maxGeminiBodyBytes + 1} {
		text := strings.Repeat("a", size)
		body := "data: " + `{"candidates":[{"content":{"parts":[{"text":"` + text + `"}]},"finishReason":"STOP"}]}` + "\n\n"
		p := newGeminiTestProvider("text/event-stream", body)
		var out bytes.Buffer
		err := p.StreamCall(context.Background(), CallRequest{}, &out)
		if size < maxGeminiBodyBytes {
			if err != nil || out.Len() != size {
				t.Fatalf("large accepted event: len=%d err=%v", out.Len(), err)
			}
		} else if err == nil {
			t.Fatal("oversized event accepted")
		}
	}
}

func TestOpenAIOptionalOutputLimit(t *testing.T) {
	for _, limit := range []int{-1, 0, 32} {
		p := NewOpenAI("key", "model", "http://test", 0, limit).(*openAIProvider)
		body, err := json.Marshal(p.buildParams(CallRequest{Model: "model"}))
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]any
		if err := json.Unmarshal(body, &fields); err != nil {
			t.Fatal(err)
		}
		_, present := fields["max_output_tokens"]
		if present != (limit > 0) {
			t.Fatalf("limit %d: %s", limit, body)
		}
	}
}

func TestWindowsGcloudExtensionFixtures(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "Cloud SDK", "gcloud")
	if err := os.MkdirAll(filepath.Dir(base), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(base+".cmd", []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := resolveGcloudCandidate(base, "windows", ".EXE;.CMD"); got != base+".cmd" {
		t.Fatalf("candidate = %q", got)
	}
	if got := resolveGcloudCandidate(base+".cmd", "windows", ".EXE;.CMD"); got != base+".cmd" {
		t.Fatalf("explicit candidate = %q", got)
	}
	if got := resolveGcloudCandidate(base, "windows", ".EXE"); got != "" {
		t.Fatalf("ignored PATHEXT = %q", got)
	}
}
