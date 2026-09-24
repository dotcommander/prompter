package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/atotto/clipboard"
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
	stderr   io.Writer
}

func NewSpinner(logger *slog.Logger, model string, stderr io.Writer) *Spinner {
	return &Spinner{
		done:   make(chan struct{}),
		model:  model,
		start:  time.Now(),
		logger: logger,
		stderr: stderr,
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
				fmt.Fprintf(s.stderr, "\r%s %s %v", frames[i%len(frames)], s.model, elapsed)
				i++
			}
		}
	}()
}

func (s *Spinner) Stop() time.Duration {
	s.stopOnce.Do(func() { close(s.done) })
	elapsed := time.Since(s.start)
	fmt.Fprintf(s.stderr, "\r\033[K")
	s.logger.Info("completed", "model", s.model, "duration", elapsed.Round(time.Millisecond))
	return elapsed
}

// -----------------------------------------------------------------------------
// Flags
// -----------------------------------------------------------------------------

type flags struct {
	provider   string
	model      string
	baseURL    string
	verbose    bool
	stream     bool
	dryRun     bool
	copy       bool
	file       string
	output     string
	style      string
	styleSet   bool
	profile    string
	count      int
	json       bool
	noArtist   bool
	noPlatform bool
	categories string
	seed       string
	command    string
	args       []string
}

func parseArgs(args []string) (*flags, error) {
	return parseArgsTo(args, os.Stderr)
}

func parseArgsTo(args []string, stderr io.Writer) (*flags, error) {
	command, rest, err := selectOperation(args)
	if err != nil {
		return nil, err
	}

	f := &flags{command: command}
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}
	if hasHelpFlag(rest) {
		printCommandUsageTo(stderr, command)
		return nil, flag.ErrHelp
	}

	registerFlags(fs, f)

	parsed := interspersedFlagArgs(fs, rest)
	if err := fs.Parse(parsed); err != nil {
		return nil, err
	}
	fs.Visit(func(visited *flag.Flag) {
		if visited.Name == "style" || visited.Name == "s" {
			f.styleSet = true
		}
	})

	f.args = fs.Args()
	if command == commandConfig && len(f.args) > 0 {
		return nil, fmt.Errorf("%s does not accept input arguments", configOperationFlag)
	}
	return f, nil
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
	case commandRefine:
		registerLLMFlags(fs, f)
		fs.StringVar(&f.style, "style", "", "")
		fs.StringVar(&f.style, "s", "", "")
	case commandImage:
		registerOutputFlags(fs, f)
		fs.StringVar(&f.profile, "profile", "default", "")
		fs.IntVar(&f.count, "count", 1, "")
		fs.BoolVar(&f.json, "json", false, "")
		fs.BoolVar(&f.noArtist, "no-artist", false, "")
		fs.BoolVar(&f.noPlatform, "no-platform", false, "")
		fs.StringVar(&f.categories, "categories", "", "")
		fs.StringVar(&f.seed, "seed", "", "")
	case commandConfig:
		// --config accepts no operation flags.
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
	return newLoggerTo(os.Stderr, verbose)
}

func newLoggerTo(stderr io.Writer, verbose bool) *slog.Logger {
	level := slog.LevelWarn
	if verbose {
		level = slog.LevelDebug
	}
	return slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{
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
	return runWithOutput(ctx, f, cfg, logger, commandOutput{stdout: os.Stdout, stderr: os.Stderr})
}

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

	prov, err := resolveProvider(cfg, f.provider, f.baseURL)
	if err != nil {
		return err
	}

	input, err := readInput(f.file, f.args)
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

	if f.dryRun {
		printDryRun(output.stderr, prov, modelName, f, cfg, input, timeout)
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
		UserPrompt:   boundPromptInput(input),
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
			fmt.Fprintln(output.stderr, "Copied to clipboard")
		}
	}
	printResult(output.stdout, result)
	return nil
}

func printResult(w io.Writer, result string) {
	if strings.HasSuffix(result, "\n") {
		fmt.Fprint(w, result)
		return
	}
	fmt.Fprintln(w, result)
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

func runAssemble(f *flags, cfg *config.Config, output commandOutput) error {
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

	var result string
	if f.json {
		data, err := json.MarshalIndent(singleOrMany(results), "", "  ")
		if err != nil {
			return fmt.Errorf("encode assemble json: %w", err)
		}
		result = string(data) + "\n"
	} else {
		var b strings.Builder
		for i, prompt := range results {
			if len(results) > 1 {
				fmt.Fprintf(&b, "=== Variation %d ===\n", i+1)
			}
			fmt.Fprintln(&b, prompt.FullPrompt)
			if i < len(results)-1 {
				fmt.Fprintln(&b)
			}
		}
		result = b.String()
	}
	if f.output != "" {
		if err := writeOutput(f.output, result); err != nil {
			return err
		}
	}
	if f.copy || cfg.DefaultCopy {
		if err := copyToClipboard(result); err != nil {
			return err
		}
		fmt.Fprintln(output.stderr, "Copied to clipboard")
	}
	printResult(output.stdout, result)
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

// rootArgs applies default-enrichment routing to the raw root invocation.
// Piped or positional input without an explicit operation enriches by default.
// Retired command words pass through untouched so argument parsing can return
// the migration error instead of silently enriching them.
func rootArgs(args []string, stdinPiped bool) []string {
	if len(args) == 0 {
		if stdinPiped {
			return []string{commandRefine}
		}
		return nil
	}
	switch args[0] {
	case commandRefine, imageOperationFlag, configOperationFlag, "--help", "-h", "--version", "-V":
		return args
	}
	if isRetiredCommand(args[0]) {
		return args
	}
	return append([]string{commandRefine}, args...)
}

// copyToClipboard copies text to the system clipboard using a cross-platform library.
func copyToClipboard(text string) error {
	if err := clipboard.WriteAll(text); err != nil {
		return fmt.Errorf("clipboard write failed: %w", err)
	}
	return nil
}
