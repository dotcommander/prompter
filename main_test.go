package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/dotcommander/prompter/internal/config"
	"github.com/dotcommander/prompter/internal/provider"
)

// Test-only adapters preserve the existing call sites while keeping production
// entry points and output wiring in their owning files.
func parseArgs(args []string) (*flags, error) {
	return parseArgsTo(args, os.Stderr)
}

func newLogger(verbose bool) *slog.Logger {
	return newLoggerTo(os.Stderr, verbose)
}

func run(ctx context.Context, f *flags, cfg *config.Config, logger *slog.Logger) error {
	return runWithOutput(ctx, f, cfg, logger, commandOutput{stdout: os.Stdout, stderr: os.Stderr})
}

// -----------------------------------------------------------------------------
// Provider Interface Tests
// -----------------------------------------------------------------------------

func TestProvider_Name(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		prov provider.Provider
		want string
	}{
		{"openai", provider.NewOpenAI("key", "model", "", 3, 4096), "openai"},
		{"cerebras", provider.NewChat("cerebras", "key", "model", "http://test", 3), "cerebras"},
		{"groq", provider.NewChat("groq", "key", "model", "http://test", 3), "groq"},
		{"openrouter", provider.NewChat("openrouter", "key", "model", "http://test", 3), "openrouter"},
		{"zai", provider.NewChat("zai", "key", "model", "http://test", 3), "zai"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			t.Parallel()
			if got := tt.prov.Name(); got != tt.want {
				t.Errorf("Name() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestProvider_Model(t *testing.T) {
	t.Parallel()
	prov := provider.NewChat("test", "key", "test-model", "http://test", 3)
	if got := prov.Model(); got != "test-model" {
		t.Errorf("Model() = %q, want %q", got, "test-model")
	}
}

// -----------------------------------------------------------------------------
// Input Reader Tests
// -----------------------------------------------------------------------------

func TestParseArgs_NewIOFlags(t *testing.T) {
	t.Parallel()

	f, err := parseArgs([]string{
		"refine",
		"--dry-run",
		"-f", "input.txt",
		"-o", "output.txt",
		"--stream",
	})
	if err != nil {
		t.Fatalf("parseArgs error: %v", err)
	}
	if !f.dryRun {
		t.Error("dryRun = false, want true")
	}
	if f.file != "input.txt" {
		t.Errorf("file = %q, want input.txt", f.file)
	}
	if f.output != "output.txt" {
		t.Errorf("output = %q, want output.txt", f.output)
	}
	if !f.stream {
		t.Error("stream = false, want true")
	}
	if f.command != commandRefine {
		t.Errorf("command = %q, want refine", f.command)
	}
}

func TestRootArgsDefaultEnrichment(t *testing.T) {
	t.Parallel()

	if args := rootArgs(nil, true); !reflect.DeepEqual(args, []string{commandRefine}) {
		t.Fatalf("rootArgs(nil, true) = %v, want [%s]", args, commandRefine)
	}
	if args := rootArgs(nil, false); args != nil {
		t.Fatalf("rootArgs(nil, false) = %v, want nil", args)
	}
	if args := rootArgs([]string{"rough text"}, false); !reflect.DeepEqual(args, []string{commandRefine, "rough text"}) {
		t.Fatalf("positional input = %v, want refine prefix", args)
	}
	flags := []string{"--provider", "openai", "--style", "concise"}
	if args := rootArgs(flags, true); !reflect.DeepEqual(args, append([]string{commandRefine}, flags...)) {
		t.Fatalf("rootArgs(flags, true) = %v, want refine plus %v", args, flags)
	}
	for _, metadataFlag := range []string{"--help", "-h", "--version", "-V"} {
		if args := rootArgs([]string{metadataFlag}, true); !reflect.DeepEqual(args, []string{metadataFlag}) {
			t.Errorf("rootArgs(%q, true) = %v, want root metadata flag unchanged", metadataFlag, args)
		}
	}
	for _, operation := range []string{commandRefine, imageOperationFlag, configOperationFlag} {
		if args := rootArgs([]string{operation}, false); !reflect.DeepEqual(args, []string{operation}) {
			t.Errorf("rootArgs(%q, false) = %v, want unchanged", operation, args)
		}
	}
	if args := rootArgs([]string{"--", "critique"}, false); !reflect.DeepEqual(args, []string{commandRefine, "--", "critique"}) {
		t.Fatalf("literal boundary = %v, want refine prefix preserving --", args)
	}
	// Retired words pass through untouched so parsing returns the migration error.
	if args := rootArgs([]string{"critique"}, true); !reflect.DeepEqual(args, []string{"critique"}) {
		t.Fatalf("retired word = %v, want unchanged for migration error", args)
	}
}

func TestParseArgs_ImageFlags(t *testing.T) {
	t.Parallel()

	f, err := parseArgs([]string{
		imageOperationFlag,
		"--profile", "minimal",
		"--count", "2",
		"--json",
		"--no-artist",
		"--categories", "quality,style",
		"--seed", "test-seed",
		"moon castle",
	})
	if err != nil {
		t.Fatalf("parseArgs error: %v", err)
	}
	if f.command != commandImage {
		t.Fatalf("command = %q, want image", f.command)
	}
	if f.profile != "minimal" || f.count != 2 || !f.json || !f.noArtist || f.categories != "quality,style" || f.seed != "test-seed" {
		t.Fatalf("image flags not parsed correctly: %+v", f)
	}
	if strings.Join(f.args, " ") != "moon castle" {
		t.Fatalf("args = %v, want moon castle", f.args)
	}
}

func TestParseArgs_ShortFlags(t *testing.T) {
	t.Parallel()

	f, err := parseArgs([]string{"refine", "-p", "openai", "-m", "gpt-test", "-c", "-f", "input.txt"})
	if err != nil {
		t.Fatalf("parseArgs error: %v", err)
	}
	if f.provider != "openai" || f.model != "gpt-test" || !f.copy || f.file != "input.txt" {
		t.Fatalf("short flags not parsed correctly: %+v", f)
	}
}

func TestParseArgs_CommandBeforeFlags(t *testing.T) {
	t.Parallel()

	f, err := parseArgs([]string{"refine", "--file", "notes.md", "--style", "code"})
	if err != nil {
		t.Fatalf("parseArgs error: %v", err)
	}
	if f.command != commandRefine || f.file != "notes.md" || f.style != "code" {
		t.Fatalf("command-first flags not parsed correctly: %+v", f)
	}
}

func TestParseArgs_InterspersedFlags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		args        []string
		wantArgs    string
		wantOutput  string
		wantProfile string
		wantCount   int
		wantJSON    bool
	}{
		{
			name:       "output after prompt",
			args:       []string{"refine", "write a release checklist", "-o", "prompt.txt"},
			wantArgs:   "write a release checklist",
			wantOutput: "prompt.txt",
		},
		{
			name:        "assembly flags after subject",
			args:        []string{imageOperationFlag, "portrait of a clockmaker", "--profile", "minimal", "--count", "3", "--json"},
			wantArgs:    "portrait of a clockmaker",
			wantProfile: "minimal",
			wantCount:   3,
			wantJSON:    true,
		},
		{
			name:     "default enrichment with flags after text",
			args:     []string{"write a release checklist", "--dry-run"},
			wantArgs: "write a release checklist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f, err := parseArgs(tt.args)
			if err != nil {
				t.Fatalf("parseArgs error: %v", err)
			}
			if f.command != commandRefine && f.command != commandImage {
				t.Fatalf("command = %q", f.command)
			}
			if got := strings.Join(f.args, " "); got != tt.wantArgs {
				t.Fatalf("args = %q, want %q", got, tt.wantArgs)
			}
			if f.output != tt.wantOutput || f.profile != tt.wantProfile || f.count != tt.wantCount || f.json != tt.wantJSON {
				t.Fatalf("parsed flags = %+v", f)
			}
		})
	}
}

