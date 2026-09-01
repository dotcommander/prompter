package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dotcommander/prompter/internal/config"
)

const (
	modelsDevURL          = "https://models.dev/api.json"
	openRouterModelsURL   = "https://openrouter.ai/api/v1/models?sort=top-weekly&limit=1000"
	modelCatalogSchema    = 2
	modelCatalogMaxBytes  = 32 << 20
	modelOutputPriceLimit = 15.0
	modelPreferredPrice   = 5.0
	modelCatalogLimit     = 5
)

var modelsDevProviders = map[string]string{
	"gemini":     "google",
	"openai":     "openai",
	"groq":       "groq",
	"cerebras":   "cerebras",
	"deepseek":   "deepseek",
	"openrouter": "openrouter",
	"zai":        "zai",
}

type modelsDevCost struct {
	Input  *float64 `json:"input"`
	Output *float64 `json:"output"`
}

type modelsDevLimit struct {
	Context int `json:"context"`
	Output  int `json:"output"`
}

type modelsDevModel struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Family      string         `json:"family"`
	ReleaseDate string         `json:"release_date"`
	LastUpdated string         `json:"last_updated"`
	Cost        modelsDevCost  `json:"cost"`
	Limit       modelsDevLimit `json:"limit"`
}

type modelsDevProvider struct {
	Models map[string]modelsDevModel `json:"models"`
}

type catalogModel struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	ReleaseDate string  `json:"release_date,omitempty"`
	InputCPM    float64 `json:"input_cpm,omitempty"`
	OutputCPM   float64 `json:"output_cpm,omitempty"`
	Local       bool    `json:"local,omitempty"`
}

type modelCatalogCache struct {
	SchemaVersion    int                       `json:"schema_version"`
	Source           string                    `json:"source"`
	OpenRouterSource string                    `json:"openrouter_source"`
	FetchedAt        time.Time                 `json:"fetched_at"`
	Providers        map[string][]catalogModel `json:"providers"`
}

type modelCatalogService struct {
	client              *http.Client
	sourceURL           string
	openRouterSourceURL string
	cachePath           string
}

func newModelCatalogService() (*modelCatalogService, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home for model catalog: %w", err)
	}
	return &modelCatalogService{
		client:              &http.Client{Timeout: 15 * time.Second},
		sourceURL:           modelsDevURL,
		openRouterSourceURL: openRouterModelsURL,
		cachePath:           filepath.Join(home, ".config", "prompter", "models-dev.json"),
	}, nil
}

func (s *modelCatalogService) loadOrFetch(ctx context.Context, cfg *config.Config) (modelCatalogCache, bool, error) {
	cache, err := s.load()
	if err == nil {
		return cache, true, nil
	}
	if !os.IsNotExist(err) {
		return modelCatalogCache{}, false, fmt.Errorf("load model catalog cache: %w", err)
	}
	cache, err = s.refresh(ctx, cfg)
	return cache, false, err
}

func (s *modelCatalogService) refresh(ctx context.Context, cfg *config.Config) (modelCatalogCache, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.sourceURL, nil)
	if err != nil {
		return modelCatalogCache{}, fmt.Errorf("create Models.dev request: %w", err)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return modelCatalogCache{}, fmt.Errorf("fetch Models.dev catalog: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		return modelCatalogCache{}, fmt.Errorf("fetch Models.dev catalog: %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, modelCatalogMaxBytes+1))
	if err != nil {
		return modelCatalogCache{}, fmt.Errorf("read Models.dev catalog: %w", err)
	}
	if len(body) > modelCatalogMaxBytes {
		return modelCatalogCache{}, fmt.Errorf("models.dev catalog exceeds %d bytes", modelCatalogMaxBytes)
	}
	var providers map[string]modelsDevProvider
	if err := json.Unmarshal(body, &providers); err != nil {
		return modelCatalogCache{}, fmt.Errorf("decode Models.dev catalog: %w", err)
	}

	openRouterOrder, err := s.fetchOpenRouterWeekly(ctx)
	if err != nil {
		return modelCatalogCache{}, err
	}

	cache := modelCatalogCache{
		SchemaVersion:    modelCatalogSchema,
		Source:           s.sourceURL,
		OpenRouterSource: s.openRouterSourceURL,
		FetchedAt:        time.Now().UTC(),
		Providers:        make(map[string][]catalogModel, len(modelsDevProviders)+1),
	}
	for prompterName, modelsDevName := range modelsDevProviders {
		providerData, ok := providers[modelsDevName]
		if !ok {
			return modelCatalogCache{}, fmt.Errorf("models.dev catalog missing provider %q", modelsDevName)
		}
		if prompterName == "openrouter" {
			cache.Providers[prompterName] = selectCatalogModelsByOrder(providerData.Models, openRouterOrder, true)
			continue
		}
		applyOutputPriceLimit := prompterName != "cerebras" && prompterName != "groq"
		cache.Providers[prompterName] = selectCatalogModels(providerData.Models, applyOutputPriceLimit)
	}
	if local := fetchOmlxModels(ctx, s.client, cfg.Providers["omlx"].BaseURL); len(local) > 0 {
		cache.Providers["omlx"] = local
	}
	if err := s.save(cache); err != nil {
		return modelCatalogCache{}, err
	}
	return cache, nil
}

