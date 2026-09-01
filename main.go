package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/x/term"
	"github.com/dotcommander/prompter/internal/config"
	"github.com/dotcommander/prompter/internal/provider"
)

// -----------------------------------------------------------------------------
// Input Reading
// -----------------------------------------------------------------------------

type CLIInputReader struct {
	args []string
}

func (r *CLIInputReader) Read() (string, error) {
	if len(r.args) > 0 {
		return strings.Join(r.args, " "), nil
	}
	return readStdin()
}

func isStdinPiped() bool {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) == 0
}

type terminalFile interface {
	Stat() (os.FileInfo, error)
	Fd() uintptr
}

// isInteractiveTerminal accepts only a character device confirmed by the
// platform terminal detector. Stat or detector failure is non-interactive.
func isInteractiveTerminal(file terminalFile, detector func(uintptr) bool) bool {
	stat, err := file.Stat()
	if err != nil || stat.Mode()&os.ModeCharDevice == 0 {
		return false
	}
	return detector(file.Fd())
}

const maxInputBytes = 1 << 20 // 1 MB

func readLimited(r io.Reader) (string, error) {
	limited := io.LimitReader(r, maxInputBytes+1)
	reader := bufio.NewReader(limited)
	var builder strings.Builder

	for {
		line, err := reader.ReadString('\n')
		builder.WriteString(line)
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
	}

	if builder.Len() > maxInputBytes {
		return "", fmt.Errorf("input exceeds %d bytes (1 MB limit; use --file or split large inputs)", maxInputBytes)
	}

	return builder.String(), nil
}

func readStdin() (string, error) {
	return readLimited(os.Stdin)
}

func readFileInput(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	return readLimited(f)
}

// -----------------------------------------------------------------------------
// Spinner
// -----------------------------------------------------------------------------

type Spinner struct {
	done     chan struct{}
	stopOnce sync.Once
	model    string
	start    time.Time
	logger   *slog.Logger
}

func NewSpinner(logger *slog.Logger, model string) *Spinner {
	return &Spinner{
		done:   make(chan struct{}),
		model:  model,
		start:  time.Now(),
		logger: logger,
	}
}

func (s *Spinner) Start() {
	go func() {
		frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		i := 0
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-s.done:
				return
			case <-ticker.C:
				elapsed := time.Since(s.start).Round(100 * time.Millisecond)
				fmt.Fprintf(os.Stderr, "\r%s %s %v", frames[i%len(frames)], s.model, elapsed)
				i++
			}
		}
	}()
}

func (s *Spinner) Stop() time.Duration {
	s.stopOnce.Do(func() { close(s.done) })
	elapsed := time.Since(s.start)
	fmt.Fprintf(os.Stderr, "\r\033[K")
	s.logger.Info("completed", "model", s.model, "duration", elapsed.Round(time.Millisecond))
	return elapsed
}

// -----------------------------------------------------------------------------
// Flags
// -----------------------------------------------------------------------------

type flags struct {
	provider         string
	model            string
	baseURL          string
	verbose          bool
	stream           bool
	dryRun           bool
	copy             bool
	file             string
	output           string
	style            string
	styleSet         bool
	profile          string
	count            int
	json             bool
	noArtist         bool
	noPlatform       bool
	categories       string
	seed             string
	rewriteMode      string
	promptAction     string
	command          string
	promptName       string
	outputValidation *OutputValidation
	args             []string
}