func TestParseArgs_LiteralInputBoundary(t *testing.T) {
	t.Parallel()

	f, err := parseArgs([]string{"refine", "--copy", "--", "-literal", "--json"})
	if err != nil {
		t.Fatalf("parseArgs error: %v", err)
	}
	if !f.copy {
		t.Fatal("copy = false, want true")
	}
	if f.json {
		t.Fatal("json = true, want false after --")
	}
	if got := strings.Join(f.args, " "); got != "-literal --json" {
		t.Fatalf("args = %q, want literal input", got)
	}
}

func TestParseArgs_LiteralRetiredWordAfterBoundary(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{
		{"--", "critique"},
		{"refine", "--", "critique"},
		{"--", "--image"},
	} {
		f, err := parseArgs(args)
		if err != nil {
			t.Fatalf("parseArgs(%q) error: %v", args, err)
		}
		if f.command != commandRefine {
			t.Fatalf("parseArgs(%q) command = %q, want refine", args, f.command)
		}
		want := args[len(args)-1]
		if got := strings.Join(f.args, " "); got != want {
			t.Fatalf("parseArgs(%q) args = %q, want the literal input %q", args, got, want)
		}
	}
}

func TestParseArgs_DefaultEnrichment(t *testing.T) {
	t.Parallel()

	f, err := parseArgs([]string{"rough prompt text"})
	if err != nil {
		t.Fatalf("parseArgs error: %v", err)
	}
	if f.command != commandRefine {
		t.Fatalf("command = %q, want refine", f.command)
	}
	if got := strings.Join(f.args, " "); got != "rough prompt text" {
		t.Fatalf("args = %q, want rough prompt text", got)
	}
}

func TestParseArgs_ConfigOperation(t *testing.T) {
	t.Parallel()

	f, err := parseArgs([]string{configOperationFlag})
	if err != nil {
		t.Fatalf("parseArgs error: %v", err)
	}
	if f.command != commandConfig {
		t.Fatalf("command = %q, want config", f.command)
	}
	if len(f.args) != 0 {
		t.Fatalf("args = %v, want empty", f.args)
	}
}

func TestParseArgs_MissingInterspersedFlagValue(t *testing.T) {
	t.Parallel()

	if _, err := parseArgs([]string{"refine", "prompt", "--output"}); err == nil {
		t.Fatal("parseArgs expected a missing output value error")
	}
}

