package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/huh/v2"
	"github.com/dotcommander/prompter/internal/config"
)

func defaultKeyEnvFor(p string) string {
	switch p {
	case "gemini":
		return "GEMINI_API_KEY"
	case "openai":
		return "OPENAI_API_KEY"
	case "groq":
		return "GROQ_API_KEY"
	case "cerebras":
		return "CEREBRAS_API_KEY"
	case "deepseek":
		return "DEEPSEEK_API_KEY"
	case "openrouter":
		return "OPENROUTER_API_KEY"
	case "zai":
		return "ZAI_API_KEY"
	case "omlx":
		return "OMLX_API_KEY"
	default:
		return strings.ToUpper(p) + "_API_KEY"
	}
}

func defaultModelFor(p string) string {
	defaults := config.DefaultProviders()
	if def, ok := defaults[p]; ok && def.Model != "" {
		return def.Model
	}
	return ""
}

func defaultBaseURLFor(p string) string {
	defaults := config.DefaultProviders()
	if def, ok := defaults[p]; ok && def.BaseURL != "" {
		return def.BaseURL
	}
	return ""
}

func isProviderConfigured(p string, cfg *config.Config) (bool, string) {
	if p == "omlx" {
		return true, "local loopback / keyless"
	}

	pCfg := cfg.Providers[p]
	keyVar := pCfg.KeyEnv
	if keyVar == "" {
		keyVar = defaultKeyEnvFor(p)
	}

	if val := os.Getenv(keyVar); val != "" {
		return true, fmt.Sprintf("$%s detected", keyVar)
	}
	if pCfg.APIKey != "" {
		return true, "API key in config"
	}
	if p == "gemini" {
		if os.Getenv("GEMINI_API_KEY") != "" {
			return true, "$GEMINI_API_KEY detected"
		}
		if os.Getenv("PROMPTER_GEMINI_API_KEY") != "" {
			return true, "$PROMPTER_GEMINI_API_KEY detected"
		}
		if os.Getenv("GOOGLE_APPLICATION_CREDENTIALS") != "" {
			return false, "$GOOGLE_APPLICATION_CREDENTIALS set; ADC validity not checked"
		}
		return false, "ADC availability not checked"
	}
	return false, "key not found"
}

type modelChoice struct {
	id    string
	label string
}

func popularModelsFor(p string, discovered ...map[string][]modelChoice) []modelChoice {
	if len(discovered) > 0 && len(discovered[0][p]) > 0 {
		return discovered[0][p]
	}
	switch p {
	case "gemini":
		return []modelChoice{
			{"gemini-3.7-flash", "gemini-3.7-flash (Default / Fast Hybrid Reasoning)"},
		}
	case "openai":
		return []modelChoice{
			{"gpt-5.6-luna", "gpt-5.6-luna (Default Flagship)"},
		}
	case "groq":
		return []modelChoice{
			{"qwen/qwen3.8-27b", "qwen/qwen3.8-27b (Default / Latest 27B)"},
			{"qwen/qwen3.6-27b", "qwen/qwen3.6-27b (Previous 27B)"},
		}
	case "cerebras":
		return []modelChoice{
			{"gpt-oss-120b", "gpt-oss-120b (Default)"},
			{"gemma-4-31b", "gemma-4-31b"},
		}
	case "deepseek":
		return []modelChoice{
			{"deepseek-v4-pro", "deepseek-v4-pro (Default)"},
			{"deepseek-v4-flash", "deepseek-v4-flash"},
			{"deepseek-v4-flash-vision-exp", "deepseek-v4-flash-vision-exp (Experimental Vision)"},
		}
	case "openrouter":
		return []modelChoice{
			{"openrouter/free", "openrouter/free (Default / Free Router Tier)"},
			{"anthropic/claude-sonnet-5", "anthropic/claude-sonnet-5 (Latest Sonnet)"},
			{"meta-llama/llama-3.3-70b-instruct", "meta-llama/llama-3.3-70b-instruct (Llama 3.3 70B)"},
		}
	case "zai":
		return []modelChoice{
			{"glm-5.3-flash", "glm-5.3-flash (Default / High Speed)"},
			{"glm-5.3", "glm-5.3 (Latest Flagship)"},
		}
	case "omlx":
		return []modelChoice{
			{"Ornith-1.5-35B-A3B-oQ4e-mtp", "Ornith-1.5-35B-A3B-oQ4e-mtp (Default / Apple MLX)"},
			{"Qwen2.5-Coder-7B-Instruct-4bit", "Qwen2.5-Coder-7B-Instruct-4bit (Coding Optimized)"},
			{"Llama-3.2-3B-Instruct-4bit", "Llama-3.2-3B-Instruct-4bit (Compact 3B)"},
		}
	default:
		return nil
	}
}

