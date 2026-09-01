package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dotcommander/prompter/internal/config"
)

func catalogPrice(value float64) *float64 { return &value }

type catalogRoundTripper func(*http.Request) (*http.Response, error)

func (f catalogRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestSelectCatalogModelsPrefersAffordableRecentModels(t *testing.T) {
	t.Parallel()
	models := map[string]modelsDevModel{
		"cheap-new": {ID: "cheap-new", Name: "Cheap New", Family: "cheap", ReleaseDate: "2026-08-01", Cost: modelsDevCost{Output: catalogPrice(4)}},
		"cheap-old": {ID: "cheap-old", Name: "Cheap Old", Family: "old", ReleaseDate: "2026-07-01", Cost: modelsDevCost{Output: catalogPrice(3)}},
		"premium":   {ID: "premium", Name: "Premium", Family: "premium", ReleaseDate: "2026-08-30", Cost: modelsDevCost{Output: catalogPrice(6)}},
		"excluded":  {ID: "excluded", Name: "Excluded", Family: "excluded", ReleaseDate: "2026-08-31", Cost: modelsDevCost{Output: catalogPrice(15)}},
	}

	got := selectCatalogModels(models, true)
	want := []string{"cheap-new", "cheap-old", "premium"}
	if ids := catalogModelIDs(got); !reflect.DeepEqual(ids, want) {
		t.Fatalf("selected IDs = %v, want %v", ids, want)
	}
}

func TestSelectCatalogModelsByOrderPreservesWeeklyRankWithinPriceTier(t *testing.T) {
	t.Parallel()
	models := map[string]modelsDevModel{
		"weekly-first":  {ID: "weekly-first", Family: "first", ReleaseDate: "2026-01-01", Cost: modelsDevCost{Output: catalogPrice(2)}},
		"weekly-second": {ID: "weekly-second", Family: "second", ReleaseDate: "2026-08-01", Cost: modelsDevCost{Output: catalogPrice(4)}},
		"premium":       {ID: "premium", Family: "premium", ReleaseDate: "2026-08-30", Cost: modelsDevCost{Output: catalogPrice(8)}},
	}
	order := []string{"premium", "weekly-first", "missing", "weekly-second"}

	got := selectCatalogModelsByOrder(models, order, true)
	want := []string{"weekly-first", "weekly-second", "premium"}
	if ids := catalogModelIDs(got); !reflect.DeepEqual(ids, want) {
		t.Fatalf("selected IDs = %v, want %v", ids, want)
	}
}

func TestModelCatalogRefreshKeepsExpensiveCerebrasAndGroqModels(t *testing.T) {
	t.Parallel()
	providerModels := map[string]modelsDevProvider{}
	for _, modelsDevName := range modelsDevProviders {
		providerModels[modelsDevName] = modelsDevProvider{Models: map[string]modelsDevModel{
			"affordable": {ID: "affordable", Family: modelsDevName, ReleaseDate: "2026-08-01", Cost: modelsDevCost{Output: catalogPrice(1)}},
		}}
	}
	providerModels["cerebras"] = modelsDevProvider{Models: map[string]modelsDevModel{
		"premium-cerebras": {ID: "premium-cerebras", Family: "cerebras", ReleaseDate: "2026-08-01", Cost: modelsDevCost{Output: catalogPrice(20)}},
		"missing-price":    {ID: "missing-price", Family: "cerebras", ReleaseDate: "2026-08-02"},
		"invalid-date":     {ID: "invalid-date", Family: "cerebras", ReleaseDate: "not-a-date", Cost: modelsDevCost{Output: catalogPrice(20)}},
	}}
	providerModels["groq"] = modelsDevProvider{Models: map[string]modelsDevModel{
		"premium-groq": {ID: "premium-groq", Family: "groq", ReleaseDate: "2026-08-01", Cost: modelsDevCost{Output: catalogPrice(20)}},
	}}
	providerModels["openai"] = modelsDevProvider{Models: map[string]modelsDevModel{
		"premium-openai": {ID: "premium-openai", Family: "openai", ReleaseDate: "2026-08-01", Cost: modelsDevCost{Output: catalogPrice(20)}},
	}}
	modelsDevJSON, err := json.Marshal(providerModels)
	if err != nil {
		t.Fatalf("marshal Models.dev fixture: %v", err)
	}
	client := &http.Client{Transport: catalogRoundTripper(func(req *http.Request) (*http.Response, error) {
		var body []byte
		switch req.URL.Host {
		case "models.test":
			body = modelsDevJSON
		case "openrouter.test":
			body = []byte(`{"data":[{"id":"affordable"}]}`)
		case "127.0.0.1:8000":
			return &http.Response{
				StatusCode: http.StatusServiceUnavailable,
				Status:     "503 Service Unavailable",
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("")),
				Request:    req,
			}, nil
		default:
			t.Fatalf("unexpected request URL: %s", req.URL)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(string(body))),
			Request:    req,
		}, nil
	})}
	service := &modelCatalogService{
		client:              client,
		sourceURL:           "https://models.test/catalog",
		openRouterSourceURL: "https://openrouter.test/models?sort=top-weekly",
		cachePath:           filepath.Join(t.TempDir(), "catalog.json"),
	}
	cache, err := service.refresh(context.Background(), &config.Config{Providers: config.DefaultProviders()})
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if got, want := catalogModelIDs(cache.Providers["cerebras"]), []string{"premium-cerebras"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Cerebras IDs = %v, want %v", got, want)
	}
	if got, want := catalogModelIDs(cache.Providers["groq"]), []string{"premium-groq"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Groq IDs = %v, want %v", got, want)
	}
	if got := cache.Providers["openai"]; len(got) != 0 {
		t.Errorf("OpenAI models = %v, want expensive model excluded", catalogModelIDs(got))
	}
}