func TestParseArgs_RetiredCommandsReturnMigrationError(t *testing.T) {
	t.Parallel()

	for _, command := range []string{"critique", "rewrite", "apply", "browse", "image", "configure", "config", "models", "prompts"} {
		t.Run(command, func(t *testing.T) {
			t.Parallel()
			_, err := parseArgs([]string{command, "some text"})
			if err == nil {
				t.Fatalf("parseArgs(%q) expected a migration error", command)
			}
			if !strings.Contains(err.Error(), "removed") {
				t.Fatalf("parseArgs(%q) error = %v, want removal notice", command, err)
			}
			switch command {
			case "image":
				if !strings.Contains(err.Error(), imageOperationFlag) {
					t.Errorf("image migration error = %v, want %s mapping", err, imageOperationFlag)
				}
			case "configure", "config":
				if !strings.Contains(err.Error(), configOperationFlag) {
					t.Errorf("configure migration error = %v, want %s mapping", err, configOperationFlag)
				}
			}
		})
	}
}

func TestParseArgs_RejectsOperationCollisions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want string
	}{
		{"image with config", []string{imageOperationFlag, configOperationFlag}, "cannot be combined"},
		{"config with image", []string{configOperationFlag, imageOperationFlag}, "cannot be combined"},
		{"refine with image", []string{commandRefine, imageOperationFlag, "x"}, "cannot be combined"},
		{"refine with config", []string{commandRefine, configOperationFlag}, "cannot be combined"},
		{"image with refine", []string{imageOperationFlag, commandRefine, "x"}, "refine"},
		{"image control after bool flag", []string{commandRefine, "--verbose", imageOperationFlag}, "cannot be combined"},
		{"config control after file flag with value", []string{commandRefine, "--file", "notes.md", "--config"}, "cannot be combined"},
		{"config with input", []string{configOperationFlag, "extra"}, "does not accept input"},
		{"config with literal input", []string{configOperationFlag, "--", "extra"}, "does not accept input"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := parseArgs(tt.args)
			if err == nil {
				t.Fatalf("parseArgs(%q) expected an error", tt.args)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("parseArgs(%q) error = %v, want containing %q", tt.args, err, tt.want)
			}
		})
	}
}

func TestParseArgs_FlagValuesAreNotOperationControls(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		args      []string
		wantCmd   string
		wantFile  string
		wantStyle string
		wantSeed  string
	}{
		{name: "config word as file value", args: []string{commandRefine, "--file", "--config"}, wantCmd: commandRefine, wantFile: "--config"},
		{name: "image flag as style value", args: []string{"--style", "--image"}, wantCmd: commandRefine, wantStyle: "--image"},
		{name: "config flag as inline style value", args: []string{"--style=--config"}, wantCmd: commandRefine, wantStyle: "--config"},
		{name: "config flag as image seed value", args: []string{imageOperationFlag, "--seed", "--config"}, wantCmd: commandImage, wantSeed: "--config"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f, err := parseArgs(tt.args)
			if err != nil {
				t.Fatalf("parseArgs(%q) error: %v", tt.args, err)
			}
			if f.command != tt.wantCmd {
				t.Fatalf("parseArgs(%q) command = %q, want %q", tt.args, f.command, tt.wantCmd)
			}
			if f.file != tt.wantFile || f.style != tt.wantStyle || f.seed != tt.wantSeed {
				t.Fatalf("parseArgs(%q) parsed flags = %+v", tt.args, f)
			}
		})
	}
}

func TestParseArgs_HelpValueDoesNotTriggerUsage(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer
	f, err := parseArgsTo([]string{commandRefine, "--file", "-h"}, &stderr)
	if err != nil {
		t.Fatalf("parseArgsTo error: %v", err)
	}
	if f.file != "-h" {
		t.Fatalf("file = %q, want %q", f.file, "-h")
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want no usage output for a flag value", stderr.String())
	}
}

func TestParseArgs_MetadataFlagCombinedWithArguments(t *testing.T) {
	t.Parallel()

	if _, err := parseArgs([]string{"--help", "refine"}); err == nil || !strings.Contains(err.Error(), "cannot be combined") {
		t.Fatalf("parseArgs error = %v, want metadata flag conflict", err)
	}
}

func TestCLIInputReader(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "single arg",
			args: []string{"test prompt"},
			want: "test prompt",
		},
		{
			name: "multiple args joined",
			args: []string{"test", "prompt", "here"},
			want: "test prompt here",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			reader := &CLIInputReader{args: tt.args}
			got, err := reader.Read()
			if err != nil {
				t.Fatalf("Read() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Read() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestReadInput_PositionalSizeLimit(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		args      []string
		wantError bool
	}{
		{name: "exactly limit", args: []string{strings.Repeat("x", maxInputBytes)}},
		{name: "single oversized argument", args: []string{strings.Repeat("x", maxInputBytes+1)}, wantError: true},
		{name: "separator exceeds limit", args: []string{strings.Repeat("x", maxInputBytes), "y"}, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := readInput("", tc.args)
			if tc.wantError {
				if err == nil || !strings.Contains(err.Error(), "input exceeds") {
					t.Fatalf("readInput error = %v, want size limit", err)
				}
				return
			}
			if err != nil || len(got) != maxInputBytes {
				t.Fatalf("readInput length = %d, error = %v", len(got), err)
			}
		})
	}
}

func TestRunRejectsOversizedPositionalInputBeforeProviderCall(t *testing.T) {
	t.Parallel()
	f := &flags{command: commandRefine, args: []string{strings.Repeat("x", maxInputBytes+1)}}
	cfg := &config.Config{Provider: "groq", Providers: map[string]config.ProviderConfig{
		"groq": {Model: "model", APIKey: "key"},
	}}
	err := runWithOutput(context.Background(), f, cfg, newLogger(false), commandOutput{stdout: &strings.Builder{}, stderr: &strings.Builder{}})
	if err == nil || !strings.Contains(err.Error(), "input exceeds") {
		t.Fatalf("runWithOutput error = %v, want positional size limit", err)
	}
}