// RunConfigForm launches an interactive TUI form to configure prompter settings.
func RunConfigForm(cfg *config.Config, discovered ...map[string][]modelChoice) error {
	selectedProvider := cfg.Provider
	if selectedProvider == "" {
		selectedProvider = "gemini"
	}

	effort := cfg.Effort
	if effort == "" {
		effort = "low"
	}
	defaultCopy := cfg.DefaultCopy

	type providerEntry struct {
		id   string
		name string
	}

	providersList := []providerEntry{
		{"gemini", "Google Gemini (ADC / Vertex AI / AI Studio)"},
		{"openai", "OpenAI (Responses API)"},
		{"groq", "Groq (Fast Cloud Inference)"},
		{"cerebras", "Cerebras (Fast Cloud Inference)"},
		{"deepseek", "DeepSeek"},
		{"openrouter", "OpenRouter (Multi-model Router)"},
		{"zai", "Zai (Zhipu AI)"},
		{"omlx", "OMLX (Local MLX Server)"},
	}

	var firstDetected string
	providerOptions := make([]huh.Option[string], len(providersList))

	for i, prov := range providersList {
		configured, detail := isProviderConfigured(prov.id, cfg)
		var label string
		if configured {
			label = fmt.Sprintf("%-48s [✓ %s]", prov.name, detail)
			if firstDetected == "" && prov.id != "gemini" {
				firstDetected = prov.id
			}
		} else {
			if detail != "key not found" {
				label = fmt.Sprintf("%-48s [? %s]", prov.name, detail)
			} else {
				keyVar := cfg.Providers[prov.id].KeyEnv
				if keyVar == "" {
					keyVar = defaultKeyEnvFor(prov.id)
				}
				label = fmt.Sprintf("%-48s [✗ $%s not set]", prov.name, keyVar)
			}
		}
		providerOptions[i] = huh.NewOption(label, prov.id)
	}

	if selectedProvider == "" && firstDetected != "" {
		selectedProvider = firstDetected
	}

	effortOptions := []huh.Option[string]{
		huh.NewOption("Low (Fastest — direct, low-latency generation)", "low"),
		huh.NewOption("Medium (Balanced — balanced thinking & reasoning)", "medium"),
		huh.NewOption("High (Deep — multi-step reasoning for complex tasks)", "high"),
	}

	keyMap := huh.NewDefaultKeyMap()
	keyMap.Quit = key.NewBinding(key.WithKeys("ctrl+c", "q"))

	// =========================================================================
	// STEP 1: Provider Selection
	// =========================================================================
	providerSelect := huh.NewSelect[string]().
		Title("Active AI Provider").
		Description("Select your default LLM backend (use ↑/↓ to choose, ENTER to continue):").
		Options(providerOptions...).
		Value(&selectedProvider)

	group1 := huh.NewGroup(providerSelect).
		Title("Step 1 of 3: Provider Selection").
		Description("Choose which LLM provider prompter should route requests to by default.")

	step1 := huh.NewForm(group1).WithKeyMap(keyMap).WithShowHelp(true)
	if err := step1.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return nil
		}
		return fmt.Errorf("config form: %w", err)
	}

	// Adjust defaults for newly selected provider
	pCfg := cfg.Providers[selectedProvider]
	var keyEnv string
	if pCfg.KeyEnv != "" {
		keyEnv = pCfg.KeyEnv
	} else {
		keyEnv = defaultKeyEnvFor(selectedProvider)
	}
	var model string
	if pCfg.Model != "" {
		model = pCfg.Model
	} else {
		model = defaultModelFor(selectedProvider)
	}
	baseURL := pCfg.BaseURL

	// Prepare recent model choices
	popularModels := popularModelsFor(selectedProvider, discovered...)
	modelOptions := make([]huh.Option[string], 0, len(popularModels)+1)
	isPreset := false

	for _, m := range popularModels {
		modelOptions = append(modelOptions, huh.NewOption(m.label, m.id))
		if m.id == model {
			isPreset = true
		}
	}
	modelOptions = append(modelOptions, huh.NewOption("Custom model (type name in next field)", "custom"))

	selectedModelOption := model
	customModelInputVal := ""
	if !isPreset {
		selectedModelOption = "custom"
		customModelInputVal = model
	}

	// =========================================================================
	// STEP 2: Model & Reasoning Profile
	// =========================================================================
	modelSelect := huh.NewSelect[string]().
		Title("Default Model Selection").
		Description(fmt.Sprintf("Choose from the latest %s models, or select 'Custom model':", strings.ToUpper(selectedProvider))).
		Options(modelOptions...).
		Value(&selectedModelOption)

	customModelInput := huh.NewInput().
		Title("Custom Model Identifier").
		Description("(Optional) Only applied when 'Custom model' is selected above").
		Placeholder(defaultModelFor(selectedProvider)).
		Value(&customModelInputVal)

	effortSelect := huh.NewSelect[string]().
		Title("Reasoning Effort Level").
		Description("Controls reasoning/thinking token budget for supported models (Gemini 3.7, OpenAI o-series):").
		Options(effortOptions...).
		Value(&effort)

	group2 := huh.NewGroup(modelSelect, customModelInput, effortSelect).
		Title("Step 2 of 3: Model & Intelligence Settings").
		Description("Hit TAB / SHIFT+TAB to switch between fields  •  ENTER to advance to Step 3")

	step2 := huh.NewForm(group2).WithKeyMap(keyMap).WithShowHelp(true)
	if err := step2.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return nil
		}
		return fmt.Errorf("config form: %w", err)
	}

	// =========================================================================
	// STEP 3: Authentication & System Settings
	// =========================================================================
	currentKeyEnv := keyEnv
	if currentKeyEnv == "" {
		currentKeyEnv = defaultKeyEnvFor(selectedProvider)
	}

	keyStatusNote := ""
	if selectedProvider == "omlx" {
		keyStatusNote = "(optional — local server / keyless)"
	} else if selectedProvider == "gemini" {
		if os.Getenv(currentKeyEnv) != "" {
			keyStatusNote = fmt.Sprintf("(✓ $%s detected in shell)", currentKeyEnv)
		} else if os.Getenv("PROMPTER_GEMINI_API_KEY") != "" {
			keyStatusNote = "(✓ $PROMPTER_GEMINI_API_KEY detected in shell)"
		} else {
			keyStatusNote = "(ADC availability not checked; API key optional)"
		}
	} else if os.Getenv(currentKeyEnv) != "" {
		keyStatusNote = fmt.Sprintf("(✓ $%s is set in your environment)", currentKeyEnv)
	} else {
		keyStatusNote = fmt.Sprintf("(✗ $%s is NOT set in environment)", currentKeyEnv)
	}

	keyEnvDescription := fmt.Sprintf("Shell variable holding your API key %s", keyStatusNote)

	keyEnvInput := huh.NewInput().
		Title("API Key Variable Name").
		Description(keyEnvDescription).
		Placeholder(defaultKeyEnvFor(selectedProvider)).
		Value(&keyEnv)

	baseURLPlaceholder := defaultBaseURLFor(selectedProvider)
	if baseURLPlaceholder == "" {
		baseURLPlaceholder = "https://api.example.com/v1"
	}
	baseURLInput := huh.NewInput().
		Title("Custom Base URL Override (Optional)").
		Description("Custom API endpoint or local proxy (leave blank to use provider default):").
		Placeholder(baseURLPlaceholder).
		Value(&baseURL)

	copyOptions := []huh.Option[bool]{
		huh.NewOption("Disabled (do not copy to clipboard automatically)", false),
		huh.NewOption("Enabled (automatically copy non-streamed results to clipboard)", true),
	}

	copySelect := huh.NewSelect[bool]().
		Title("Automatic System Clipboard Sync").
		Description("Choose whether to automatically copy prompt outputs to system clipboard:").
		Options(copyOptions...).
		Value(&defaultCopy)

	group3 := huh.NewGroup(keyEnvInput, baseURLInput, copySelect).
		Title("Step 3 of 3: Authentication & System Integration").
		Description("Hit TAB / SHIFT+TAB to switch between fields  •  ENTER to confirm and save settings")

	step3 := huh.NewForm(group3).WithKeyMap(keyMap).WithShowHelp(true)
	if err := step3.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return nil
		}
		return fmt.Errorf("config form: %w", err)
	}

	// Resolve final model identifier
	finalModel := selectedModelOption
	if selectedModelOption == "custom" {
		if trimmed := strings.TrimSpace(customModelInputVal); trimmed != "" {
			finalModel = trimmed
		} else {
			finalModel = defaultModelFor(selectedProvider)
		}
	}

	// Apply updates to config struct
	cfg.Provider = selectedProvider
	updatedProvider := cfg.Providers[selectedProvider]
	updatedProvider.KeyEnv = strings.TrimSpace(keyEnv)
	updatedProvider.Model = strings.TrimSpace(finalModel)
	updatedProvider.BaseURL = strings.TrimSpace(baseURL)
	cfg.Providers[selectedProvider] = updatedProvider
	cfg.Effort = effort
	cfg.DefaultCopy = defaultCopy

	// Save to config file with portable ~ paths
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	home, _ := os.UserHomeDir()
	configFilePath := "~/.config/prompter/config.json"
	if home != "" {
		configFilePath = filepath.Join(home, ".config", "prompter", "config.json")
	}

	// Print clean, structured summary card
	fmt.Println("\n✓ Configuration Saved Successfully!")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("  Active Provider:     %s\n", cfg.Provider)
	fmt.Printf("  Default Model:       %s\n", finalModel)
	if updatedProvider.KeyEnv != "" {
		keyStatus := ""
		if cfg.Provider == "omlx" {
			keyStatus = "local server / keyless ✓"
		} else if cfg.Provider == "gemini" {
			if os.Getenv(updatedProvider.KeyEnv) != "" {
				keyStatus = "detected in environment ✓"
			} else if os.Getenv("PROMPTER_GEMINI_API_KEY") != "" {
				keyStatus = "$PROMPTER_GEMINI_API_KEY detected ✓"
			} else {
				keyStatus = "ADC availability not checked (optional key not set)"
			}
		} else if os.Getenv(updatedProvider.KeyEnv) != "" {
			keyStatus = "detected in environment ✓"
		} else {
			keyStatus = fmt.Sprintf("NOT set in environment ✗ (export %s=...)", updatedProvider.KeyEnv)
		}
		fmt.Printf("  API Key Variable:    $%s [%s]\n", updatedProvider.KeyEnv, keyStatus)
	}
	if updatedProvider.BaseURL != "" {
		fmt.Printf("  Custom Base URL:     %s\n", updatedProvider.BaseURL)
	}
	fmt.Printf("  Reasoning Effort:    %s\n", cfg.Effort)
	fmt.Printf("  Clipboard Sync:      %t\n", cfg.DefaultCopy)
	fmt.Printf("  Config File:         %s\n", configFilePath)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("\nReady! Try running:")
	fmt.Println("  prompter refine \"explain quantum computing to a 10 year old\"")
	fmt.Println("  prompter browse")

	return nil
}

func confirmEmbeddedModelCatalog(fetchErr error) (bool, error) {
	useEmbedded := false
	confirm := huh.NewConfirm().
		Title("Models.dev catalog unavailable").
		Description(fmt.Sprintf("%v\nUse prompter's embedded verified model catalog instead?", fetchErr)).
		Affirmative("Use embedded catalog").
		Negative("Cancel").
		Value(&useEmbedded)
	if err := huh.NewForm(huh.NewGroup(confirm)).Run(); err != nil {
		return false, fmt.Errorf("model catalog fallback confirmation: %w", err)
	}
	return useEmbedded, nil
}