func TestModelCatalogRefreshUsesOpenRouterWeeklyOrder(t *testing.T) {
	t.Parallel()
	providerModels := map[string]modelsDevProvider{}
	for _, modelsDevName := range modelsDevProviders {
		providerModels[modelsDevName] = modelsDevProvider{Models: map[string]modelsDevModel{
			"model": {ID: "model", Name: "Model", Family: modelsDevName, ReleaseDate: "2026-08-01", Cost: modelsDevCost{Output: catalogPrice(1)}},
		}}
	}
	providerModels["openrouter"] = modelsDevProvider{Models: map[string]modelsDevModel{
		"vendor/first":  {ID: "vendor/first", Name: "First", Family: "first", ReleaseDate: "2026-07-01", Cost: modelsDevCost{Output: catalogPrice(2)}},
		"vendor/second": {ID: "vendor/second", Name: "Second", Family: "second", ReleaseDate: "2026-08-01", Cost: modelsDevCost{Output: catalogPrice(3)}},
	}}
	modelsDevJSON, err := json.Marshal(providerModels)
	if err != nil {
		t.Fatalf("marshal Models.dev fixture: %v", err)
	}
	client := &http.Client{Transport: catalogRoundTripper(func(req *http.Request) (*http.Response, error) {
		var body []byte
		switch req.URL.Host {
		case "models.test":
			body = modelsDevJSON
		case "openrouter.test":
			if got := req.URL.Query().Get("sort"); got != "top-weekly" {
				t.Errorf("OpenRouter sort = %q, want top-weekly", got)
			}
			body = []byte(`{"data":[{"id":"vendor/second"},{"id":"vendor/first"}]}`)
		case "127.0.0.1:8000":
			return &http.Response{
				StatusCode: http.StatusServiceUnavailable,
				Status:     "503 Service Unavailable",
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("")),
				Request:    req,
			}, nil
		default:
			t.Fatalf("unexpected request URL: %s", req.URL)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(string(body))),
			Request:    req,
		}, nil
	})}

	service := &modelCatalogService{
		client:              client,
		sourceURL:           "https://models.test/catalog",
		openRouterSourceURL: "https://openrouter.test/models?sort=top-weekly",
		cachePath:           filepath.Join(t.TempDir(), "catalog.json"),
	}
	cache, err := service.refresh(context.Background(), &config.Config{Providers: config.DefaultProviders()})
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if got, want := catalogModelIDs(cache.Providers["openrouter"]), []string{"vendor/second", "vendor/first"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("OpenRouter IDs = %v, want %v", got, want)
	}
	loaded, err := service.load()
	if err != nil {
		t.Fatalf("load saved cache: %v", err)
	}
	if !reflect.DeepEqual(loaded.Providers["openrouter"], cache.Providers["openrouter"]) {
		t.Fatalf("loaded OpenRouter models = %#v, want %#v", loaded.Providers["openrouter"], cache.Providers["openrouter"])
	}
}

func catalogModelIDs(models []catalogModel) []string {
	ids := make([]string, len(models))
	for i, model := range models {
		ids[i] = model.ID
	}
	return ids
}