func TestReadInput_File(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "prompt.txt")
	if err := os.WriteFile(inputPath, []byte("  file prompt  \n"), 0644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	got, err := readInput(inputPath, []string{"arg prompt"})
	if err != nil {
		t.Fatalf("readInput error: %v", err)
	}
	if got != "file prompt" {
		t.Errorf("readInput = %q, want file prompt", got)
	}
}

func TestReadInput_FileSizeLimit(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "large.txt")
	if err := os.WriteFile(inputPath, bytes.Repeat([]byte("a"), maxInputBytes+1), 0644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	_, err := readInput(inputPath, nil)
	if err == nil {
		t.Fatal("readInput expected size error, got nil")
	}
	if !strings.Contains(err.Error(), "input exceeds") {
		t.Fatalf("readInput error = %v, want input exceeds", err)
	}
}

func TestWriteOutput(t *testing.T) {
	t.Parallel()
	outputPath := filepath.Join(t.TempDir(), "nested", "prompt.txt")

	if err := writeOutput(outputPath, "enhanced prompt"); err != nil {
		t.Fatalf("writeOutput error: %v", err)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if string(data) != "enhanced prompt" {
		t.Errorf("output = %q, want enhanced prompt", string(data))
	}
}

func TestPrintDryRun(t *testing.T) {
	t.Parallel()
	var out strings.Builder
	prov := provider.NewChat("groq", "key", "model-a", "http://test", 3)
	f := &flags{
		command: commandRefine,
		dryRun:  true,
		style:   "code",
		file:    "input.txt",
		output:  "output.txt",
	}
	cfg := &config.Config{
		Effort:          "low",
		MaxOutputTokens: 4096,
		SystemPrompt:    "system prompt",
	}

	printDryRun(&out, prov, "model-b", f, cfg, "prompt", 60*time.Second)

	got := out.String()
	for _, want := range []string{
		"Dry run: no API call made",
		"Provider: groq",
		"Model: model-b",
		"Base URL: default",
		"Credential source: GROQ_API_KEY",
		"Command: refine",
		"Style: code",
		"Max output tokens: 4096",
		"Max retries: 0",
		"System prompt bytes: 13",
		"Input bytes: 6",
		"Input file: input.txt",
		"Output file: output.txt",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("dry run output missing %q:\n%s", want, got)
		}
	}
}

func TestPrintDryRunDefaultStyle(t *testing.T) {
	t.Parallel()

	var out strings.Builder
	prov := provider.NewChat("groq", "key", "model-a", "http://test", 3)
	f := &flags{command: commandRefine, dryRun: true}
	cfg := &config.Config{MaxOutputTokens: 4096}

	printDryRun(&out, prov, "model-a", f, cfg, "prompt", time.Minute)

	if !strings.Contains(out.String(), "Style: default") {
		t.Fatalf("dry run output missing default style:\n%s", out.String())
	}
}

func TestDryRunCredentialSourceDoesNotExposeCredential(t *testing.T) {
	t.Parallel()
	if got := dryRunCredentialSource("groq", config.ProviderConfig{APIKey: "secret"}); got != "config-or-GROQ_API_KEY" {
		t.Fatalf("standard credential source = %q", got)
	}
	if got := dryRunCredentialSource("groq", config.ProviderConfig{APIKey: "secret", KeyEnv: "CUSTOM_KEY"}); got != "CUSTOM_KEY" {
		t.Fatalf("custom credential source = %q", got)
	}
	if got := dryRunCredentialSource("gemini", config.ProviderConfig{}); got != "google-adc" {
		t.Fatalf("Gemini credential source = %q", got)
	}
}

func TestPrintUsageAdvertisesOnlyNewInterface(t *testing.T) {
	t.Parallel()
	var out strings.Builder

	printUsageTo(&out)

	got := out.String()
	for _, want := range []string{"refine [input]", imageOperationFlag, configOperationFlag, "--version"} {
		if !strings.Contains(got, want) {
			t.Errorf("usage output missing %q:\n%s", want, got)
		}
	}
	for _, retired := range []string{"critique", "rewrite", "apply", "browse", "models refresh", "prompts status", "configure "} {
		if strings.Contains(got, retired) {
			t.Errorf("usage output contains retired operation %q:\n%s", retired, got)
		}
	}
}

func TestAssemblePromptDeterministic(t *testing.T) {
	t.Parallel()

	lib, err := loadComponentLibrary("")
	if err != nil {
		t.Fatalf("loadComponentLibrary error: %v", err)
	}
	profile, err := assemblyProfile("default")
	if err != nil {
		t.Fatalf("assemblyProfile error: %v", err)
	}
	got1, err := assemblePrompt(lib, "portrait of a clockmaker", profile, nil, "seed")
	if err != nil {
		t.Fatalf("assemblePrompt error: %v", err)
	}
	got2, err := assemblePrompt(lib, "portrait of a clockmaker", profile, nil, "seed")
	if err != nil {
		t.Fatalf("assemblePrompt second error: %v", err)
	}
	if got1.FullPrompt == "" {
		t.Fatal("FullPrompt is empty")
	}
	if got1.FullPrompt != got2.FullPrompt {
		t.Fatalf("assemblePrompt not deterministic:\n%s\n%s", got1.FullPrompt, got2.FullPrompt)
	}
	for _, want := range []string{"portrait of a clockmaker", "by "} {
		if !strings.Contains(got1.FullPrompt, want) {
			t.Fatalf("assembled prompt missing %q: %s", want, got1.FullPrompt)
		}
	}
}

