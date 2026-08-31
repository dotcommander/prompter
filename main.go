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

func isStdoutTerminal() bool {
	stat, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
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
	modeSet          bool
	command          string
	promptName       string
	outputValidation *OutputValidation
	version          bool
	force            bool
	args             []string
}

func parseArgs(args []string) (*flags, error) {
	f := &flags{}
	if len(args) > 0 && isCommand(args[0]) {
		f.command = args[0]
		args = args[1:]
	}
	fs := flag.NewFlagSet("prompter", flag.ContinueOnError)
	fs.Usage = func() {
		if f.command != "" && f.command != "help" {
			printCommandUsageTo(fs.Output(), f.command)
		} else {
			printUsageTo(fs.Output())
		}
	}

	fs.StringVar(&f.provider, "provider", "", "")
	fs.StringVar(&f.provider, "p", "", "")
	fs.StringVar(&f.model, "model", "", "")
	fs.StringVar(&f.model, "m", "", "")
	fs.StringVar(&f.baseURL, "base-url", "", "")
	fs.BoolVar(&f.verbose, "verbose", false, "")
	fs.BoolVar(&f.verbose, "v", false, "")
	fs.BoolVar(&f.version, "version", false, "")
	fs.BoolVar(&f.version, "V", false, "")
	fs.BoolVar(&f.force, "force", false, "")
	fs.BoolVar(&f.stream, "stream", false, "")
	fs.BoolVar(&f.dryRun, "dry-run", false, "")
	fs.BoolVar(&f.copy, "copy", false, "")
	fs.BoolVar(&f.copy, "c", false, "")
	fs.StringVar(&f.file, "file", "", "")
	fs.StringVar(&f.file, "f", "", "")
	fs.StringVar(&f.output, "output", "", "")
	fs.StringVar(&f.output, "o", "", "")
	fs.StringVar(&f.style, "style", "", "")
	fs.StringVar(&f.style, "s", "", "")
	fs.StringVar(&f.profile, "profile", "default", "")
	fs.IntVar(&f.count, "count", 1, "")
	fs.BoolVar(&f.json, "json", false, "")
	fs.BoolVar(&f.noArtist, "no-artist", false, "")
	fs.BoolVar(&f.noPlatform, "no-platform", false, "")
	fs.StringVar(&f.categories, "categories", "", "")
	fs.StringVar(&f.seed, "seed", "", "")
	fs.StringVar(&f.rewriteMode, "mode", "clean", "")

	args = interspersedFlagArgs(fs, args)
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if f.version {
		f.command = commandVersion
	}
	fs.Visit(func(visited *flag.Flag) {
		switch visited.Name {
		case "style", "s":
			f.styleSet = true
		case "mode":
			f.modeSet = true
		}
	})

	positional := fs.Args()
	if f.command != "" {
		f.args = positional
	} else if len(positional) > 0 {
		if isCommand(positional[0]) {
			f.command = positional[0]
			f.args = positional[1:]
		} else {
			f.args = positional
		}
	}
	if f.command == commandRun {
		if len(f.args) == 0 {
			return nil, fmt.Errorf("run requires a prompt name or alias")
		}
		f.promptName = f.args[0]
		f.args = f.args[1:]
	}
	if f.command == commandUpdate && (len(f.args) > 0 || fs.NFlag() > 0) {
		return nil, fmt.Errorf("update does not accept arguments or flags")
	}
	return f, nil
}