func (s *modelCatalogService) load() (modelCatalogCache, error) {
	data, err := os.ReadFile(s.cachePath)
	if err != nil {
		return modelCatalogCache{}, err
	}
	var cache modelCatalogCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return modelCatalogCache{}, err
	}
	if cache.SchemaVersion != modelCatalogSchema || cache.Source != s.sourceURL || cache.OpenRouterSource != s.openRouterSourceURL || cache.Providers == nil {
		return modelCatalogCache{}, fmt.Errorf("incompatible model catalog cache")
	}
	return cache, nil
}

func (s *modelCatalogService) fetchOpenRouterWeekly(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.openRouterSourceURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create OpenRouter models request: %w", err)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch OpenRouter weekly models: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		return nil, fmt.Errorf("fetch OpenRouter weekly models: %s", resp.Status)
	}
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, modelCatalogMaxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read OpenRouter weekly models: %w", err)
	}
	if len(body) > modelCatalogMaxBytes {
		return nil, fmt.Errorf("OpenRouter weekly models response exceeds %d bytes", modelCatalogMaxBytes)
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode OpenRouter weekly models: %w", err)
	}
	order := make([]string, 0, len(payload.Data))
	for _, model := range payload.Data {
		if id := strings.TrimSpace(model.ID); id != "" {
			order = append(order, id)
		}
	}
	if len(order) == 0 {
		return nil, fmt.Errorf("OpenRouter weekly models response is empty")
	}
	return order, nil
}

func (s *modelCatalogService) save(cache modelCatalogCache) error {
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return fmt.Errorf("encode model catalog cache: %w", err)
	}
	dir := filepath.Dir(s.cachePath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create model catalog cache directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".models-dev-*.tmp")
	if err != nil {
		return fmt.Errorf("create model catalog cache: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("secure model catalog cache: %w", err)
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write model catalog cache: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close model catalog cache: %w", err)
	}
	if err := os.Rename(tmpPath, s.cachePath); err != nil {
		return fmt.Errorf("replace model catalog cache: %w", err)
	}
	return nil
}

func selectCatalogModels(models map[string]modelsDevModel, applyOutputPriceLimit bool) []catalogModel {
	candidates := eligibleCatalogModels(models, applyOutputPriceLimit)
	sort.Slice(candidates, func(i, j int) bool {
		iPreferred := *candidates[i].Cost.Output <= modelPreferredPrice
		jPreferred := *candidates[j].Cost.Output <= modelPreferredPrice
		if iPreferred != jPreferred {
			return iPreferred
		}
		if candidates[i].ReleaseDate != candidates[j].ReleaseDate {
			return candidates[i].ReleaseDate > candidates[j].ReleaseDate
		}
		if candidates[i].LastUpdated != candidates[j].LastUpdated {
			return candidates[i].LastUpdated > candidates[j].LastUpdated
		}
		return candidates[i].ID < candidates[j].ID
	})
	return catalogModels(candidates)
}

func selectCatalogModelsByOrder(models map[string]modelsDevModel, order []string, applyOutputPriceLimit bool) []catalogModel {
	eligible := eligibleCatalogModels(models, applyOutputPriceLimit)
	byID := make(map[string]modelsDevModel, len(eligible))
	for _, model := range eligible {
		byID[model.ID] = model
	}
	preferred := make([]modelsDevModel, 0, modelCatalogLimit)
	fallback := make([]modelsDevModel, 0, modelCatalogLimit)
	seen := make(map[string]bool)
	for _, id := range order {
		model, ok := byID[id]
		if !ok || seen[id] {
			continue
		}
		seen[id] = true
		if *model.Cost.Output <= modelPreferredPrice {
			preferred = append(preferred, model)
		} else {
			fallback = append(fallback, model)
		}
	}
	return catalogModels(append(preferred, fallback...))
}