func TestAssemblePromptCustomCategories(t *testing.T) {
	t.Parallel()

	lib, err := loadComponentLibrary("")
	if err != nil {
		t.Fatalf("loadComponentLibrary error: %v", err)
	}
	profile, err := assemblyProfile("minimal")
	if err != nil {
		t.Fatalf("assemblyProfile error: %v", err)
	}
	got, err := assemblePrompt(lib, "desert observatory", profile, []string{"composition"}, "seed")
	if err != nil {
		t.Fatalf("assemblePrompt error: %v", err)
	}
	if got.Profile != "custom" {
		t.Fatalf("Profile = %q, want custom", got.Profile)
	}
	if len(got.Modifiers) != 1 || got.Modifiers[0].Category != "composition" {
		t.Fatalf("modifiers = %+v, want one composition modifier", got.Modifiers)
	}
}

func TestAssemblePromptRejectsUnknownAndDuplicateCategories(t *testing.T) {
	t.Parallel()
	lib, err := loadComponentLibrary("")
	if err != nil {
		t.Fatalf("loadComponentLibrary: %v", err)
	}
	profile, err := assemblyProfile("default")
	if err != nil {
		t.Fatalf("assemblyProfile: %v", err)
	}

	for name, categories := range map[string][]string{
		"unknown":   {"not-a-category"},
		"duplicate": {"quality", "quality"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := assemblePrompt(lib, "subject", profile, categories, "seed"); err == nil {
				t.Fatalf("assemblePrompt(%v) expected validation error", categories)
			}
		})
	}
}

func TestDefaultComponentLibraryHasEachComponentType(t *testing.T) {
	t.Parallel()

	lib, err := loadComponentLibrary("")
	if err != nil {
		t.Fatalf("loadComponentLibrary error: %v", err)
	}
	if len(lib.Subjects) == 0 || len(lib.Modifiers) == 0 || len(lib.Artists) == 0 || len(lib.Platforms) == 0 {
		t.Fatalf("default component library is missing a component type")
	}
	if !slices.ContainsFunc(lib.Modifiers, func(m PromptModifier) bool { return m.Category == "quality" }) {
		t.Fatal("default component library has no quality modifiers")
	}
}

func TestLoadComponentLibraryConfiguredFile(t *testing.T) {
	t.Parallel()

	t.Run("empty file is rejected", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "components.json")
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatalf("write empty components file: %v", err)
		}

		_, err := loadComponentLibrary(path)
		if err == nil || !strings.Contains(err.Error(), "parse components") {
			t.Fatalf("loadComponentLibrary(%q) error = %v, want parse components error", path, err)
		}
	})

	t.Run("missing file uses defaults", func(t *testing.T) {
		t.Parallel()

		lib, err := loadComponentLibrary(filepath.Join(t.TempDir(), "missing-components.json"))
		if err != nil {
			t.Fatalf("loadComponentLibrary missing file: %v", err)
		}
		if len(lib.Modifiers) == 0 {
			t.Fatal("missing file library has no default modifiers")
		}
	})

	t.Run("oversized file is rejected", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "components.json")
		if err := os.WriteFile(path, bytes.Repeat([]byte("a"), maxInputBytes+1), 0o600); err != nil {
			t.Fatalf("write oversized components file: %v", err)
		}

		_, err := loadComponentLibrary(path)
		if err == nil {
			t.Fatal("loadComponentLibrary expected size error, got nil")
		}
		for _, want := range []string{"read components file", "1 MB limit"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("loadComponentLibrary error = %v, want containing %q", err, want)
			}
		}
	})

	t.Run("custom file is preserved", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "components.json")
		data := []byte(`{"modifiers":[{"text":"custom modifier","category":"quality","weight":1}]}`)
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatalf("write custom components file: %v", err)
		}

		lib, err := loadComponentLibrary(path)
		if err != nil {
			t.Fatalf("loadComponentLibrary custom file: %v", err)
		}
		if len(lib.Modifiers) != 1 || lib.Modifiers[0].Text != "custom modifier" {
			t.Fatalf("custom modifiers = %+v, want custom modifier", lib.Modifiers)
		}
	})
}

func TestResolveStyle_Spec(t *testing.T) {
	t.Parallel()

	prompt, err := resolveStyle("spec")
	if err != nil {
		t.Fatalf("resolveStyle(spec) error: %v", err)
	}
	for _, want := range []string{
		"Autonomous Output Contract",
		"DAWN Structure Pass",
		"Final Output Contract",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("spec style missing %q", want)
		}
	}
}

func TestResolveStyleUserOverride(t *testing.T) {
	t.Parallel()

	stylesDir := t.TempDir()
	want := "custom style prompt"
	if err := os.WriteFile(filepath.Join(stylesDir, "custom.md"), []byte(want), 0600); err != nil {
		t.Fatalf("write custom style override: %v", err)
	}

	got, err := resolveStyleFromDir("custom", stylesDir)
	if err != nil {
		t.Fatalf("resolveStyleFromDir(custom) error: %v", err)
	}
	if got != want {
		t.Fatalf("resolveStyleFromDir(custom) = %q, want %q", got, want)
	}
}

