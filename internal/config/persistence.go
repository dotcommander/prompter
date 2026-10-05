package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"reflect"
	"strings"
)

// ConfigPatch records explicit assignments, including default-equal values and clears.
// Nil pointers leave persisted intent untouched.
type ConfigPatch struct {
	Provider        *string
	PromptFile      *string
	PromptsDir      *string
	PromptsDirs     *[]string
	ComponentsFile  *string
	Effort          *string
	Timeout         *int
	MaxOutputTokens *int
	MaxRetries      *int
	DefaultCopy     *bool
	Providers       map[string]ProviderPatch
}

// ProviderPatch records explicit per-provider assignments; nil pointers leave
// persisted intent untouched.
type ProviderPatch struct {
	KeyEnv    *string
	Model     *string
	BaseURL   *string
	ProjectID *string
	Location  *string
}

var configFields = map[string]string{
	"Provider": "provider", "PromptFile": "prompt_file", "PromptsDir": "prompts_dir", "PromptsDirs": "prompts_dirs", "ComponentsFile": "components_file", "Effort": "effort", "Timeout": "timeout", "MaxOutputTokens": "max_output_tokens", "MaxRetries": "max_retries", "DefaultCopy": "default_copy",
}
var providerFields = map[string]string{"KeyEnv": "key_env", "Model": "model", "BaseURL": "base_url", "ProjectID": "project_id", "Location": "location"}

// ApplyPatch updates runtime values and marks deliberate persistence intent.
func (cfg *Config) ApplyPatch(patch ConfigPatch) {
	if cfg.explicit == nil {
		cfg.explicit = map[string]any{}
	}
	value, target := reflect.ValueOf(patch), reflect.ValueOf(cfg).Elem()
	for field, key := range configFields {
		pointer := value.FieldByName(field)
		if !pointer.IsNil() {
			target.FieldByName(field).Set(pointer.Elem())
			cfg.explicit[key] = portableValue(key, pointer.Elem().Interface())
		}
	}
	if patch.MaxOutputTokens != nil {
		cfg.MaxOutputTokensExplicit = true
	}
	if cfg.Providers == nil {
		cfg.Providers = map[string]ProviderConfig{}
	}
	for name, p := range patch.Providers {
		provider := cfg.Providers[name]
		target, value := reflect.ValueOf(&provider).Elem(), reflect.ValueOf(p)
		fields := map[string]any{}
		for field, key := range providerFields {
			pointer := value.FieldByName(field)
			if !pointer.IsNil() {
				target.FieldByName(field).Set(pointer.Elem())
				fields[key] = pointer.Elem().Interface()
			}
		}
		cfg.Providers[name] = provider
		if len(fields) != 0 {
			existing, _ := cfg.explicit[name].(map[string]any)
			if existing == nil {
				existing = map[string]any{}
			}
			for key, value := range fields {
				existing[key] = value
			}
			cfg.explicit[name] = existing
		}
	}
}

func portableValue(key string, value any) any {
	switch key {
	case "prompt_file", "prompts_dir", "components_file":
		return unexpandPath(value.(string))
	case "prompts_dirs":
		return unexpandPaths(value.([]string))
	}
	return value
}

func (cfg *Config) snapshot() map[string]any {
	result := map[string]any{}
	value := reflect.ValueOf(cfg).Elem()
	for field, key := range configFields {
		result[key] = portableValue(key, value.FieldByName(field).Interface())
	}
	for name, provider := range cfg.Providers {
		fields := map[string]any{}
		value := reflect.ValueOf(provider)
		for field, key := range providerFields {
			fields[key] = value.FieldByName(field).Interface()
		}
		result[name] = fields
	}
	return result
}

func cloneIntent(source map[string]any) map[string]any {
	data, _ := json.Marshal(source)
	result := map[string]any{}
	_ = json.Unmarshal(data, &result)
	if result == nil {
		result = map[string]any{}
	}
	return result
}

func sameIntent(a, b any) bool {
	// JSON normalizes integer and decoded float values, and treats empty lists consistently.
	aa, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	return string(aa) == string(bb)
}

