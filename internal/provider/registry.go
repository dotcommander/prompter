package provider

import (
	"fmt"
	"sort"
	"strings"
)

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

// NewProvider constructs only the named registered provider.
func NewProvider(name string, rc RegistryConfig) (Provider, error) {
	for _, descriptor := range providerDescriptors {
		if descriptor.name == name {
			return constructProvider(descriptor, rc), nil
		}
	}
	return nil, fmt.Errorf("unknown provider %q (valid: %s)", name, KnownNamesString())
}

// NewRegistry builds a map of all providers keyed by name.
func NewRegistry(rc RegistryConfig) map[string]Provider {
	registry := make(map[string]Provider, len(providerDescriptors))
	for _, descriptor := range providerDescriptors {
		registry[descriptor.name] = constructProvider(descriptor, rc)
	}
	return registry
}

func constructProvider(descriptor providerDescriptor, rc RegistryConfig) Provider {
	settings := rc.Providers[descriptor.name]
	apiKey := normalizedAPIKey(settings.APIKey, descriptor.keyless)
	switch descriptor.transport {
	case transportOpenAI:
		return NewOpenAI(apiKey, settings.Model, settings.BaseURL, rc.MaxRetries, rc.MaxOutputTokens)
	case transportGemini:
		return NewGemini(apiKey, settings.ProjectID, settings.Location, settings.Model, settings.BaseURL, rc.MaxRetries, rc.MaxOutputTokens)
	default:
		return NewChat(descriptor.name, apiKey, settings.Model, settings.BaseURL, rc.MaxRetries, rc.MaxOutputTokens)
	}
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