func TestResolveStyleUserOverrideSizeLimit(t *testing.T) {
	t.Parallel()

	stylesDir := t.TempDir()
	path := filepath.Join(stylesDir, "custom.md")
	if err := os.WriteFile(path, bytes.Repeat([]byte("a"), maxInputBytes+1), 0600); err != nil {
		t.Fatalf("write oversized style override: %v", err)
	}

	_, err := resolveStyleFromDir("custom", stylesDir)
	if err == nil {
		t.Fatal("resolveStyleFromDir expected size error, got nil")
	}
	for _, want := range []string{"read style file", "1 MB limit"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("resolveStyleFromDir error = %v, want containing %q", err, want)
		}
	}
}

func TestResolveStyle_RejectsPathSeparators(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"../enhance", "sub/dir", `a\b`} {
		_, err := resolveStyle(name)
		if err == nil || !strings.Contains(err.Error(), "invalid style name") {
			t.Errorf("resolveStyle(%q) error = %v, want invalid style name", name, err)
		}
	}
}

func TestLoadSystemPrompt_PromptFileSizeLimit(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "big-prompt.md")
	if err := os.WriteFile(path, bytes.Repeat([]byte("a"), maxInputBytes+1), 0600); err != nil {
		t.Fatalf("write oversized prompt file: %v", err)
	}

	cfg := &config.Config{PromptFile: path}
	err := loadSystemPrompt(cfg)
	if err == nil {
		t.Fatal("loadSystemPrompt expected size error, got nil")
	}
	for _, want := range []string{"read prompt file", "1 MB limit"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("loadSystemPrompt error = %v, want containing %q", err, want)
		}
	}
	if cfg.SystemPrompt != "" {
		t.Errorf("SystemPrompt = %q, want empty on failure", cfg.SystemPrompt)
	}
}

func TestResolveStyle_UnknownListsValidStyles(t *testing.T) {
	t.Parallel()

	_, err := resolveStyle("missing-style")
	if err == nil {
		t.Fatal("resolveStyle expected error, got nil")
	}
	got := err.Error()
	for _, want := range []string{"missing-style", "valid:", "default", "spec"} {
		if !strings.Contains(got, want) {
			t.Errorf("resolveStyle error missing %q: %v", want, err)
		}
	}
}

func TestAvailableStylesStableOrder(t *testing.T) {
	t.Parallel()

	got := availableStyles()
	want := []string{"default", "code", "concise", "creative", "spec"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("availableStyles = %v, want %v", got, want)
	}
}

// -----------------------------------------------------------------------------
// Resolve Provider Tests
// -----------------------------------------------------------------------------

func TestResolveProvider(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Providers: map[string]config.ProviderConfig{
			"openai":     {APIKey: "key"},
			"cerebras":   {APIKey: "key", BaseURL: "http://test"},
			"groq":       {APIKey: "key", BaseURL: "http://test"},
			"openrouter": {APIKey: "key", BaseURL: "http://test"},
			"zai":        {APIKey: "key", BaseURL: "http://test"},
			"omlx":       {Model: "Ornith-1.5-35B-A3B-oQ4e-mtp", BaseURL: "http://127.0.0.1:8000/v1"},
		},
	}
	tests := []struct {
		name         string
		providerName string
		wantName     string
		wantErr      bool
	}{
		{"valid openai", "openai", "openai", false},
		{"valid cerebras", "cerebras", "cerebras", false},
		{"valid groq", "groq", "groq", false},
		{"valid openrouter", "openrouter", "openrouter", false},
		{"valid zai", "zai", "zai", false},
		{"valid gemini", "gemini", "gemini", false},
		{"valid omlx", "omlx", "omlx", false},
		{"unknown provider", "unknown", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			prov, err := resolveProvider(cfg, tt.providerName, "")
			if tt.wantErr {
				if err == nil {
					t.Error("resolveProvider() expected error, got nil")
				}
				for _, want := range []string{"unknown provider", "valid:", "openai"} {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("resolveProvider error missing %q: %v", want, err)
					}
				}
				return
			}
			if err != nil {
				t.Errorf("resolveProvider() unexpected error: %v", err)
				return
			}
			if prov.Name() != tt.wantName {
				t.Errorf("resolveProvider().Name() = %q, want %q", prov.Name(), tt.wantName)
			}
		})
	}
}

func TestResolveProviderDefersMissingRemoteAPIKeyValidation(t *testing.T) {
	t.Parallel()

	prov, err := resolveProvider(&config.Config{
		Providers: map[string]config.ProviderConfig{
			"groq": {Model: "model", BaseURL: "http://test"},
		},
	}, "groq", "")
	if err != nil {
		t.Fatalf("resolveProvider error = %v, want metadata resolution to succeed", err)
	}
	if err := validateProviderCredentials(prov); err == nil || err.Error() != "groq API key not set" {
		t.Fatalf("validateProviderCredentials error = %v, want missing groq API key", err)
	}
}

func TestRunDryRunDoesNotRequireAPIKey(t *testing.T) {
	t.Parallel()
	f := &flags{command: commandRefine, dryRun: true, provider: "groq", args: []string{"input"}}
	cfg := &config.Config{
		Provider:        "groq",
		Effort:          "low",
		Timeout:         60,
		MaxOutputTokens: 4096,
		Providers: map[string]config.ProviderConfig{
			"groq": {Model: "model", BaseURL: "http://test"},
		},
	}
	if err := run(context.Background(), f, cfg, newLogger(false)); err != nil {
		t.Fatalf("dry run with missing API key: %v", err)
	}
}

