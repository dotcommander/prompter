package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/dotcommander/prompter/internal/config"
	"github.com/dotcommander/prompter/internal/provider"
)

// -----------------------------------------------------------------------------
// Provider Resolution
// -----------------------------------------------------------------------------

func resolveProvider(cfg *config.Config, providerName, flagBaseURL string) (provider.Provider, error) {
	configured := cfg.Providers[providerName]
	settings := provider.ProviderSettings{
		APIKey: configured.APIKey, Model: configured.Model, BaseURL: configured.BaseURL,
		ProjectID: configured.ProjectID, Location: configured.Location,
	}
	if flagBaseURL != "" {
		settings.BaseURL = flagBaseURL
	}
	return provider.NewProvider(providerName, provider.RegistryConfig{
		MaxOutputTokens: cfg.MaxOutputTokens,
		MaxRetries:      cfg.MaxRetries,
		Providers:       map[string]provider.ProviderSettings{providerName: settings},
	})
}

func validateProviderCredentials(prov provider.Provider) error {
	if prov.APIKey() == "" {
		return fmt.Errorf("%s API key not set", prov.Name())
	}
	return nil
}

// -----------------------------------------------------------------------------
// Run
// -----------------------------------------------------------------------------

func runWithOutput(ctx context.Context, f *flags, cfg *config.Config, logger *slog.Logger, output commandOutput) error {
	if f.stream && f.output != "" {
		return fmt.Errorf("--output cannot be used with --stream")
	}
	if f.stream && (f.copy || cfg.DefaultCopy) {
		return fmt.Errorf("--copy cannot be used with --stream")
	}
	switch f.command {
	case commandImage:
		return runAssemble(f, cfg, output)
	}

	input, err := readInput(f.file, f.args)
	if err != nil {
		return err
	}

	preparedInput := &promptInput{source: input}

	// L0 triage: when routing_mode=auto owns selection, estimate the bounded
	// input and pick the local or frontier tier. Explicit -p, env, and file
	// provider selections bypass routing entirely.
	providerName := f.provider
	routingNote := ""
	if providerName == "" {
		providerName = cfg.Provider
		if routingEnabled(cfg.RoutingMode, "", cfg.Origin("provider")) {
			decision, err := decideRoute(cfg.RoutingLocalProvider, cfg.RoutingFrontierProvider, cfg.RoutingLocalMaxTokens, preparedInput)
			if err != nil {
				return err
			}
			providerName = decision.Provider
			routingNote = decision.Detail
		}
	}

	prov, err := resolveProvider(cfg, providerName, f.baseURL)
	if err != nil {
		return err
	}

	modelName := prov.Model()
	if f.model != "" {
		modelName = f.model
	}
	if modelName == "" {
		return fmt.Errorf("%s model not set", prov.Name())
	}

	logger.Debug("starting", "provider", prov.Name(), "model", modelName, "effort", cfg.Effort)

	timeout := time.Duration(config.DefaultTimeout) * time.Second
	if cfg.Timeout > 0 {
		timeout = time.Duration(cfg.Timeout) * time.Second
	}
	if f.stream {
		timeout = max(timeout, time.Duration(config.StreamingTimeout)*time.Second)
	}

	// Deterministic preflight: reject prompts that cannot fit the configured
	// context window before contacting the provider or emitting a dry run.
	if err := preflightContextWindow(cfg.Providers[prov.Name()].ContextWindow, cfg.MaxOutputTokens, cfg.SystemPrompt, preparedInput, prov.Name()); err != nil {
		return err
	}

	if f.dryRun {
		printDryRun(output.stderr, prov, modelName, f, cfg, preparedInput, timeout, routingNote)
		return nil
	}
	if err := validateProviderCredentials(prov); err != nil {
		return err
	}

	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req := provider.CallRequest{
		Model:        modelName,
		SystemPrompt: cfg.SystemPrompt,
		UserPrompt:   preparedInput.bound(),
		Effort:       cfg.Effort,
	}

	timeoutErr := func(err error) error {
		if callCtx.Err() != nil {
			return fmt.Errorf("%s timed out after %s", prov.Name(), timeout)
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}

	if f.stream {
		logger.Debug("streaming")
		start := time.Now()
		if err := prov.StreamCall(callCtx, req, output.stdout); err != nil {
			return timeoutErr(err)
		}
		fmt.Fprintln(output.stdout)
		logger.Info("completed", "model", modelName, "duration", time.Since(start).Round(time.Millisecond))
		return nil
	}

	var spinner *Spinner
	if f.verbose {
		spinner = NewSpinner(logger, modelName, output.stderr)
		spinner.Start()
	}

	result, err := prov.Call(callCtx, req)

	if spinner != nil {
		spinner.Stop()
	}

	if err != nil {
		return timeoutErr(err)
	}

	result = ensureTrailingNewline(result)

	if f.output != "" {
		if err := writeOutput(f.output, result); err != nil {
			return err
		}
	}
	if err := dispatchClipboard(result, f.copy, cfg.DefaultCopy, output.stderr, copyToClipboard); err != nil {
		return err
	}
	printResult(output.stdout, result)
	return nil
}

func printDryRun(w io.Writer, prov provider.Provider, modelName string, f *flags, cfg *config.Config, input *promptInput, timeout time.Duration, routingNote string) {
	style := f.style
	if style == "" && f.command == commandRefine {
		style = "default"
	}
	fmt.Fprintf(w, "Dry run: no API call made\n")
	fmt.Fprintf(w, "Provider: %s\n", prov.Name())
	if routingNote != "" {
		fmt.Fprintf(w, "Routing: auto -> %s (%s)\n", prov.Name(), routingNote)
	}
	fmt.Fprintf(w, "Model: %s\n", modelName)
	providerConfig := cfg.Providers[prov.Name()]
	baseURL := providerConfig.BaseURL
	if f.baseURL != "" {
		baseURL = f.baseURL
	}
	if baseURL == "" {
		baseURL = "default"
	} else {
		baseURL = redactURLUserinfo(baseURL)
	}
	fmt.Fprintf(w, "Base URL: %s\n", baseURL)
	fmt.Fprintf(w, "Credential source: %s\n", cfg.CredentialStatus(prov.Name()).Source)
	if prov.Name() == "gemini" {
		fmt.Fprintf(w, "Project ID: %s\n", providerConfig.ProjectID)
		fmt.Fprintf(w, "Location: %s\n", providerConfig.Location)
	}
	fmt.Fprintf(w, "Command: %s\n", f.command)
	if style != "" {
		fmt.Fprintf(w, "Style: %s\n", style)
	}
	fmt.Fprintf(w, "Stream: %t\n", f.stream)
	fmt.Fprintf(w, "Timeout: %s\n", timeout)
	fmt.Fprintf(w, "Max output tokens: %d\n", cfg.MaxOutputTokens)
	fmt.Fprintf(w, "Max retries: %d\n", cfg.MaxRetries)
	fmt.Fprintf(w, "Effort: %s\n", cfg.Effort)
	fmt.Fprintf(w, "System prompt bytes: %d\n", len(cfg.SystemPrompt))
	fmt.Fprintf(w, "Input bytes: %d\n", len(input.source))
	fmt.Fprintf(w, "Input tokens (est): %d\n", input.estimatedTokens())
	if window := cfg.Providers[prov.Name()].ContextWindow; window > 0 {
		fmt.Fprintf(w, "Context window: %d\n", window)
	}
	if f.file != "" {
		fmt.Fprintf(w, "Input file: %s\n", f.file)
	}
	if f.output != "" {
		fmt.Fprintf(w, "Output file: %s\n", f.output)
	}
}