func eligibleCatalogModels(models map[string]modelsDevModel, applyOutputPriceLimit bool) []modelsDevModel {
	candidates := make([]modelsDevModel, 0, len(models))
	for key, model := range models {
		if model.ID == "" {
			model.ID = key
		}
		if model.ReleaseDate == "" || model.Cost.Output == nil {
			continue
		}
		if applyOutputPriceLimit && *model.Cost.Output >= modelOutputPriceLimit {
			continue
		}
		if _, err := time.Parse(time.DateOnly, model.ReleaseDate); err != nil {
			continue
		}
		candidates = append(candidates, model)
	}
	return shrinkAliases(candidates)
}

func catalogModels(candidates []modelsDevModel) []catalogModel {
	if len(candidates) > modelCatalogLimit {
		candidates = candidates[:modelCatalogLimit]
	}
	selected := make([]catalogModel, 0, len(candidates))
	for _, model := range candidates {
		input := 0.0
		if model.Cost.Input != nil {
			input = *model.Cost.Input
		}
		selected = append(selected, catalogModel{
			ID: model.ID, Name: model.Name, ReleaseDate: model.ReleaseDate,
			InputCPM: input, OutputCPM: *model.Cost.Output,
		})
	}
	return selected
}

func shrinkAliases(models []modelsDevModel) []modelsDevModel {
	nonAliases := make(map[string]bool)
	for _, model := range models {
		if !isModelAlias(model.ID) {
			nonAliases[modelFingerprint(model)] = true
		}
	}
	result := make([]modelsDevModel, 0, len(models))
	for _, model := range models {
		if isModelAlias(model.ID) && nonAliases[modelFingerprint(model)] {
			continue
		}
		result = append(result, model)
	}
	return result
}

func isModelAlias(id string) bool {
	return strings.HasPrefix(id, "~") || strings.HasSuffix(id, "-latest")
}

func modelFingerprint(model modelsDevModel) string {
	input, output := 0.0, 0.0
	if model.Cost.Input != nil {
		input = *model.Cost.Input
	}
	if model.Cost.Output != nil {
		output = *model.Cost.Output
	}
	return fmt.Sprintf("%s|%s|%.6f|%.6f|%d|%d", model.Family, model.ReleaseDate, input, output, model.Limit.Context, model.Limit.Output)
}

func fetchOmlxModels(ctx context.Context, client *http.Client, baseURL string) []catalogModel {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil
	}
	reqCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, baseURL+"/models", nil)
	if err != nil {
		return nil
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		return nil
	}
	ids := make([]string, 0, len(payload.Data))
	for _, model := range payload.Data {
		if strings.TrimSpace(model.ID) != "" {
			ids = append(ids, model.ID)
		}
	}
	sort.Strings(ids)
	if len(ids) > modelCatalogLimit {
		ids = ids[:modelCatalogLimit]
	}
	models := make([]catalogModel, 0, len(ids))
	for _, id := range ids {
		models = append(models, catalogModel{ID: id, Name: id, Local: true})
	}
	return models
}

func catalogModelChoices(cache modelCatalogCache) map[string][]modelChoice {
	choices := make(map[string][]modelChoice, len(cache.Providers))
	for providerName, models := range cache.Providers {
		for _, model := range models {
			label := model.Name
			if label == "" {
				label = model.ID
			}
			if model.Local {
				label += " (local; pricing/date unavailable)"
			} else {
				label += fmt.Sprintf(" ($%.2f/M output; released %s)", model.OutputCPM, model.ReleaseDate)
			}
			choices[providerName] = append(choices[providerName], modelChoice{id: model.ID, label: label})
		}
	}
	return choices
}

func printModelCatalog(w io.Writer, cache modelCatalogCache) {
	fmt.Fprintf(w, "Models.dev catalog fetched %s\n", cache.FetchedAt.Format(time.RFC3339))
	for _, providerName := range []string{"gemini", "openai", "groq", "cerebras", "deepseek", "openrouter", "zai", "omlx"} {
		fmt.Fprintf(w, "\n%s\n", strings.ToUpper(providerName))
		models := cache.Providers[providerName]
		if len(models) == 0 {
			fmt.Fprintln(w, "  unavailable; embedded/configured models retained")
			continue
		}
		for _, model := range models {
			if model.Local {
				fmt.Fprintf(w, "  %-42s local; pricing/date unavailable\n", model.ID)
			} else {
				fmt.Fprintf(w, "  %-42s %s  $%.2f/M output\n", model.ID, model.ReleaseDate, model.OutputCPM)
			}
		}
	}
}
