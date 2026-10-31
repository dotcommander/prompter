package provider

import (
	"fmt"
	"reflect"
	"testing"
)

func TestNewProviderMatchesRegistry(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		rc   RegistryConfig
	}{
		{name: "unset", rc: RegistryConfig{}},
		{name: "configured", rc: RegistryConfig{
			MaxRetries: 4, MaxOutputTokens: 987,
			Providers: map[string]ProviderSettings{
				"openai":     {APIKey: "openai-key", Model: "openai-model", BaseURL: "https://openai.example/v1"},
				"cerebras":   {APIKey: "cerebras-key", Model: "cerebras-model", BaseURL: "https://cerebras.example/v1"},
				"deepseek":   {APIKey: "deepseek-key", Model: "deepseek-model", BaseURL: "https://deepseek.example/v1"},
				"groq":       {APIKey: "groq-key", Model: "groq-model", BaseURL: "https://groq.example/v1"},
				"openrouter": {APIKey: "openrouter-key", Model: "openrouter-model", BaseURL: "https://openrouter.example/v1"},
				"zai":        {APIKey: "zai-key", Model: "zai-model", BaseURL: "https://zai.example/v1"},
				"gemini":     {APIKey: "gemini-key", Model: "gemini-model", BaseURL: "https://gemini.example/v1/", ProjectID: "project-override", Location: "us-central1"},
				"omlx":       {Model: "omlx-model", BaseURL: "http://localhost:8000/v1"},
			},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			registry := NewRegistry(tc.rc)
			if len(registry) != len(KnownNames()) {
				t.Fatalf("registry size = %d, want %d", len(registry), len(KnownNames()))
			}
			for _, name := range KnownNames() {
				t.Run(name, func(t *testing.T) {
					t.Parallel()
					selected, err := NewProvider(name, tc.rc)
					if err != nil {
						t.Fatalf("NewProvider(%q): %v", name, err)
					}
					all := registry[name]
					if reflect.TypeOf(selected) != reflect.TypeOf(all) || selected.Name() != all.Name() || selected.Model() != all.Model() || selected.APIKey() != all.APIKey() {
						t.Fatalf("selected %T (%q, %q, %q) differs from registry %T (%q, %q, %q)", selected, selected.Name(), selected.Model(), selected.APIKey(), all, all.Name(), all.Model(), all.APIKey())
					}
					switch got := selected.(type) {
					case *openAIProvider:
						want := all.(*openAIProvider)
						if got.baseURL != want.baseURL || got.baseURL != tc.rc.Providers[name].BaseURL || got.maxOutputTokens != want.maxOutputTokens || got.maxOutputTokens != tc.rc.MaxOutputTokens || got.client == nil {
							t.Fatal("openai endpoint, token limit, or client differs")
						}
					case *geminiProvider:
						want := all.(*geminiProvider)
						if got.projectID != want.projectID || got.location != want.location || got.model != want.model || got.baseURL != want.baseURL || got.maxOutputTokens != want.maxOutputTokens || got.maxOutputTokens != tc.rc.MaxOutputTokens || got.client == nil || got.tokenResolver == nil {
							t.Fatal("gemini endpoint, project, location, model, token limit, or client differs")
						}
						if settings := tc.rc.Providers[name]; settings.BaseURL != "" && (got.baseURL != "https://gemini.example/v1" || got.projectID != settings.ProjectID || got.location != settings.Location || got.model != settings.Model) {
							t.Fatal("gemini configured overrides not applied")
						}
					case *chatProvider:
						want := all.(*chatProvider)
						if got.maxOutputTokens != want.maxOutputTokens || got.maxOutputTokens != tc.rc.MaxOutputTokens || got.client == nil {
							t.Fatal("chat token limit or client differs")
						}
					default:
						t.Fatalf("unexpected provider type %T", selected)
					}
				})
			}
		})
	}
}

func TestNewProviderUnknownName(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"", "missing", "OpenAI"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := NewProvider(name, RegistryConfig{})
			if got != nil {
				t.Fatalf("NewProvider(%q) = %T, want nil", name, got)
			}
			want := fmt.Sprintf("unknown provider %q (valid: %s)", name, KnownNamesString())
			if err == nil || err.Error() != want {
				t.Fatalf("NewProvider(%q) error = %v, want %q", name, err, want)
			}
		})
	}
}

func TestNewProviderKeylessNormalization(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, key, want string
	}{
		{name: "omlx", want: "local"},
		{name: "omlx", key: "explicit", want: "explicit"},
		{name: "openai", want: ""},
		{name: "gemini", want: "adc"},
	} {
		t.Run(tc.name+"/"+tc.key, func(t *testing.T) {
			t.Parallel()
			selected, err := NewProvider(tc.name, RegistryConfig{Providers: map[string]ProviderSettings{tc.name: {APIKey: tc.key}}})
			if err != nil {
				t.Fatal(err)
			}
			if got := selected.APIKey(); got != tc.want {
				t.Fatalf("APIKey = %q, want %q", got, tc.want)
			}
		})
	}
}