func isCommand(value string) bool {
	switch value {
	case "help", commandEnhance, commandCritique, commandRun, commandRewrite, commandAssemble, commandStats,
		commandFind, commandSearch, commandBrowse, commandConfig, commandStyles, commandModes, commandProviders,
		commandUpdate, commandVersion, commandInit:
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

	// Validate API key for selected provider before any network call.
	if prov.APIKey() == "" {
		return nil, fmt.Errorf("%s API key not set", providerName)
	}

	return prov, nil
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
	case commandAssemble:
		return runAssemble(f, cfg)
	case commandStats:
		return runStats(f, cfg)
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
		input = preprocessRewriteInput(input)
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
	command := f.command
	if command == "" {
		command = "enhance"
	}
	style := f.style
	if style == "" && f.command != commandCritique && f.command != commandRun {
		style = "default"
	}
	fmt.Fprintf(w, "Dry run: no API call made\n")
	fmt.Fprintf(w, "Provider: %s\n", prov.Name())
	fmt.Fprintf(w, "Model: %s\n", modelName)
	fmt.Fprintf(w, "Command: %s\n", command)
	if f.command == "rewrite" {
		fmt.Fprintf(w, "Mode: %s\n", f.rewriteMode)
	}
	if f.command == commandRun {
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

func runStats(f *flags, cfg *config.Config) error {
	lib, err := loadComponentLibrary(cfg.ComponentsFile)
	if err != nil {
		return err
	}
	stats := componentStats(lib)
	if f.json {
		data, err := json.MarshalIndent(stats, "", "  ")
		if err != nil {
			return fmt.Errorf("encode stats json: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}
	fmt.Println("Prompt Component Statistics")
	fmt.Println("===========================")
	fmt.Printf("Subjects:  %d\n", stats["subjects"])
	fmt.Printf("Modifiers: %d\n", stats["modifiers"])
	fmt.Printf("Artists:   %d\n", stats["artists"])
	fmt.Printf("Platforms: %d\n", stats["platforms"])
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

func commandRequiresInput(command string) bool {
	return command == "" || command == commandEnhance || command == commandCritique || command == commandRun || command == commandRewrite || command == commandAssemble
}

// -----------------------------------------------------------------------------
// Main
// -----------------------------------------------------------------------------

func main() {
	// Fast-path instant exit for zero-overhead version and help queries
	if len(os.Args) == 2 {
		switch os.Args[1] {
		case "version", "--version", "-V", "-v":
			printVersion(os.Stdout)
			return
		case "help", "--help", "-h":
			printUsage()
			return
		}
	}

	f, err := parseArgs(os.Args[1:])
	if err != nil {
		// ContinueOnError: -h prints usage and returns ErrHelp; other errors are
		// already printed by the FlagSet. Either way, exit cleanly.
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		os.Exit(2)
	}

	if f.command == "help" {
		if len(f.args) > 0 {
			printCommandUsageTo(os.Stderr, f.args[0])
		} else {
			printUsage()
		}
		return
	}
	if f.command == commandStyles || f.command == commandModes {
		printStyles(os.Stdout)
		return
	}
	if f.command == commandUpdate {
		if err := runUpdate(context.Background(), os.Stdout, os.Stderr, runUpdateCommand); err != nil {
			exitWithError(err)
		}
		return
	}
	if f.command == commandVersion {
		printVersion(os.Stdout)
		return
	}

	cfg, err := config.Load()
	if err != nil {
		exitWithError(err)
	}

	if f.command == commandInit {
		targetDir := ""
		if len(f.args) > 0 {
			targetDir = f.args[0]
		}
		if err := runInit(os.Stdout, os.Stderr, cfg, targetDir, f.force); err != nil {
			exitWithError(err)
		}
		return
	}

	switch f.command {
	case commandFind, commandSearch, commandBrowse:
		if err := showFinder(cfg); err != nil {
			exitWithError(err)
		}
		return
	case commandConfig:
		if isStdinPiped() || len(f.args) > 0 || f.dryRun || !isStdoutTerminal() {
			printConfig(os.Stdout, cfg)
			return
		}
		if err := RunConfigForm(cfg); err != nil {
			exitWithError(err)
		}
		return
	case commandProviders:
		printProviders(os.Stdout, cfg)
		return
	}

	if f.command == commandEnhance || f.command == commandCritique || f.command == commandRun || f.command == commandRewrite || f.command == commandAssemble {
		if len(f.args) == 0 && f.file == "" && !isStdinPiped() {
			exitWithError(fmt.Errorf("%s requires input", f.command))
		}
	}

	if commandRequiresInput(f.command) && len(f.args) == 0 && f.file == "" && !isStdinPiped() {
		if err := showFinder(cfg); err != nil {
			exitWithError(err)
		}
		return
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

// ensurePromptVault ensures the prompt directory exists, auto-seeding starter prompts if empty.
func ensurePromptVault(cfg *config.Config) ([]PromptEntry, []string, error) {
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

	entries, err := scanPromptDirs(dirs)
	if err != nil {
		return nil, nil, fmt.Errorf("scan prompts: %w", err)
	}

	if len(entries) == 0 && primaryDir != "" {
		// Auto-seed starter prompts on first interactive finder launch
		var initOut, initErr bytes.Buffer
		if initErrVal := runInit(&initOut, &initErr, cfg, primaryDir, false); initErrVal == nil {
			fmt.Fprintf(os.Stderr, "Seeded starter prompts in %s\n", primaryDir)
			entries, _ = scanPromptDirs(dirs)
		}
	}

	return entries, dirs, nil
}

// showFinder scans the prompts directory and shows the fuzzy finder.
func showFinder(cfg *config.Config) error {
	entries, dirs, err := ensurePromptVault(cfg)
	if err != nil {
		return err
	}

	if len(entries) == 0 {
		fmt.Fprintf(os.Stderr, "No prompts found in %s\n\nRun 'prompter init' to install starter prompts or add .md files.\n", strings.Join(dirs, ", "))
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