func parseArgs(args []string) (*flags, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("command required")
	}
	requestedCommand := args[0]
	command := canonicalCommand(requestedCommand)
	if !isCommand(command) {
		return nil, fmt.Errorf("unknown command %q", args[0])
	}

	f := &flags{command: command}
	args = args[1:]
	fs := flag.NewFlagSet(f.command, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}
	if hasHelpFlag(args) {
		printCommandUsageTo(os.Stderr, f.command)
		return nil, flag.ErrHelp
	}

	registerFlags(fs, f)

	args = interspersedFlagArgs(fs, args)
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	fs.Visit(func(visited *flag.Flag) {
		if visited.Name == "style" || visited.Name == "s" {
			f.styleSet = true
		}
	})

	f.args = fs.Args()
	if f.command == commandApply {
		if len(f.args) == 0 {
			return nil, fmt.Errorf("apply requires a prompt name or alias")
		}
		f.promptName = f.args[0]
		f.args = f.args[1:]
	}
	if f.command == commandModels {
		if len(f.args) != 1 || f.args[0] != "refresh" {
			return nil, fmt.Errorf("models requires the refresh action")
		}
	}
	if f.command == commandPrompts {
		if len(f.args) != 1 || (f.args[0] != "status" && f.args[0] != "upgrade") {
			return nil, fmt.Errorf("prompts requires the status or upgrade action")
		}
		f.promptAction = f.args[0]
		if f.dryRun && f.promptAction != "upgrade" {
			return nil, fmt.Errorf("--dry-run is only valid with prompts upgrade")
		}
	}
	if (f.command == commandBrowse || f.command == commandConfigure) && len(f.args) > 0 {
		return nil, fmt.Errorf("%s does not accept arguments", f.command)
	}
	return f, nil
}

func canonicalCommand(command string) string {
	if command == commandConfigAlias {
		return commandConfigure
	}
	return command
}

func hasHelpFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--" {
			return false
		}
		if arg == "-h" || arg == "--help" {
			return true
		}
	}
	return false
}

func registerFlags(fs *flag.FlagSet, f *flags) {
	switch f.command {
	case commandRefine, commandCritique, commandRewrite, commandApply:
		registerLLMFlags(fs, f)
	}
	switch f.command {
	case commandRefine:
		fs.StringVar(&f.style, "style", "", "")
		fs.StringVar(&f.style, "s", "", "")
	case commandRewrite:
		fs.StringVar(&f.rewriteMode, "mode", "clean", "")
	case commandImage:
		registerOutputFlags(fs, f)
		fs.StringVar(&f.profile, "profile", "default", "")
		fs.IntVar(&f.count, "count", 1, "")
		fs.BoolVar(&f.json, "json", false, "")
		fs.BoolVar(&f.noArtist, "no-artist", false, "")
		fs.BoolVar(&f.noPlatform, "no-platform", false, "")
		fs.StringVar(&f.categories, "categories", "", "")
		fs.StringVar(&f.seed, "seed", "", "")
	case commandPrompts:
		fs.BoolVar(&f.dryRun, "dry-run", false, "")
	}
}

func registerLLMFlags(fs *flag.FlagSet, f *flags) {
	fs.StringVar(&f.provider, "provider", "", "")
	fs.StringVar(&f.provider, "p", "", "")
	fs.StringVar(&f.model, "model", "", "")
	fs.StringVar(&f.model, "m", "", "")
	fs.StringVar(&f.baseURL, "base-url", "", "")
	fs.BoolVar(&f.verbose, "verbose", false, "")
	fs.BoolVar(&f.verbose, "v", false, "")
	fs.BoolVar(&f.stream, "stream", false, "")
	fs.BoolVar(&f.dryRun, "dry-run", false, "")
	registerOutputFlags(fs, f)
}

func registerOutputFlags(fs *flag.FlagSet, f *flags) {
	fs.BoolVar(&f.copy, "copy", false, "")
	fs.BoolVar(&f.copy, "c", false, "")
	fs.StringVar(&f.file, "file", "", "")
	fs.StringVar(&f.file, "f", "", "")
	fs.StringVar(&f.output, "output", "", "")
	fs.StringVar(&f.output, "o", "", "")
}

func isCommand(value string) bool {
	switch value {
	case commandRefine, commandCritique, commandRewrite, commandApply, commandBrowse, commandImage, commandConfigure, commandModels, commandPrompts:
		return true
	default:
		return false
	}
}

func printUsage() {
	printUsageTo(os.Stderr)
}

// exitWithError prints an error to stderr and exits with code 1.
func exitWithError(err error) {
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}

