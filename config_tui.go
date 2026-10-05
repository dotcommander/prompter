package main

import (
	"errors"
	"fmt"
	"net/url"
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

func defaultProviderFor(p string) config.ProviderConfig {
	return config.DefaultProviders()[p]
}

var ErrConfigCancelled = errors.New("configuration cancelled; no settings saved")

var runConfigStep = func(form *huh.Form) error { return form.Run() }

func configKeyMap() *huh.KeyMap {
	keyMap := huh.NewDefaultKeyMap()
	keyMap.Quit = key.NewBinding(key.WithKeys("ctrl+c"))
	return keyMap
}

func configFormError(err error) error {
	if errors.Is(err, huh.ErrUserAborted) {
		return ErrConfigCancelled
	}
	return fmt.Errorf("config form: %w", err)
}

func validateBaseURL(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Hostname() == "" || parsed.Opaque != "" {
		return errors.New("enter an absolute HTTP or HTTPS URL with a host, or leave blank")
	}
	return nil
}

func isProviderConfigured(p string, cfg *config.Config) (bool, string) {
	status := cfg.CredentialStatus(p)
	return status.Available, status.Source
}

type modelChoice struct {
	id    string
	label string
}

// popularModelsFor returns the local, offline model choices offered by the
// configuration form. No catalog fetch happens to open configuration.
func popularModelsFor(p string) []modelChoice {
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
			{"deepseek-v4.1-flash", "deepseek-v4.1-flash (Default)"},
			{"deepseek-v4.1-flash-vision-exp", "deepseek-v4.1-flash-vision-exp (Experimental Vision)"},
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
// It uses only the configured model and local model choices; opening the form
// never performs a network request.
func RunConfigForm(cfg *config.Config) error {
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

	providerOptions := make([]huh.Option[string], len(providersList))

	for i, prov := range providersList {
		configured, detail := isProviderConfigured(prov.id, cfg)
		var label string
		if configured {
			label = fmt.Sprintf("%-48s [✓ %s]", prov.name, detail)

		} else {
			label = fmt.Sprintf("%-48s [? %s]", prov.name, detail)
		}
		providerOptions[i] = huh.NewOption(label, prov.id)
	}

	effortOptions := []huh.Option[string]{
		huh.NewOption("Low (Fastest — direct, low-latency generation)", "low"),
		huh.NewOption("Medium (Balanced — balanced thinking & reasoning)", "medium"),
		huh.NewOption("High (Deep — multi-step reasoning for complex tasks)", "high"),
	}

	keyMap := configKeyMap()

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
	if err := runConfigStep(step1); err != nil {
		return configFormError(err)
	}

	// Adjust defaults for newly selected provider
	pCfg := cfg.Providers[selectedProvider]
	providerDefault := defaultProviderFor(selectedProvider)
	var keyEnv string
	if pCfg.KeyEnv != "" {
		keyEnv = pCfg.KeyEnv
		if config.ValidateKeyEnv(keyEnv) != nil {
			keyEnv = ""
		}
	} else {
		keyEnv = defaultKeyEnvFor(selectedProvider)
	}
	var model string
	if pCfg.Model != "" {
		model = pCfg.Model
	} else {
		model = providerDefault.Model
	}
	baseURL := pCfg.BaseURL

	// Prepare recent model choices
	popularModels := popularModelsFor(selectedProvider)
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
		Placeholder(providerDefault.Model).
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
	if err := runConfigStep(step2); err != nil {
		return configFormError(err)
	}

	// =========================================================================
	// STEP 3: Authentication & System Settings
	// =========================================================================
	keyEnvDescription := "Shell variable holding your API key (" + cfg.CredentialStatus(selectedProvider).Source + ")"

	keyEnvInput := huh.NewInput().
		Title("API Key Variable Name").
		Description(keyEnvDescription).
		Placeholder(defaultKeyEnvFor(selectedProvider)).
		Value(&keyEnv).
		Validate(config.ValidateKeyEnv)

	baseURLPlaceholder := providerDefault.BaseURL
	if baseURLPlaceholder == "" {
		baseURLPlaceholder = "https://api.example.com/v1"
	}
	baseURLInput := huh.NewInput().
		Title("Custom Base URL Override (Optional)").
		Description("Custom API endpoint or local proxy (leave blank to use provider default):").
		Placeholder(baseURLPlaceholder).
		Value(&baseURL).
		Validate(validateBaseURL)

	copyOptions := []huh.Option[bool]{
		huh.NewOption("Disabled (do not copy to clipboard automatically)", false),
		huh.NewOption("Enabled (automatically copy non-streamed results to clipboard)", true),
	}

	copySelect := huh.NewSelect[bool]().
		Title("Automatic System Clipboard Sync").
		Description("Choose whether to automatically copy prompt outputs to system clipboard:").
		Options(copyOptions...).
		Value(&defaultCopy)

	projectID, location := pCfg.ProjectID, pCfg.Location
	fields := []huh.Field{keyEnvInput, baseURLInput, copySelect}
	if selectedProvider == "gemini" {
		fields = append(fields,
			huh.NewInput().Title("Google Cloud Project ID").Description("Used by Vertex AI; optional when using an AI Studio URL.").Value(&projectID),
			huh.NewInput().Title("Google Cloud Location").Placeholder("global").Value(&location))
	}
	group3 := huh.NewGroup(fields...).
		Title("Step 3 of 3: Authentication & System Integration").
		Description("Hit TAB / SHIFT+TAB to switch between fields  •  ENTER to confirm and save settings")

	step3 := huh.NewForm(group3).WithKeyMap(keyMap).WithShowHelp(true)
	if err := runConfigStep(step3); err != nil {
		return configFormError(err)
	}

	// Resolve final model identifier
	finalModel := selectedModelOption
	if selectedModelOption == "custom" {
		if trimmed := strings.TrimSpace(customModelInputVal); trimmed != "" {
			finalModel = trimmed
		} else {
			finalModel = providerDefault.Model
		}
	}

	// Every confirmed wizard field is an explicit assignment, even if equal to a default.
	keyEnv = strings.TrimSpace(keyEnv)
	finalModel = strings.TrimSpace(finalModel)
	baseURL = strings.TrimSpace(baseURL)
	projectID, location = strings.TrimSpace(projectID), strings.TrimSpace(location)
	providerPatch := config.ProviderPatch{KeyEnv: &keyEnv, Model: &finalModel, BaseURL: &baseURL}
	if selectedProvider == "gemini" {
		providerPatch.ProjectID = &projectID
		providerPatch.Location = &location
	}
	cfg.ApplyPatch(config.ConfigPatch{Provider: &selectedProvider, Effort: &effort, DefaultCopy: &defaultCopy,
		Providers: map[string]config.ProviderPatch{selectedProvider: providerPatch}})
	updatedProvider := cfg.Providers[selectedProvider]

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
	fmt.Printf("  Credential source:   %s\n", cfg.CredentialStatus(cfg.Provider).Source)

	if updatedProvider.BaseURL != "" {
		fmt.Printf("  Custom Base URL:     %s\n", redactURLUserinfo(updatedProvider.BaseURL))
	}
	fmt.Printf("  Reasoning Effort:    %s\n", cfg.Effort)
	fmt.Printf("  Clipboard Sync:      %t\n", cfg.DefaultCopy)
	fmt.Printf("  Config File:         %s\n", configFilePath)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("\nReady! Try running:")
	fmt.Println("  prompter refine \"explain quantum computing to a 10 year old\"")
	fmt.Println("  prompter --image \"desert observatory\" --profile minimal")

	return nil
}
