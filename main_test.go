package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/dotcommander/prompter/internal/config"
	"github.com/dotcommander/prompter/internal/provider"
)

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
		{"synthetic", provider.NewChat("synthetic", "key", "model", "http://test", 3), "synthetic"},
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
	if f.command != "refine" {
		t.Errorf("command = %q, want refine", f.command)
	}
}

func TestParseArgs_ImageFlags(t *testing.T) {
	t.Parallel()

	f, err := parseArgs([]string{
		"image",
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
	if f.command != "image" {
		t.Fatalf("command = %q, want image", f.command)
	}
	if f.profile != "minimal" || f.count != 2 || !f.json || !f.noArtist || f.categories != "quality,style" || f.seed != "test-seed" {
		t.Fatalf("image flags not parsed correctly: %+v", f)
	}
	if strings.Join(f.args, " ") != "moon castle" {
		t.Fatalf("args = %v, want moon castle", f.args)
	}
}

func TestParseArgs_RewriteMode(t *testing.T) {
	t.Parallel()

	f, err := parseArgs([]string{
		"rewrite",
		"--mode", "academic",
		"rough notes",
	})
	if err != nil {
		t.Fatalf("parseArgs error: %v", err)
	}
	if f.command != "rewrite" {
		t.Fatalf("command = %q, want rewrite", f.command)
	}
	if f.rewriteMode != "academic" {
		t.Fatalf("rewriteMode = %q, want academic", f.rewriteMode)
	}
	if strings.Join(f.args, " ") != "rough notes" {
		t.Fatalf("args = %v, want rough notes", f.args)
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

	f, err := parseArgs([]string{"rewrite", "--file", "notes.md", "--mode", "academic"})
	if err != nil {
		t.Fatalf("parseArgs error: %v", err)
	}
	if f.command != "rewrite" || f.file != "notes.md" || f.rewriteMode != "academic" {
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
			args:        []string{"image", "portrait of a clockmaker", "--profile", "minimal", "--count", "3", "--json"},
			wantArgs:    "portrait of a clockmaker",
			wantProfile: "minimal",
			wantCount:   3,
			wantJSON:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f, err := parseArgs(tt.args)
			if err != nil {
				t.Fatalf("parseArgs error: %v", err)
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

func TestParseArgs_MissingInterspersedFlagValue(t *testing.T) {
	t.Parallel()

	if _, err := parseArgs([]string{"refine", "prompt", "--output"}); err == nil {
		t.Fatal("parseArgs expected a missing output value error")
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
	prov := provider.NewChat("synthetic", "key", "model-a", "http://test", 3)
	f := &flags{
		command: commandRefine,
		dryRun:  true,
		style:   "code",
		file:    "input.txt",
		output:  "output.txt",
	}
	cfg := &config.Config{
		Effort:       "low",
		SystemPrompt: "system prompt",
	}

	printDryRun(&out, prov, "model-b", f, cfg, "prompt", 60*time.Second)

	got := out.String()
	for _, want := range []string{
		"Dry run: no API call made",
		"Provider: synthetic",
		"Model: model-b",
		"Style: code",
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

func TestPrintUsageIncludesOnlyPublicCommands(t *testing.T) {
	t.Parallel()
	var out strings.Builder

	printUsageTo(&out)

	got := out.String()
	for _, command := range []string{"refine", "critique", "rewrite", "apply", "browse", "image", "configure"} {
		if !strings.Contains(got, command) {
			t.Errorf("usage output missing command %q:\n%s", command, got)
		}
	}
	for _, legacy := range []string{"  enhance", "  run ", "  assemble", "  find", "  config ", "  stats", "  styles", "  providers", "  init", "  update", "  version "} {
		if strings.Contains(got, legacy) {
			t.Errorf("usage output contains legacy command %q:\n%s", legacy, got)
		}
	}
}

func TestResolveRewritePrompt(t *testing.T) {
	t.Parallel()

	prompt, err := resolveRewritePrompt("code")
	if err != nil {
		t.Fatalf("resolveRewritePrompt error: %v", err)
	}
	for _, want := range []string{"Mode: code", "preserve code blocks", "Return only Markdown"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("rewrite prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestResolveRewritePromptUnknownListsModes(t *testing.T) {
	t.Parallel()

	_, err := resolveRewritePrompt("unknown")
	if err == nil {
		t.Fatal("resolveRewritePrompt expected error, got nil")
	}
	got := err.Error()
	for _, want := range []string{"unknown", "valid:", "clean", "synthesis"} {
		if !strings.Contains(got, want) {
			t.Fatalf("resolveRewritePrompt error missing %q: %v", want, err)
		}
	}
}

func TestPreprocessRewriteInput(t *testing.T) {
	t.Parallel()

	longUnbroken := strings.Repeat("a", 101)
	input := strings.Join([]string{
		"# Notes",
		"Subscribe",
		"Subscribe now to the beta list.",
		"Keep this fact.",
		"Keep this fact.",
		"",
		"",
		"",
		longUnbroken,
		"Final line.",
	}, "\n")

	got := preprocessRewriteInput(input)
	if strings.Contains(got, "Subscribe\n") {
		t.Fatalf("preprocessRewriteInput kept standalone cruft line:\n%s", got)
	}
	if !strings.Contains(got, "Subscribe now to the beta list.") || !strings.Contains(got, longUnbroken) {
		t.Fatalf("preprocessRewriteInput deleted legitimate prose or unbroken lines:\n%s", got)
	}
	if strings.Count(got, "Keep this fact.") != 1 {
		t.Fatalf("preprocessRewriteInput duplicate handling failed:\n%s", got)
	}
	if strings.Contains(got, "\n\n\n") {
		t.Fatalf("preprocessRewriteInput kept excessive blanks:\n%s", got)
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

func TestComponentStats(t *testing.T) {
	t.Parallel()

	lib, err := loadComponentLibrary("")
	if err != nil {
		t.Fatalf("loadComponentLibrary error: %v", err)
	}
	stats := componentStats(lib)
	for _, key := range []string{"subjects", "modifiers", "artists", "platforms", "category:quality"} {
		if stats[key] == 0 {
			t.Fatalf("stats[%q] = 0, want > 0", key)
		}
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

func TestResolveStyleGraiOverride(t *testing.T) {
	t.Parallel()

	stylesDir := t.TempDir()
	want := "custom grai prompt"
	if err := os.WriteFile(filepath.Join(stylesDir, "grai.md"), []byte(want), 0600); err != nil {
		t.Fatalf("write grai override: %v", err)
	}

	got, err := resolveStyleFromDir("grai", stylesDir)
	if err != nil {
		t.Fatalf("resolveStyleFromDir(grai) error: %v", err)
	}
	if got != want {
		t.Fatalf("resolveStyleFromDir(grai) = %q, want %q", got, want)
	}
}

func TestResolveStyleGraiFallback(t *testing.T) {
	t.Parallel()

	got, err := resolveStyleFromDir("grai", t.TempDir())
	if err != nil {
		t.Fatalf("resolveStyleFromDir(grai) error: %v", err)
	}
	if got != defaultEnhancePrompt {
		t.Fatal("resolveStyleFromDir(grai) did not return the default enhance prompt")
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
	want := []string{"default", "code", "concise", "creative", "grai", "spec"}
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
			"synthetic":  {APIKey: "key", BaseURL: "http://test"},
			"cerebras":   {APIKey: "key", BaseURL: "http://test"},
			"groq":       {APIKey: "key", BaseURL: "http://test"},
			"openrouter": {APIKey: "key", BaseURL: "http://test"},
			"zai":        {APIKey: "key", BaseURL: "http://test"},
			"wormhole":   {Model: "groq/openai/gpt-oss-120b", BaseURL: "http://127.0.0.1:8080/v1"},
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
		{"valid synthetic", "synthetic", "synthetic", false},
		{"valid cerebras", "cerebras", "cerebras", false},
		{"valid groq", "groq", "groq", false},
		{"valid openrouter", "openrouter", "openrouter", false},
		{"valid zai", "zai", "zai", false},
		{"valid wormhole", "wormhole", "wormhole", false},
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

func TestResolveProviderWormholeAllowsConfiguredMaxOutputTokens(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Providers: map[string]config.ProviderConfig{
			"wormhole": {Model: "groq/openai/gpt-oss-120b", BaseURL: "http://127.0.0.1:8080/v1"},
		},
		MaxOutputTokens:         123,
		MaxOutputTokensExplicit: true,
	}

	prov, err := resolveProvider(cfg, "wormhole", "")
	if err != nil {
		t.Fatalf("resolveProvider unexpected error: %v", err)
	}
	if prov.Name() != "wormhole" {
		t.Fatalf("provider = %q, want wormhole", prov.Name())
	}
}

func TestResolveProviderRejectsMissingRemoteAPIKey(t *testing.T) {
	t.Parallel()

	_, err := resolveProvider(&config.Config{
		Providers: map[string]config.ProviderConfig{
			"groq": {Model: "model", BaseURL: "http://test"},
		},
	}, "groq", "")
	if err == nil || err.Error() != "groq API key not set" {
		t.Fatalf("resolveProvider error = %v, want missing groq API key", err)
	}
}

func TestResolveProviderOverridesWormholeBaseURL(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Providers: map[string]config.ProviderConfig{
			"wormhole": {Model: "groq/openai/gpt-oss-120b", BaseURL: "http://127.0.0.1:8080/v1"},
			"groq":     {APIKey: "groq-key", BaseURL: "https://api.groq.example/v1"},
		},
	}

	prov, err := resolveProvider(cfg, "wormhole", "http://127.0.0.1:9090/v1")
	if err != nil {
		t.Fatalf("resolveProvider unexpected error: %v", err)
	}
	if prov.Name() != "wormhole" {
		t.Fatalf("provider = %q, want wormhole", prov.Name())
	}
	if got := cfg.Providers["wormhole"].BaseURL; got != "http://127.0.0.1:8080/v1" {
		t.Errorf("loaded wormhole BaseURL mutated to %q", got)
	}
	if got := cfg.Providers["groq"].BaseURL; got != "https://api.groq.example/v1" {
		t.Errorf("unselected groq BaseURL mutated to %q", got)
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

func TestResolveCommandSystemPrompt_LocalCommandsSkipPromptFile(t *testing.T) {
	t.Parallel()

	for _, command := range []string{commandImage, commandBrowse, commandConfigure} {
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

// -----------------------------------------------------------------------------
// Frontmatter Parsing Tests
// -----------------------------------------------------------------------------

func TestParseFrontmatter(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		content      string
		wantName     string
		wantDesc     string
		wantAliases  []string
		wantTriggers []string
		wantExamples []string
		wantBody     string
		wantErr      bool
	}{
		{
			name:     "no frontmatter",
			content:  "Just a plain prompt",
			wantBody: "Just a plain prompt",
		},
		{
			name: "valid frontmatter",
			content: `---
description: Test prompt
aliases:
  - test
  - example
---
The body content`,
			wantDesc:    "Test prompt",
			wantAliases: []string{"test", "example"},
			wantBody:    "The body content",
		},
		{
			name: "role frontmatter",
			content: `---
name: spec
description: Crystallize requirements
triggers:
  - acceptance criteria
examples:
  - spec 'make it faster'
---
Role body`,
			wantName:     "spec",
			wantDesc:     "Crystallize requirements",
			wantTriggers: []string{"acceptance criteria"},
			wantExamples: []string{"spec 'make it faster'"},
			wantBody:     "Role body",
		},
		{
			name: "frontmatter at EOF",
			content: `---
description: EOF test
---`,
			wantDesc: "EOF test",
			wantBody: "",
		},
		{
			name:     "incomplete frontmatter (no closing)",
			content:  "---\ndescription: broken\nNo closing delimiter",
			wantBody: "---\ndescription: broken\nNo closing delimiter",
		},
		{
			name: "malformed YAML (gracefully ignored by goldmark)",
			content: `---
description: [invalid yaml
aliases: not: valid: yaml
---
body`,
			wantBody: "body",
			wantErr:  false,
		},
		{
			name: "empty frontmatter treated as no frontmatter",
			content: `---
---
Body after empty frontmatter`,
			// Parser requires content between delimiters, so this is treated as plain text
			wantBody: "---\n---\nBody after empty frontmatter",
		},
		{
			name:     "CRLF line endings parsed correctly",
			content:  "---\r\ndescription: hello\r\n---\r\n\r\nbody text\r\n",
			wantDesc: "hello",
			wantBody: "body text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fm, body, err := parseFrontmatter([]byte(tt.content))

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if fm.Description != tt.wantDesc {
				t.Errorf("Description = %q, want %q", fm.Description, tt.wantDesc)
			}
			if fm.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", fm.Name, tt.wantName)
			}
			if len(fm.Aliases) != len(tt.wantAliases) {
				t.Errorf("Aliases = %v, want %v", fm.Aliases, tt.wantAliases)
			} else {
				for i, alias := range fm.Aliases {
					if alias != tt.wantAliases[i] {
						t.Errorf("Aliases[%d] = %q, want %q", i, alias, tt.wantAliases[i])
					}
				}
			}
			if strings.Join(fm.Triggers, "\n") != strings.Join(tt.wantTriggers, "\n") {
				t.Errorf("Triggers = %v, want %v", fm.Triggers, tt.wantTriggers)
			}
			if strings.Join(fm.Examples, "\n") != strings.Join(tt.wantExamples, "\n") {
				t.Errorf("Examples = %v, want %v", fm.Examples, tt.wantExamples)
			}
			if body != tt.wantBody {
				t.Errorf("body = %q, want %q", body, tt.wantBody)
			}
		})
	}
}

// -----------------------------------------------------------------------------
// ScanPromptsDir Tests
// -----------------------------------------------------------------------------

func TestScanPromptsDir(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		setup     func(dir string) error
		wantCount int
		wantNames []string
		wantErr   bool
	}{
		{
			name: "empty directory",
			setup: func(dir string) error {
				return nil
			},
			wantCount: 0,
		},
		{
			name: "single prompt file",
			setup: func(dir string) error {
				return os.WriteFile(filepath.Join(dir, "test.md"), []byte("test content"), 0644)
			},
			wantCount: 1,
			wantNames: []string{"test"},
		},
		{
			name: "nested directories",
			setup: func(dir string) error {
				subDir := filepath.Join(dir, "subdir")
				if err := os.MkdirAll(subDir, 0755); err != nil {
					return err
				}
				if err := os.WriteFile(filepath.Join(dir, "root.md"), []byte("root"), 0644); err != nil {
					return err
				}
				return os.WriteFile(filepath.Join(subDir, "nested.md"), []byte("nested"), 0644)
			},
			wantCount: 2,
			wantNames: []string{"root", "nested"},
		},
		{
			name: "ignores non-md files",
			setup: func(dir string) error {
				if err := os.WriteFile(filepath.Join(dir, "prompt.md"), []byte("prompt"), 0644); err != nil {
					return err
				}
				if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("txt"), 0644); err != nil {
					return err
				}
				return os.WriteFile(filepath.Join(dir, "config.json"), []byte("{}"), 0644)
			},
			wantCount: 1,
			wantNames: []string{"prompt"},
		},
		{
			name: "prompt with frontmatter",
			setup: func(dir string) error {
				content := `---
description: A test prompt
aliases:
  - tp
---
Body content`
				return os.WriteFile(filepath.Join(dir, "test-prompt.md"), []byte(content), 0644)
			},
			wantCount: 1,
			wantNames: []string{"test-prompt"},
		},
		{
			name: "role prompt frontmatter uses name",
			setup: func(dir string) error {
				content := `---
name: spec
description: Crystallize requirements
aliases:
  - requirements
triggers:
  - acceptance criteria
examples:
  - spec 'make it faster'
---
Role content`
				return os.WriteFile(filepath.Join(dir, "thinking-spec.md"), []byte(content), 0644)
			},
			wantCount: 1,
			wantNames: []string{"spec"},
		},
		{
			name: "symlinked directory",
			setup: func(dir string) error {
				externalDir := filepath.Join(dir, "..", "external-vault")
				if err := os.MkdirAll(externalDir, 0755); err != nil {
					return err
				}
				if err := os.WriteFile(filepath.Join(externalDir, "vault-prompt.md"), []byte("vault prompt"), 0644); err != nil {
					return err
				}
				return os.Symlink(externalDir, filepath.Join(dir, "vault-link"))
			},
			wantCount: 1,
			wantNames: []string{"vault-prompt"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tmpDir := t.TempDir()
			if err := tt.setup(tmpDir); err != nil {
				t.Fatalf("setup failed: %v", err)
			}

			entries, err := ScanPromptsDir(tmpDir)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(entries) != tt.wantCount {
				t.Errorf("got %d entries, want %d", len(entries), tt.wantCount)
			}

			// Check names are present (order may vary)
			for _, wantName := range tt.wantNames {
				found := false
				for _, e := range entries {
					if e.Name == wantName {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected to find entry named %q", wantName)
				}
			}
		})
	}
}

func TestScanPromptDirs(t *testing.T) {
	t.Parallel()
	first := t.TempDir()
	second := t.TempDir()

	if err := os.WriteFile(filepath.Join(first, "enhance.md"), []byte("enhance"), 0644); err != nil {
		t.Fatalf("write first prompt: %v", err)
	}
	rolePrompt := `---
name: spec
description: Crystallize requirements
triggers:
  - acceptance criteria
examples:
  - spec 'make it faster'
---
Role content`
	if err := os.WriteFile(filepath.Join(second, "thinking-spec.md"), []byte(rolePrompt), 0644); err != nil {
		t.Fatalf("write role prompt: %v", err)
	}

	entries, err := scanPromptDirs([]string{first, filepath.Join(t.TempDir(), "missing"), second})
	if err != nil {
		t.Fatalf("scanPromptDirs error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}

	var foundRole bool
	for _, entry := range entries {
		if entry.Name != "spec" {
			continue
		}
		foundRole = true
		aliases := strings.Join(entry.Aliases, "\n")
		for _, want := range []string{"acceptance criteria", "spec 'make it faster'"} {
			if !strings.Contains(aliases, want) {
				t.Errorf("role aliases = %v, missing %q", entry.Aliases, want)
			}
		}
	}
	if !foundRole {
		t.Fatal("expected role prompt named spec")
	}
}

func TestPromptDirs(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		PromptsDir: "/prompts",
		PromptsDirs: []string{
			"/roles",
			"/prompts",
			"/more",
		},
	}
	got, err := promptDirs(cfg)
	if err != nil {
		t.Fatalf("promptDirs error: %v", err)
	}
	want := []string{"/prompts", "/roles", "/more"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("promptDirs = %v, want %v", got, want)
	}
}

func TestPromptDirs_IncludesDefaultRolePrompts(t *testing.T) {
	tmpDir := t.TempDir()
	rolePrompts := filepath.Join(tmpDir, ".config", "roles", "prompts")
	if err := os.MkdirAll(rolePrompts, 0755); err != nil {
		t.Fatalf("mkdir role prompts: %v", err)
	}
	t.Setenv("HOME", tmpDir)

	got, err := promptDirs(&config.Config{PromptsDir: "/prompts"})
	if err != nil {
		t.Fatalf("promptDirs error: %v", err)
	}
	want := []string{"/prompts", rolePrompts}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("promptDirs = %v, want %v", got, want)
	}
}

func TestScanPromptsDir_NonexistentDir(t *testing.T) {
	t.Parallel()
	_, err := ScanPromptsDir("/nonexistent/path/that/does/not/exist")
	if err == nil {
		t.Error("expected error for nonexistent directory")
	}
}

// TestScanPromptsDir_DepthLimit verifies maxScanDepth (5) skips directories
// deeper than the limit. Files inside skipped directories are not returned.
//
// Layout (relative to tmpDir):
//
//	root.md                                          # depth 0, scanned
//	a/file.md                                        # depth 1, scanned
//	a/b/file.md                                      # depth 2, scanned
//	a/b/c/file.md                                    # depth 3, scanned
//	a/b/c/d/file.md                                  # depth 4, scanned
//	a/b/c/d/e/file.md                                # depth 5, scanned (dir relPath "a/b/c/d/e" has 4 separators, < maxScanDepth)
//	a/b/c/d/e/f/file.md                              # dir relPath "a/b/c/d/e/f" has 5 separators, >= maxScanDepth -> SkipDir
//	a/b/c/d/e/f/g/file.md                            # unreachable (parent skipped)
func TestScanPromptsDir_DepthLimit(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Files at progressively deeper levels. Names encode their depth.
	depths := []string{
		"root.md",
		filepath.Join("a", "d1.md"),
		filepath.Join("a", "b", "d2.md"),
		filepath.Join("a", "b", "c", "d3.md"),
		filepath.Join("a", "b", "c", "d", "d4.md"),
		filepath.Join("a", "b", "c", "d", "e", "d5.md"),
		filepath.Join("a", "b", "c", "d", "e", "f", "d6.md"),
		filepath.Join("a", "b", "c", "d", "e", "f", "g", "d7.md"),
	}

	for _, rel := range depths {
		full := filepath.Join(tmpDir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", full, err)
		}
		if err := os.WriteFile(full, []byte("body"), 0o644); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
	}

	entries, err := ScanPromptsDir(tmpDir)
	if err != nil {
		t.Fatalf("ScanPromptsDir error: %v", err)
	}

	names := make(map[string]bool, len(entries))
	for _, e := range entries {
		names[e.Name] = true
	}

	// Within depth limit -> must be present.
	wantPresent := []string{"root", "d1", "d2", "d3", "d4", "d5"}
	for _, n := range wantPresent {
		if !names[n] {
			t.Errorf("expected entry %q to be scanned within depth limit, got entries: %v", n, names)
		}
	}

	// Beyond depth limit -> must be absent.
	wantAbsent := []string{"d6", "d7"}
	for _, n := range wantAbsent {
		if names[n] {
			t.Errorf("expected entry %q to be skipped beyond depth limit, but it was scanned", n)
		}
	}
}

// TestScanPromptsDir_FileCountLimit verifies maxScanFiles (1000) caps the
// returned entries. Creating maxScanFiles+10 markdown files must produce
// exactly maxScanFiles entries.
func TestScanPromptsDir_FileCountLimit(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Spread files across a few subdirectories to exercise WalkDir ordering
	// without exceeding the depth limit (all dirs sit at depth 1).
	const extra = 10
	total := maxScanFiles + extra

	subdirs := []string{"a", "b", "c", "d"}
	for _, sd := range subdirs {
		if err := os.MkdirAll(filepath.Join(tmpDir, sd), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", sd, err)
		}
	}

	for i := 0; i < total; i++ {
		sd := subdirs[i%len(subdirs)]
		name := filepath.Join(tmpDir, sd, "p"+itoa(i)+".md")
		if err := os.WriteFile(name, []byte("body"), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	entries, err := ScanPromptsDir(tmpDir)
	if err != nil {
		t.Fatalf("ScanPromptsDir error: %v", err)
	}

	if len(entries) > maxScanFiles {
		t.Errorf("got %d entries, want <= %d (maxScanFiles)", len(entries), maxScanFiles)
	}
	if len(entries) != maxScanFiles {
		t.Errorf("got %d entries, want exactly %d (maxScanFiles); fewer entries suggests the limit was not the gating factor", len(entries), maxScanFiles)
	}
}

// itoa is a tiny test-local int formatter to avoid pulling strconv into the
// test imports just for filename construction.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

// -----------------------------------------------------------------------------
// Weighted Rank Tests
// -----------------------------------------------------------------------------

var weightedRankTests = []struct { //nolint:gochecknoglobals // test table data
	name      string
	query     string
	entry     PromptEntry
	wantMatch bool
	wantMin   int // minimum expected score (0 = any match)
}{
	{
		name:      "empty query returns 0",
		query:     "",
		entry:     PromptEntry{Name: "test"},
		wantMatch: true,
		wantMin:   0,
	},
	{
		name:      "exact name match",
		query:     "test",
		entry:     PromptEntry{Name: "test"},
		wantMatch: true,
		wantMin:   weightName,
	},
	{
		name:      "partial name match",
		query:     "tes",
		entry:     PromptEntry{Name: "test-prompt"},
		wantMatch: true,
		wantMin:   weightName,
	},
	{
		name:      "alias match",
		query:     "tp",
		entry:     PromptEntry{Name: "test-prompt", Aliases: []string{"tp", "testing"}},
		wantMatch: true,
		wantMin:   weightAlias,
	},
	{
		name:      "description match",
		query:     "generate",
		entry:     PromptEntry{Name: "code", Description: "Generate code snippets"},
		wantMatch: true,
		wantMin:   weightDescription,
	},
	{
		name:      "body match",
		query:     "unique",
		entry:     PromptEntry{Name: "test", Content: "This has a unique keyword"},
		wantMatch: true,
		wantMin:   weightBody,
	},
	{
		name:      "no match",
		query:     "xyz123",
		entry:     PromptEntry{Name: "test", Description: "desc", Content: "content"},
		wantMatch: false,
	},
	{
		name:      "case insensitive",
		query:     "TEST",
		entry:     PromptEntry{Name: "test"},
		wantMatch: true,
		wantMin:   weightName,
	},
	{
		name:  "path component match",
		query: "think",
		entry: PromptEntry{
			Name: "ultrathink",
			Path: "/path/to/project/prompts.d/think/ultrathink.md",
		},
		wantMatch: true,
		wantMin:   weightPath,
	},
}

func TestWeightedRank(t *testing.T) {
	t.Parallel()
	for _, tt := range weightedRankTests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			score := weightedRank(tt.query, tt.entry)

			if tt.wantMatch {
				if score < 0 {
					t.Errorf("expected match, got score %d", score)
				}
				if score < tt.wantMin {
					t.Errorf("score = %d, want >= %d", score, tt.wantMin)
				}
			} else if score >= 0 {
				t.Errorf("expected no match (score -1), got %d", score)
			}
		})
	}
}

func TestWeightedRank_Priority(t *testing.T) {
	t.Parallel()
	// Test that weights are correctly ordered: name > alias > path > description > body
	// Using same query length for fair comparison

	nameOnlyEntry := PromptEntry{Name: "test"}
	aliasOnlyEntry := PromptEntry{Name: "other", Aliases: []string{"test"}}
	descOnlyEntry := PromptEntry{Name: "other", Description: "test item"}
	bodyOnlyEntry := PromptEntry{Name: "other", Content: "test content"}

	nameScore := weightedRank("test", nameOnlyEntry)
	aliasScore := weightedRank("test", aliasOnlyEntry)
	descScore := weightedRank("test", descOnlyEntry)
	bodyScore := weightedRank("test", bodyOnlyEntry)

	// Verify the weight ordering is respected
	if nameScore < weightName {
		t.Errorf("name score (%d) should be >= weightName (%d)", nameScore, weightName)
	}
	if aliasScore < weightAlias {
		t.Errorf("alias score (%d) should be >= weightAlias (%d)", aliasScore, weightAlias)
	}
	if descScore < weightDescription {
		t.Errorf("description score (%d) should be >= weightDescription (%d)", descScore, weightDescription)
	}
	if bodyScore < weightBody {
		t.Errorf("body score (%d) should be >= weightBody (%d)", bodyScore, weightBody)
	}

	// Verify ordering: name > alias > description > body
	if nameScore <= aliasScore {
		t.Errorf("name score (%d) should be > alias score (%d)", nameScore, aliasScore)
	}
	if aliasScore <= descScore {
		t.Errorf("alias score (%d) should be > description score (%d)", aliasScore, descScore)
	}
	if descScore <= bodyScore {
		t.Errorf("description score (%d) should be > body score (%d)", descScore, bodyScore)
	}
}

func TestRunFinderPreservesSelectedEntryIdentity(t *testing.T) {
	t.Parallel()
	entries := []PromptEntry{{Name: "first"}, {Name: "second", Description: "chosen"}}

	got, err := runFinder(entries, nil, func(m tea.Model) (tea.Model, error) {
		fm := m.(*finderModel)
		fm.selected = &fm.entries[1]
		return fm, nil
	})
	if err != nil {
		t.Fatalf("runFinder() error = %v", err)
	}
	if got != &entries[1] {
		t.Fatalf("runFinder() = %p, want original entry %p", got, &entries[1])
	}
}

func TestRunFinderCancellation(t *testing.T) {
	t.Parallel()
	got, err := runFinder([]PromptEntry{{Name: "first"}}, nil, func(m tea.Model) (tea.Model, error) {
		fm := m.(*finderModel)
		fm.canceled = true
		return fm, nil
	})
	if err != nil || got != nil {
		t.Fatalf("runFinder() = (%v, %v), want (nil, nil)", got, err)
	}
}

func TestFinderModel_FilteringAndNavigation(t *testing.T) {
	t.Parallel()
	entries := []PromptEntry{
		{Name: "code review", Description: "review PRs"},
		{Name: "commit msg", Description: "git conventional commit"},
		{Name: "creative write", Description: "fiction generator"},
	}

	model := newFinderModel(entries, []string{"/test/prompts"})

	// Initially all entries are visible in order
	if len(model.filtered) != 3 {
		t.Fatalf("expected 3 filtered entries, got %d", len(model.filtered))
	}

	// Down arrow moves cursor
	m, _ := model.Update(tea.KeyPressMsg(tea.Key{Text: "down", Code: rune(tea.KeyDown)}))
	model = m.(*finderModel)
	if model.cursor != 1 {
		t.Fatalf("cursor after down = %d, want 1", model.cursor)
	}

	// Up arrow moves cursor back
	m, _ = model.Update(tea.KeyPressMsg(tea.Key{Text: "up", Code: rune(tea.KeyUp)}))
	model = m.(*finderModel)
	if model.cursor != 0 {
		t.Fatalf("cursor after up = %d, want 0", model.cursor)
	}

	// Type query "commit"
	for _, r := range "commit" {
		m, _ = model.Update(tea.KeyPressMsg(tea.Key{Text: string(r), Code: r}))
		model = m.(*finderModel)
	}

	if len(model.filtered) != 1 || model.filtered[0].entry.Name != "commit msg" {
		t.Fatalf("filtered results after 'commit' = %+v, want only 'commit msg'", model.filtered)
	}

	// Enter selects top match
	m, _ = model.Update(tea.KeyPressMsg(tea.Key{Text: "enter", Code: rune(tea.KeyEnter)}))
	model = m.(*finderModel)
	if model.selected == nil || model.selected.Name != "commit msg" {
		t.Fatalf("selected = %v, want 'commit msg'", model.selected)
	}

	// ESC cancels
	model2 := newFinderModel(entries, nil)
	view := model2.View()
	if !view.AltScreen {
		t.Fatal("expected view.AltScreen = true")
	}
	m, _ = model2.Update(tea.KeyPressMsg(tea.Key{Text: "esc", Code: rune(tea.KeyEscape)}))
	model2 = m.(*finderModel)
	if !model2.canceled {
		t.Fatal("expected model2.canceled = true after ESC")
	}
}

func TestPrintCommandUsage(t *testing.T) {
	t.Parallel()
	cmds := []string{"refine", "critique", "rewrite", "apply", "browse", "image", "configure"}
	for _, cmd := range cmds {
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

func TestParseArgs_PublicCommands(t *testing.T) {
	t.Parallel()
	tests := [][]string{{"refine"}, {"critique"}, {"rewrite"}, {"apply", "refactor"}, {"browse"}, {"image"}, {"configure"}}
	for _, args := range tests {
		f, err := parseArgs(args)
		if err != nil {
			t.Fatalf("parseArgs(%q) error: %v", args, err)
		}
		if f.command != args[0] {
			t.Errorf("parseArgs(%q) command = %q, want %q", args, f.command, args[0])
		}
	}
}

func TestParseArgsRejectsLegacyCommands(t *testing.T) {
	t.Parallel()
	for _, command := range []string{"enhance", "run", "assemble", "find", "config", "stats", "styles", "providers", "init", "update", "version", "help"} {
		if _, err := parseArgs([]string{command}); err == nil || !strings.Contains(err.Error(), "unknown command") {
			t.Errorf("parseArgs(%q) error = %v, want unknown command", command, err)
		}
	}
}

func TestParseArgsEnforcesCommandFlagOwnership(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{
		{"refine", "--mode", "clean"},
		{"rewrite", "--style", "concise"},
		{"apply", "refactor", "--style", "concise"},
		{"image", "subject", "--provider", "openai"},
	} {
		if _, err := parseArgs(args); err == nil || !strings.Contains(err.Error(), "flag provided but not defined") {
			t.Errorf("parseArgs(%q) error = %v, want undefined flag", args, err)
		}
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

func TestResolveCommandSystemPromptRewriteMode(t *testing.T) {
	t.Parallel()
	f := &flags{command: commandRewrite, rewriteMode: "academic"}
	cfg := &config.Config{}
	if err := resolveCommandSystemPrompt(f, cfg); err != nil {
		t.Fatalf("resolveCommandSystemPrompt error: %v", err)
	}
	if !strings.Contains(cfg.SystemPrompt, "Mode: academic") {
		t.Errorf("SystemPrompt missing mode:\n%s", cfg.SystemPrompt)
	}
}

func TestResolveStyle_RewriteHint(t *testing.T) {
	t.Parallel()
	_, err := resolveStyle("clean")
	if err == nil {
		t.Fatal("expected error for 'clean' as enhance style, got nil")
	}
	if !strings.Contains(err.Error(), "rewrite mode") {
		t.Errorf("expected error to mention 'rewrite mode', got: %v", err)
	}
}

func TestResolveRewritePrompt_StyleHint(t *testing.T) {
	t.Parallel()
	_, err := resolveRewritePrompt("creative")
	if err == nil {
		t.Fatal("expected error for 'creative' as rewrite mode, got nil")
	}
	if !strings.Contains(err.Error(), "enhancement style") {
		t.Errorf("expected error to mention 'enhancement style', got: %v", err)
	}
	if !strings.Contains(err.Error(), "prompter refine --style creative") {
		t.Errorf("expected error to show the refine style command, got: %v", err)
	}
}

func TestRunStyles(t *testing.T) {
	t.Parallel()
	var out strings.Builder
	printStyles(&out)
	if !strings.Contains(out.String(), "concise") || !strings.Contains(out.String(), "academic") {
		t.Fatalf("printStyles output missing expected entries:\n%s", out.String())
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

func TestRunProviders(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Provider: "gemini",
		Providers: map[string]config.ProviderConfig{
			"gemini": {Model: "gemini-3.7-flash"},
		},
	}
	var out strings.Builder
	printProviders(&out, cfg)
	for _, want := range []string{"gemini *", "ADC/ready", "omlx", "local/ready"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("printProviders output missing %q:\n%s", want, out.String())
		}
	}
}

func TestMetadataOutputRedactsURLUserinfo(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Provider: "groq",
		Providers: map[string]config.ProviderConfig{
			"groq": {BaseURL: "https://user:secret@example.com/v1"},
		},
	}
	var out strings.Builder
	printConfig(&out, cfg)
	if strings.Contains(out.String(), "secret") || !strings.Contains(out.String(), "redacted@example.com") {
		t.Fatalf("printConfig URL redaction failed:\n%s", out.String())
	}
}

func TestRunFinder_UserAborted(t *testing.T) {
	t.Parallel()
	entries := []PromptEntry{{Name: "test", Path: "/test.md", Content: "content"}}
	got, err := runFinder(entries, nil, func(m tea.Model) (tea.Model, error) {
		fm := m.(*finderModel)
		fm.canceled = true
		return fm, nil
	})
	if err != nil {
		t.Fatalf("runFinder on UserAborted error = %v, want nil", err)
	}
	if got != nil {
		t.Fatalf("runFinder on UserAborted returned %v, want nil", got)
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

func TestPopularModelsFor(t *testing.T) {
	t.Parallel()

	providers := []string{
		"gemini",
		"openai",
		"groq",
		"cerebras",
		"openrouter",
		"synthetic",
		"zai",
		"wormhole",
		"omlx",
	}

	for _, p := range providers {
		models := popularModelsFor(p)
		if len(models) < 2 {
			t.Errorf("popularModelsFor(%q) returned %d models, want at least 2", p, len(models))
		}
		for _, m := range models {
			if m.id == "" || m.label == "" {
				t.Errorf("popularModelsFor(%q) returned invalid model: %+v", p, m)
			}
		}
	}

	// Unknown provider should return nil
	if unknown := popularModelsFor("unknown-prov"); unknown != nil {
		t.Errorf("popularModelsFor(unknown-prov) = %v, want nil", unknown)
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

func TestShowFinder_AutoSeedsEmptyDirectory(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		PromptsDir:  tmpDir,
		PromptsDirs: []string{tmpDir},
	}

	// Verify directory is empty initially
	entries, err := scanPromptDirs([]string{tmpDir})
	if err != nil {
		t.Fatalf("scanPromptDirs initial: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries initially, got %d", len(entries))
	}

	// Calling ensurePromptVault should auto-seed starter prompts
	entries, dirs, err := ensurePromptVault(cfg)
	if err != nil {
		t.Fatalf("ensurePromptVault: %v", err)
	}
	if len(dirs) == 0 {
		t.Fatalf("expected dirs to be populated, got empty")
	}
	if len(entries) < 5 {
		t.Errorf("expected at least 5 starter prompts seeded, got %d", len(entries))
	}

	files, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(files) < 5 {
		t.Errorf("expected at least 5 starter prompt files in directory, found %d", len(files))
	}
}

func TestEnsurePromptVaultSeedsEmptyPrimaryWhenSecondaryHasPrompts(t *testing.T) {
	t.Parallel()

	primaryDir := t.TempDir()
	secondaryDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(secondaryDir, "existing.md"), []byte("secondary prompt\n"), 0o644); err != nil {
		t.Fatalf("write secondary prompt: %v", err)
	}
	cfg := &config.Config{
		PromptsDir:  primaryDir,
		PromptsDirs: []string{primaryDir, secondaryDir},
	}

	entries, _, err := ensurePromptVault(cfg)
	if err != nil {
		t.Fatalf("ensurePromptVault: %v", err)
	}
	if len(entries) < 2 {
		t.Fatalf("entries = %d, want secondary prompt plus seeded starters", len(entries))
	}
	if _, err := os.Stat(filepath.Join(primaryDir, "refactor.md")); err != nil {
		t.Fatalf("primary starter prompt was not seeded: %v", err)
	}
}

func TestUsageOutputDocumentsBrowse(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	printUsageTo(&out)
	got := out.String()

	if !strings.Contains(got, "  browse") || !strings.Contains(got, "interactive prompt browser") {
		t.Errorf("expected usage to document browse command, got:\n%s", got)
	}
	if strings.Contains(got, "  find") {
		t.Errorf("expected usage not to contain legacy find command, got:\n%s", got)
	}
}