func readInput(file string, args []string) (string, error) {
	if file != "" {
		input, err := readFileInput(file)
		if err != nil {
			return "", fmt.Errorf("reading file input: %w", err)
		}
		input = strings.TrimSpace(input)
		if input == "" {
			return "", fmt.Errorf("empty input")
		}
		return input, nil
	}

	hasArgs := len(args) > 0
	if !hasArgs && !isStdinPiped() {
		return "", fmt.Errorf("no input")
	}

	inputReader := &CLIInputReader{args: args}
	input, err := inputReader.Read()
	if err != nil {
		return "", fmt.Errorf("reading input: %w", err)
	}

	input = strings.TrimSpace(input)
	if input == "" {
		return "", fmt.Errorf("empty input")
	}
	return input, nil
}

func writeOutput(path, content string) error {
	dir := filepath.Dir(path)
	if dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create output dir: %w", err)
		}
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	return nil
}

func newLogger(verbose bool) *slog.Logger {
	level := slog.LevelWarn
	if verbose {
		level = slog.LevelDebug
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	}))
}

// -----------------------------------------------------------------------------
// Provider Resolution
// -----------------------------------------------------------------------------

func resolveProvider(cfg *config.Config, flagProvider, flagBaseURL string) (provider.Provider, error) {
	providerName := cfg.Provider
	if flagProvider != "" {
		providerName = flagProvider
	}

	settings := make(map[string]provider.ProviderSettings, len(cfg.Providers))
	for name, configured := range cfg.Providers {
		settings[name] = provider.ProviderSettings{
			APIKey: configured.APIKey, Model: configured.Model, BaseURL: configured.BaseURL,
			ProjectID: configured.ProjectID, Location: configured.Location,
		}
	}

	if flagBaseURL != "" {
		selected := settings[providerName]
		selected.BaseURL = flagBaseURL
		settings[providerName] = selected
	}

	registry := provider.NewRegistry(provider.RegistryConfig{
		MaxOutputTokens: cfg.MaxOutputTokens,
		MaxRetries:      cfg.MaxRetries,
		Providers:       settings,
	})
	prov, ok := registry[providerName]
	if !ok {
		return nil, fmt.Errorf("unknown provider %q (valid: %s)", providerName, provider.KnownNamesString())
	}

	return prov, nil
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

func run(ctx context.Context, f *flags, cfg *config.Config, logger *slog.Logger) error {
	if f.stream && f.outputValidation != nil {
		return fmt.Errorf("--stream cannot be used with prompt output validation")
	}
	if f.stream && f.output != "" {
		return fmt.Errorf("--output cannot be used with --stream")
	}
	if f.stream && (f.copy || cfg.DefaultCopy) {
		return fmt.Errorf("--copy cannot be used with --stream")
	}
	switch f.command {
	case commandImage:
		return runAssemble(f, cfg)
	}

	prov, err := resolveProvider(cfg, f.provider, f.baseURL)
	if err != nil {
		return err
	}

	input, err := readInput(f.file, f.args)
	if err != nil {
		return err
	}
	if f.command == commandRewrite {
		input = preprocessRewriteInputForMode(input, f.rewriteMode)
		if strings.TrimSpace(input) == "" {
			return fmt.Errorf("rewrite preprocessing removed all content; refusing to send an empty request")
		}
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

	if f.dryRun {
		printDryRun(os.Stderr, prov, modelName, f, cfg, input, timeout)
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
		UserPrompt:   input,
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
		if err := prov.StreamCall(callCtx, req, os.Stdout); err != nil {
			return timeoutErr(err)
		}
		fmt.Println()
		logger.Info("completed", "model", modelName, "duration", time.Since(start).Round(time.Millisecond))
		return nil
	}

	var spinner *Spinner
	if f.verbose {
		spinner = NewSpinner(logger, modelName)
		spinner.Start()
	}

	result, retried, err := callWithOutputValidation(callCtx, prov, req, input, f.outputValidation)

	if spinner != nil {
		spinner.Stop()
	}

	if err != nil {
		return timeoutErr(err)
	}
	if retried {
		logger.Info("output corrected after validation retry", "prompt", f.promptName)
	}

	if f.output != "" {
		if err := writeOutput(f.output, result); err != nil {
			return err
		}
	}
	if f.copy || cfg.DefaultCopy {
		if err := copyToClipboard(result); err != nil {
			if f.copy {
				return err
			}
			logger.Debug("clipboard unavailable in headless environment", "error", err)
		} else {
			fmt.Fprintln(os.Stderr, "Copied to clipboard")
		}
	}
	if strings.HasSuffix(result, "\n") {
		fmt.Print(result)
	} else {
		fmt.Println(result)
	}
	return nil
}

func printDryRun(w io.Writer, prov provider.Provider, modelName string, f *flags, cfg *config.Config, input string, timeout time.Duration) {
	style := f.style
	if style == "" && f.command == commandRefine {
		style = "default"
	}
	fmt.Fprintf(w, "Dry run: no API call made\n")
	fmt.Fprintf(w, "Provider: %s\n", prov.Name())
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
	fmt.Fprintf(w, "Credential source: %s\n", dryRunCredentialSource(prov.Name(), providerConfig))
	if prov.Name() == "gemini" {
		fmt.Fprintf(w, "Project ID: %s\n", providerConfig.ProjectID)
		fmt.Fprintf(w, "Location: %s\n", providerConfig.Location)
	}
	fmt.Fprintf(w, "Command: %s\n", f.command)
	if f.command == commandRewrite {
		fmt.Fprintf(w, "Mode: %s\n", f.rewriteMode)
	}
	if f.command == commandApply {
		fmt.Fprintf(w, "Prompt: %s\n", f.promptName)
		if f.outputValidation != nil {
			fmt.Fprintf(w, "Output validation: enabled (%d retry, semantic=%t)\n", f.outputValidation.Retries, f.outputValidation.SemanticValidation)
		}
	}
	if style != "" {
		fmt.Fprintf(w, "Style: %s\n", style)
	}
	fmt.Fprintf(w, "Stream: %t\n", f.stream)
	fmt.Fprintf(w, "Timeout: %s\n", timeout)
	fmt.Fprintf(w, "Max output tokens: %d\n", cfg.MaxOutputTokens)
	fmt.Fprintf(w, "Max retries: %d\n", cfg.MaxRetries)
	fmt.Fprintf(w, "Effort: %s\n", cfg.Effort)
	fmt.Fprintf(w, "System prompt bytes: %d\n", len(cfg.SystemPrompt))
	fmt.Fprintf(w, "Input bytes: %d\n", len(input))
	if f.file != "" {
		fmt.Fprintf(w, "Input file: %s\n", f.file)
	}
	if f.output != "" {
		fmt.Fprintf(w, "Output file: %s\n", f.output)
	}
}

func dryRunCredentialSource(providerName string, providerConfig config.ProviderConfig) string {
	if providerName == "gemini" && providerConfig.APIKey == "" {
		return "google-adc"
	}
	if providerConfig.KeyEnv != "" {
		return providerConfig.KeyEnv
	}
	if providerConfig.APIKey != "" {
		return "config-or-" + defaultKeyEnvFor(providerName)
	}
	return defaultKeyEnvFor(providerName)
}

func runAssemble(f *flags, cfg *config.Config) error {
	if f.count < 1 {
		return fmt.Errorf("--count must be >= 1")
	}
	input, err := readInput(f.file, f.args)
	if err != nil {
		return err
	}
	profile, err := assemblyProfile(f.profile)
	if err != nil {
		return err
	}
	if f.noArtist {
		profile.IncludeArtist = false
	}
	if f.noPlatform {
		profile.IncludePlatform = false
	}
	lib, err := loadComponentLibrary(cfg.ComponentsFile)
	if err != nil {
		return err
	}
	categories := parseCSV(f.categories)
	results := make([]*AssembledPrompt, 0, f.count)
	for i := 0; i < f.count; i++ {
		seed := f.seed
		if seed == "" {
			seed = fmt.Sprintf("%s:%d", input, i+1)
		} else {
			seed = fmt.Sprintf("%s:%d", seed, i+1)
		}
		assembled, err := assemblePrompt(lib, input, profile, categories, seed)
		if err != nil {
			return err
		}
		results = append(results, assembled)
	}

	var output string
	if f.json {
		data, err := json.MarshalIndent(singleOrMany(results), "", "  ")
		if err != nil {
			return fmt.Errorf("encode assemble json: %w", err)
		}
		output = string(data) + "\n"
	} else {
		var b strings.Builder
		for i, result := range results {
			if len(results) > 1 {
				fmt.Fprintf(&b, "=== Variation %d ===\n", i+1)
			}
			fmt.Fprintln(&b, result.FullPrompt)
			if i < len(results)-1 {
				fmt.Fprintln(&b)
			}
		}
		output = b.String()
	}
	if f.output != "" {
		if err := writeOutput(f.output, output); err != nil {
			return err
		}
	}
	if f.copy || cfg.DefaultCopy {
		if err := copyToClipboard(output); err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "Copied to clipboard")
	}
	if strings.HasSuffix(output, "\n") {
		fmt.Print(output)
	} else {
		fmt.Println(output)
	}
	return nil
}

func parseCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func singleOrMany(results []*AssembledPrompt) any {
	if len(results) == 1 {
		return results[0]
	}
	return results
}

func rootArgs(args []string, stdinPiped bool) []string {
	if !stdinPiped {
		return args
	}
	if len(args) == 0 {
		return []string{commandRefine}
	}
	if args[0] != "--help" && args[0] != "-h" && args[0] != "--version" && args[0] != "-V" && strings.HasPrefix(args[0], "-") {
		return append([]string{commandRefine}, args...)
	}
	return args
}

// -----------------------------------------------------------------------------
// Main
// -----------------------------------------------------------------------------

func main() {
	args := rootArgs(os.Args[1:], isStdinPiped())

	// Fast-path root help and version without loading configuration.
	if len(args) == 0 {
		printUsage()
		return
	}
	if len(args) == 1 {
		switch args[0] {
		case "--version", "-V":
			printVersion(os.Stdout)
			return
		case "--help", "-h":
			printUsage()
			return
		}
	}

	f, err := parseArgs(args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(2)
	}

	cfg, err := config.Load()
	if err != nil {
		exitWithError(err)
	}

	switch f.command {
	case commandBrowse:
		if err := validateBrowseTerminal(
			isInteractiveTerminal(os.Stdin, term.IsTerminal),
			isInteractiveTerminal(os.Stderr, term.IsTerminal),
		); err != nil {
			exitWithError(err)
		}
		if err := showFinder(cfg); err != nil {
			exitWithError(err)
		}
		return
	case commandConfigure:
		if !isInteractiveTerminal(os.Stdin, term.IsTerminal) || !isInteractiveTerminal(os.Stdout, term.IsTerminal) {
			printConfig(os.Stdout, cfg)
			return
		}
		service, err := newModelCatalogService()
		if err != nil {
			exitWithError(err)
		}
		catalog, _, err := service.loadOrFetch(context.Background(), cfg)
		if err != nil {
			useEmbedded, confirmErr := confirmEmbeddedModelCatalog(err)
			if confirmErr != nil {
				exitWithError(confirmErr)
			}
			if !useEmbedded {
				exitWithError(err)
			}
		}
		if err := RunConfigForm(cfg, catalogModelChoices(catalog)); err != nil {
			exitWithError(err)
		}
		return
	case commandModels:
		service, err := newModelCatalogService()
		if err != nil {
			exitWithError(err)
		}
		catalog, err := service.refresh(context.Background(), cfg)
		if err != nil {
			exitWithError(err)
		}
		printModelCatalog(os.Stdout, catalog)
		return
	case commandPrompts:
		if err := runPromptMaintenance(os.Stdout, cfg.PromptsDir, f.promptAction, f.dryRun); err != nil {
			exitWithError(err)
		}
		return
	}

	if f.command == commandRefine || f.command == commandCritique || f.command == commandApply || f.command == commandRewrite || f.command == commandImage {
		if len(f.args) == 0 && f.file == "" && !isStdinPiped() {
			exitWithError(fmt.Errorf("%s requires input", f.command))
		}
	}

	if err := resolveCommandSystemPrompt(f, cfg); err != nil {
		exitWithError(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	go func() {
		<-sigChan
		cancel()
		fmt.Fprintf(os.Stderr, "\r\033[K")
	}()

	logger := newLogger(f.verbose)

	if err := run(ctx, f, cfg, logger); err != nil {
		if ctx.Err() != nil {
			os.Exit(130)
		}
		exitWithError(err)
	}
}

func validateBrowseTerminal(stdinTerminal, stderrTerminal bool) error {
	if !stdinTerminal || !stderrTerminal {
		return fmt.Errorf("browse requires interactive stdin and stderr terminals")
	}
	return nil
}

// ensurePromptVault ensures the prompt directory exists, auto-seeding starter prompts if empty.
func ensurePromptVault(cfg *config.Config) ([]PromptEntry, []string, error) {
	return ensurePromptVaultWithInit(cfg, false, runInit)
}

func ensurePromptVaultStrict(cfg *config.Config) ([]PromptEntry, []string, error) {
	return ensurePromptVaultWithInit(cfg, true, runInit)
}

func ensurePromptVaultWithInit(cfg *config.Config, strict bool, initFn func(io.Writer, io.Writer, *config.Config, string) error) ([]PromptEntry, []string, error) {
	dirs, err := promptDirs(cfg)
	if err != nil {
		return nil, nil, err
	}

	primaryDir := ""
	if len(dirs) > 0 {
		primaryDir = dirs[0]
	}
	if primaryDir == "" {
		home, _ := os.UserHomeDir()
		if home != "" {
			primaryDir = filepath.Join(home, ".config", "prompter", "prompts.d")
		}
	}

	if primaryDir != "" {
		_ = os.MkdirAll(primaryDir, 0o755)
	}

	var entries []PromptEntry
	if primaryDir != "" {
		entries, err = ScanPromptsDir(primaryDir)
		if err != nil {
			return nil, nil, fmt.Errorf("scan primary prompt directory: %w", err)
		}
	}

	initIncomplete := false
	if primaryDir != "" {
		initIncomplete, err = promptInitMarkerPresent(filepath.Join(primaryDir, promptInitMarker))
		if err != nil {
			return nil, dirs, fmt.Errorf("inspect prompt initialization marker: %w", err)
		}
	}
	if (len(entries) == 0 || initIncomplete) && primaryDir != "" {
		// Auto-seed starter prompts on first interactive launch or configure
		var initOut, initErr bytes.Buffer
		if initErrVal := initFn(&initOut, &initErr, cfg, primaryDir); initErrVal == nil {
			fmt.Fprintf(os.Stderr, "Seeded starter prompts in %s\n", primaryDir)
			entries, err = ScanPromptsDir(primaryDir)
			if err != nil && strict {
				return nil, dirs, fmt.Errorf("scan seeded prompt vault: %w", err)
			}
		} else if strict {
			return nil, dirs, fmt.Errorf("seed prompt vault: %w", initErrVal)
		}
	}

	secondaryEntries, err := scanPromptDirs(dirs[1:])
	if err != nil {
		return nil, nil, fmt.Errorf("scan prompts: %w", err)
	}
	entries = append(entries, secondaryEntries...)

	return entries, dirs, nil
}

// showFinder scans the prompts directory and shows the fuzzy finder.
func showFinder(cfg *config.Config) error {
	entries, dirs, err := ensurePromptVault(cfg)
	if err != nil {
		return err
	}

	if len(entries) == 0 {
		fmt.Fprintf(os.Stderr, "No prompts found in %s. Add a .md prompt file and run 'prompter browse' again.\n", strings.Join(dirs, ", "))
		return nil
	}

	selected, err := RunFinder(entries, dirs...)
	if err != nil {
		return err
	}

	// User cancelled (Ctrl+C)
	if selected == nil {
		return nil
	}

	// Copy to clipboard
	if err := copyToClipboard(selected.Content); err != nil {
		fmt.Fprintf(os.Stderr, "clipboard: %v\n", err)
	}

	if strings.HasSuffix(selected.Content, "\n") {
		fmt.Print(selected.Content)
	} else {
		fmt.Println(selected.Content)
	}
	return nil
}

// copyToClipboard copies text to the system clipboard using a cross-platform library.
func copyToClipboard(text string) error {
	if err := clipboard.WriteAll(text); err != nil {
		return fmt.Errorf("clipboard write failed: %w", err)
	}
	return nil
}