func TestRunRejectsOutputWithStreamBeforeProviderResolution(t *testing.T) {
	t.Parallel()

	err := run(
		context.Background(),
		&flags{stream: true, output: "out.txt"},
		&config.Config{},
		newLogger(false),
	)
	if err == nil {
		t.Fatal("run expected error, got nil")
	}
	if err.Error() != "--output cannot be used with --stream" {
		t.Fatalf("run error = %q, want stream/output validation", err)
	}
}

func TestRunRejectsCopyWithStreamBeforeProviderResolution(t *testing.T) {
	t.Parallel()

	err := run(
		context.Background(),
		&flags{stream: true, copy: true},
		&config.Config{},
		newLogger(false),
	)
	if err == nil {
		t.Fatal("run expected error, got nil")
	}
	if err.Error() != "--copy cannot be used with --stream" {
		t.Fatalf("run error = %q, want stream/copy validation", err)
	}
}

func TestRunRejectsDefaultCopyWithStreamBeforeProviderResolution(t *testing.T) {
	t.Parallel()

	err := run(
		context.Background(),
		&flags{stream: true},
		&config.Config{DefaultCopy: true},
		newLogger(false),
	)
	if err == nil || err.Error() != "--copy cannot be used with --stream" {
		t.Fatalf("run error = %v, want stream/copy validation", err)
	}
}

func TestResolveCommandSystemPrompt_LocalOperationsSkipPromptFile(t *testing.T) {
	t.Parallel()

	for _, command := range []string{commandImage, commandConfig} {
		t.Run(command, func(t *testing.T) {
			t.Parallel()
			cfg := &config.Config{PromptFile: filepath.Join(t.TempDir(), "missing.md")}
			if err := resolveCommandSystemPrompt(&flags{command: command}, cfg); err != nil {
				t.Fatalf("resolveCommandSystemPrompt(%q): %v", command, err)
			}
			if cfg.SystemPrompt != "" {
				t.Fatalf("SystemPrompt = %q, want empty", cfg.SystemPrompt)
			}
		})
	}
}

func TestResolveCommandSystemPrompt_RemoteCommandLoadsPromptFile(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{PromptFile: filepath.Join(t.TempDir(), "missing.md")}
	if err := resolveCommandSystemPrompt(&flags{command: commandRefine}, cfg); err == nil {
		t.Fatal("resolveCommandSystemPrompt expected missing prompt error")
	}
}

func TestResolveCommandSystemPromptRefineStyle(t *testing.T) {
	t.Parallel()
	f := &flags{command: commandRefine, style: "concise", styleSet: true}
	cfg := &config.Config{}
	if err := resolveCommandSystemPrompt(f, cfg); err != nil {
		t.Fatalf("resolveCommandSystemPrompt error: %v", err)
	}
	if cfg.SystemPrompt == "" {
		t.Fatal("SystemPrompt is empty")
	}
}

func TestNewLogger(t *testing.T) {
	t.Parallel()
	logger := newLogger(false)
	if logger == nil {
		t.Error("newLogger(false) returned nil")
	}

	verboseLogger := newLogger(true)
	if verboseLogger == nil {
		t.Error("newLogger(true) returned nil")
	}
}

func TestPrintCommandUsage(t *testing.T) {
	t.Parallel()
	for _, cmd := range []string{commandRefine, commandImage, commandConfig} {
		var out bytes.Buffer
		printCommandUsageTo(&out, cmd)
		got := out.String()
		if !strings.Contains(got, "Usage:") {
			t.Errorf("printCommandUsageTo(%q) missing Usage: header:\n%s", cmd, got)
		}
		if !strings.Contains(got, cmd) {
			t.Errorf("printCommandUsageTo(%q) missing command name %q:\n%s", cmd, cmd, got)
		}
	}
}

func TestParseArgs_PublicOperations(t *testing.T) {
	t.Parallel()
	tests := [][]string{{"refine"}, {imageOperationFlag}, {configOperationFlag}, {"rough text"}}
	for _, args := range tests {
		f, err := parseArgs(args)
		if err != nil {
			t.Fatalf("parseArgs(%q) error: %v", args, err)
		}
		want := commandRefine
		if args[0] == imageOperationFlag {
			want = commandImage
		} else if args[0] == configOperationFlag {
			want = commandConfig
		}
		if f.command != want {
			t.Errorf("parseArgs(%q) command = %q, want %q", args, f.command, want)
		}
	}
}

func TestRunConfig(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Provider: "gemini",
		Timeout:  60,
		Providers: map[string]config.ProviderConfig{
			"gemini": {Model: "gemini-3.7-flash"},
		},
	}
	var out strings.Builder
	printConfig(&out, cfg)
	if !strings.Contains(out.String(), "gemini-3.7-flash") {
		t.Fatalf("printConfig output missing model:\n%s", out.String())
	}
}