func (cfg *Config) persistenceIntent() map[string]any {
	result := cloneIntent(cfg.persisted)
	current := cfg.snapshot()
	for key, value := range current {
		fields, isProvider := value.(map[string]any)
		if !isProvider {
			if cfg.baseline != nil {
				if !sameIntent(value, cfg.baseline[key]) {
					result[key] = value
				}
			} else {
				// A directly constructed Config has no inherited values: retain every assignment,
				// including retry zero/false and explicitly empty lists. Omit absent strings/lists
				// and unset positive limits; explicit patches can assign any value.
				switch v := value.(type) {
				case string:
					if v != "" {
						result[key] = v
					}
				case []string:
					if cfg.PromptsDirs != nil {
						result[key] = v
					}
				case int:
					if v != 0 || key == "max_retries" {
						result[key] = v
					}
				default:
					result[key] = value
				}
			}
			continue
		}
		prior, _ := cfg.baseline[key].(map[string]any)
		persisted, _ := result[key].(map[string]any)
		if persisted == nil {
			persisted = map[string]any{}
		}
		for field, v := range fields {
			if cfg.baseline != nil {
				if !sameIntent(v, prior[field]) {
					persisted[field] = v
				}
			} else if v != "" {
				persisted[field] = v
			}
		}
		if len(persisted) != 0 {
			result[key] = persisted
		}
	}
	for key, value := range cfg.explicit {
		if fields, ok := value.(map[string]any); ok {
			target, _ := result[key].(map[string]any)
			if target == nil {
				target = map[string]any{}
			}
			for field, v := range fields {
				target[field] = v
			}
			result[key] = target
		} else {
			result[key] = value
		}
	}
	// API keys are never persisted, including existing keys and unknown provider blocks.
	for _, value := range result {
		if fields, ok := value.(map[string]any); ok {
			delete(fields, "api_key")
		}
	}
	return result
}

// Origin reports resolution provenance separately from the value itself.
func (cfg *Config) Origin(field string) string { return cfg.origins[field] }

// CredentialStatus never exposes a credential value. ADC discovery remains offline
// and cannot establish whether credentials are currently usable.
type CredentialStatus struct {
	Source    string
	Available bool
	Unchecked bool
}

// CredentialStatus reports whether the named provider has a usable credential
// and names its resolved source without exposing the value.
func (cfg *Config) CredentialStatus(name string) CredentialStatus {
	provider := cfg.Providers[name]
	if name == "omlx" {
		return CredentialStatus{Source: "not required", Available: true}
	}
	if name == "gemini" {
		endpoint, _ := url.Parse(provider.BaseURL)
		if endpoint == nil || !strings.EqualFold(endpoint.Hostname(), "generativelanguage.googleapis.com") {
			return CredentialStatus{Source: "application default credentials (unchecked)", Unchecked: true}
		}
	}
	if provider.APIKey == "" {
		return CredentialStatus{Source: "not set"}
	}
	if source := cfg.origins[name+".api_key"]; source != "" && source != "built-in default" {
		return CredentialStatus{Source: source, Available: true}
	}
	upper := strings.ToUpper(name)
	for _, env := range []string{"PROMPTER_" + upper + "_API_KEY", provider.KeyEnv, upper + "_API_KEY"} {
		if value := os.Getenv(env); value != "" && value == provider.APIKey {
			return CredentialStatus{Source: "environment: " + safeEnvName(env), Available: true}
		}
	}
	return CredentialStatus{Source: "configured API key", Available: true}
}

// ValidateKeyEnv rejects values rather than echoing rejected input in its error.
func ValidateKeyEnv(value string) error {
	if value == "" {
		return nil
	}
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "sk-") || strings.HasPrefix(value, "AIza") || strings.HasPrefix(lower, "gsk_") || strings.HasPrefix(lower, "sk_") {
		return fmt.Errorf("enter an environment variable name, not a credential")
	}
	for index, r := range value {
		if !(r == '_' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || index > 0 && r >= '0' && r <= '9') {
			return fmt.Errorf("environment variable name must use letters, digits, and underscores and cannot start with a digit")
		}
	}
	return nil
}

// SafeKeyEnv safely displays legacy selectors without disclosing credential-shaped input.
func SafeKeyEnv(value string) string {
	if ValidateKeyEnv(value) != nil {
		return "[invalid environment variable name]"
	}
	return value
}
func safeEnvName(value string) string { return SafeKeyEnv(value) }