func TestAuthKeyLine(t *testing.T) {
	tests := []struct {
		name     string
		env      map[string]string
		provider string
		settings config.ProviderConfig
		want     string
	}{
		{name: "omlx is keyless", provider: "omlx", want: "Auth Key:          $OMLX_API_KEY (local server / keyless)"},
		{name: "gemini key detected", env: map[string]string{"GEMINI_API_KEY": "AIza-test"}, provider: "gemini", want: "Auth Key:          $GEMINI_API_KEY (detected ✓)"},
		{name: "gemini falls back to adc", env: map[string]string{"GEMINI_API_KEY": ""}, provider: "gemini", want: "Auth Key:          Google ADC (not checked)"},
		{name: "default env detected", env: map[string]string{"GROQ_API_KEY": "gsk-test"}, provider: "groq", want: "Auth Key:          $GROQ_API_KEY (detected ✓)"},
		{name: "default env missing", env: map[string]string{"GROQ_API_KEY": ""}, provider: "groq", want: "Auth Key:          $GROQ_API_KEY (not set ✗)"},
		{name: "custom key env detected", env: map[string]string{"MY_KEY": "x"}, provider: "openai", settings: config.ProviderConfig{KeyEnv: "MY_KEY"}, want: "Auth Key:          $MY_KEY (detected ✓)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for key, value := range tt.env {
				t.Setenv(key, value)
			}
			if got := authKeyLine(tt.provider, tt.settings); got != tt.want {
				t.Fatalf("authKeyLine(%q) = %q, want %q", tt.provider, got, tt.want)
			}
		})
	}
}

func TestMetadataOutputRedactsURLUserinfo(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Provider: "groq",
		Providers: map[string]config.ProviderConfig{
			"groq": {BaseURL: "https://user:secret@example.com/v1?api_key=query-secret&region=us#fragment-secret"},
		},
	}
	var out strings.Builder
	printConfig(&out, cfg)
	if strings.Contains(out.String(), "secret") || !strings.Contains(out.String(), "redacted@example.com") || !strings.Contains(out.String(), "api_key=redacted") || !strings.Contains(out.String(), "region=us") {
		t.Fatalf("printConfig URL redaction failed:\n%s", out.String())
	}
}

func TestPrintConfigDoesNotClaimADCReadiness(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "")
	cfg := &config.Config{
		Provider: "gemini",
		Providers: map[string]config.ProviderConfig{
			"gemini": {Model: "model"},
		},
	}
	var out strings.Builder
	printConfig(&out, cfg)
	if strings.Contains(out.String(), "ready") || !strings.Contains(out.String(), "not checked") {
		t.Fatalf("printConfig ADC status is misleading:\n%s", out.String())
	}
}

type terminalTestFile struct {
	info os.FileInfo
	err  error
	fd   uintptr
}

func (f terminalTestFile) Stat() (os.FileInfo, error) {
	return f.info, f.err
}

func (f terminalTestFile) Fd() uintptr {
	return f.fd
}

type terminalTestFileInfo struct {
	mode os.FileMode
}

func (i terminalTestFileInfo) Name() string       { return "stdin" }
func (i terminalTestFileInfo) Size() int64        { return 0 }
func (i terminalTestFileInfo) Mode() os.FileMode  { return i.mode }
func (i terminalTestFileInfo) ModTime() time.Time { return time.Time{} }
func (i terminalTestFileInfo) IsDir() bool        { return false }
func (i terminalTestFileInfo) Sys() any           { return nil }

func TestIsInteractiveTerminal(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		file         terminalTestFile
		detector     bool
		want         bool
		wantDetected bool
	}{
		{
			name:         "tty",
			file:         terminalTestFile{info: terminalTestFileInfo{mode: os.ModeCharDevice}, fd: 7},
			detector:     true,
			want:         true,
			wantDetected: true,
		},
		{
			name:     "pipe",
			file:     terminalTestFile{info: terminalTestFileInfo{mode: os.ModeNamedPipe}, fd: 7},
			detector: true,
		},
		{
			name:     "regular file",
			file:     terminalTestFile{info: terminalTestFileInfo{mode: 0}, fd: 7},
			detector: true,
		},
		{
			name:         "character device but not tty",
			file:         terminalTestFile{info: terminalTestFileInfo{mode: os.ModeCharDevice}, fd: 7},
			detector:     false,
			wantDetected: true,
		},
		{
			name: "stat error",
			file: terminalTestFile{err: errors.New("stat stdin")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detected := false
			got := isInteractiveTerminal(tt.file, func(fd uintptr) bool {
				detected = true
				if fd != 7 {
					t.Errorf("detector fd = %d, want 7", fd)
				}
				return tt.detector
			})
			if got != tt.want {
				t.Errorf("isInteractiveTerminal() = %t, want %t", got, tt.want)
			}
			if detected != tt.wantDetected {
				t.Errorf("detector called = %t, want %t", detected, tt.wantDetected)
			}
		})
	}
}

func TestVersionCommand(t *testing.T) {
	t.Parallel()
	var out strings.Builder
	printVersion(&out)
	if !strings.Contains(out.String(), "prompter v"+AppVersion) {
		t.Errorf("printVersion output = %q, want containing %q", out.String(), "prompter v"+AppVersion)
	}
}

func TestReadLimitedExceedsLimit(t *testing.T) {
	t.Parallel()

	largeInput := strings.Repeat("x", maxInputBytes+10)
	_, err := readLimited(strings.NewReader(largeInput))
	if err == nil {
		t.Fatal("readLimited expected error on oversized input, got nil")
	}
	if !strings.Contains(err.Error(), "1 MB limit") {
		t.Errorf("error = %q, want containing '1 MB limit'", err.Error())
	}
}
