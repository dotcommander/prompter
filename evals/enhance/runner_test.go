package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

var enhanceEvalHelperDryRun = flag.Bool("dry-run", false, "used by the enhance eval test helper")

func TestRunRejectsRelativeBinary(t *testing.T) {
	err := run([]string{"--binary", "prompter", "--fixtures", "missing", "--manifest", "missing", "--prompt", "missing"})
	if err == nil || !strings.Contains(err.Error(), "absolute path") {
		t.Fatalf("run error = %v, want absolute path rejection", err)
	}
}

func TestRunRejectsNonPositiveFixtureTimeout(t *testing.T) {
	err := run([]string{"--binary", "/tmp/prompter", "--fixtures", "fixtures", "--manifest", "manifest", "--prompt", "prompt", "--fixture-timeout", "0s"})
	if err == nil || !strings.Contains(err.Error(), "fixture-timeout must be positive") {
		t.Fatalf("run error = %v, want fixture timeout rejection", err)
	}
}

func TestInvokeHonorsDeadline(t *testing.T) {
	t.Setenv("PROMPTER_EVAL_HANG_HELPER", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, _, _, err := invoke(ctx, os.Args[0], []string{"-test.run=TestEnhanceEvalHangHelperProcess"}, "", os.Environ())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("invoke error = %v, want context deadline exceeded", err)
	}
}

func TestLimitedBufferRejectsOversizeOutput(t *testing.T) {
	buffer := limitedBuffer{limit: 5}
	n, err := buffer.Write([]byte("123456"))
	if n != 5 || !errors.Is(err, errCommandOutputLimit) || string(buffer.Bytes()) != "12345" {
		t.Fatalf("Write = %d, %v, %q", n, err, buffer.Bytes())
	}
}

func TestInjectDryRunFlagImmediatelyAfterCommand(t *testing.T) {
	got := injectDryRunFlag([]string{"refine", "--model", "--"})
	want := []string{"refine", "--dry-run", "--model", "--"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("injectDryRunFlag() = %q, want %q", got, want)
	}
}

func TestInjectDryRunFlagHandlesEmptyArgs(t *testing.T) {
	got := injectDryRunFlag(nil)
	want := []string{"--dry-run"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("injectDryRunFlag() = %q, want %q", got, want)
	}
}

func TestReadFixturesRejectsReservedDryRunFlag(t *testing.T) {
	for _, args := range [][]string{
		{"refine", "--dry-run"},
		{"refine", "-dry-run"},
		{"refine", "--dry-run=false"},
		{"refine", "-dry-run=true"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "fixtures.json")
			data, err := json.Marshal(fixtureSet{Fixtures: []fixture{{ID: "guard", Args: args}}})
			if err != nil {
				t.Fatal(err)
			}
			writeTestFile(t, path, string(data))

			_, err = readFixtures(path)
			if err == nil || !strings.Contains(err.Error(), `fixture "guard" sets reserved --dry-run flag`) {
				t.Fatalf("readFixtures() error = %v, want reserved-flag rejection", err)
			}
		})
	}
}

func TestReadFixturesAllowsLiteralDryRunText(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fixtures.json")
	data, err := json.Marshal(fixtureSet{Fixtures: []fixture{{
		ID:   "literal",
		Args: []string{"refine", "--", "--dry-run=false"},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, path, string(data))

	fixtures, err := readFixtures(path)
	if err != nil {
		t.Fatalf("readFixtures() error = %v", err)
	}
	if !reflect.DeepEqual(fixtures[0].Args, []string{"refine", "--", "--dry-run=false"}) {
		t.Fatalf("fixture args = %q, want preserved literal input", fixtures[0].Args)
	}
}

func TestValidateFixtureArgsRejectsUnsafeSurfaces(t *testing.T) {
	for _, args := range [][]string{
		{"critique"},
		{"refine", "secret positional input"},
		{"refine", "provider=groq"},
		{"refine", "--base-url", "https://attacker.invalid"},
		{"refine", "--file", "/etc/passwd"},
		{"refine", "--output", "/tmp/result"},
		{"refine", "--copy"},
		{"refine", "--stream"},
	} {
		if err := validateFixtureArgs(args, "/tmp/prompter"); err == nil {
			t.Errorf("validateFixtureArgs(%q) accepted unsafe fixture", args)
		}
	}
	if err := validateFixtureArgs([]string{"refine", "--provider", "groq", "--model=test", "-s", "code", "--verbose"}, "/tmp/prompter"); err != nil {
		t.Fatalf("safe fixture rejected: %v", err)
	}
}

func TestSanitizedFixtureArgsRedactsValues(t *testing.T) {
	got := sanitizedFixtureArgs([]string{"refine", "--provider", "groq", "--model=secret-model", "-v"})
	want := []string{"refine", "--provider", "<redacted>", "--model=<redacted>", "-v"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sanitizedFixtureArgs() = %q, want %q", got, want)
	}
}

func TestEvaluationEnvOmitsUnrelatedSecrets(t *testing.T) {
	t.Setenv("UNRELATED_SECRET", "must-not-pass")
	t.Setenv("GROQ_API_KEY", "provider-key")
	t.Setenv("CUSTOM_GROQ_KEY", "custom-key")
	env := evaluationEnv("groq", "CUSTOM_GROQ_KEY", "/prompt")
	joined := strings.Join(env, "\n")
	for _, want := range []string{"GROQ_API_KEY=provider-key", "CUSTOM_GROQ_KEY=custom-key", "PROMPTER_PROMPT_FILE=/prompt"} {
		if !strings.Contains(joined, want) {
			t.Errorf("evaluation environment missing %q: %q", want, env)
		}
	}
	if strings.Contains(joined, "UNRELATED_SECRET") {
		t.Fatalf("evaluation environment leaked unrelated secret: %q", env)
	}
}

func TestRejectManifestInSourceAndExclusiveLock(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "results.jsonl")
	if err := rejectManifestInSource(root, inside); err == nil {
		t.Fatal("manifest inside source repository accepted")
	}
	outside := filepath.Join(t.TempDir(), "results.jsonl")
	if err := rejectManifestInSource(root, outside); err != nil {
		t.Fatalf("manifest outside source repository rejected: %v", err)
	}
	lock, err := acquireManifestLock(outside)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.release()
	if _, err := acquireManifestLock(outside); err == nil {
		t.Fatal("concurrent manifest lock acquired")
	}
}

func TestFixtureValueFlagsMatchCLI(t *testing.T) {
	parsed, err := parser.ParseFile(token.NewFileSet(), filepath.Join("..", "..", "main.go"), nil, 0)
	if err != nil {
		t.Fatalf("parse CLI flags: %v", err)
	}
	current := make(map[string]bool)
	ast.Inspect(parsed, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || (selector.Sel.Name != "StringVar" && selector.Sel.Name != "IntVar") || len(call.Args) < 2 {
			return true
		}
		literal, ok := call.Args[1].(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			return true
		}
		name, err := strconv.Unquote(literal.Value)
		if err != nil {
			t.Fatalf("unquote CLI flag %q: %v", literal.Value, err)
		}
		current[name] = true
		return true
	})

	want := make(map[string]bool, len(fixtureLLMValueFlags)+len(fixtureImageValueFlags)+3)
	for name := range fixtureLLMValueFlags {
		want[name] = true
	}
	for name := range fixtureImageValueFlags {
		want[name] = true
	}
	want["style"] = true
	want["s"] = true
	want["mode"] = true
	if !reflect.DeepEqual(current, want) {
		t.Fatalf("CLI value flags = %v, evaluator grammar = %v", current, want)
	}
}

func TestRunWritesSourceBoundManifestAndResumes(t *testing.T) {
	t.Setenv("PROMPTER_EVAL_HELPER", "1")
	tempDir := t.TempDir()
	promptPath := filepath.Join(tempDir, "enhance.md")
	fixturePath := filepath.Join(tempDir, "fixtures.json")
	manifestPath := filepath.Join(tempDir, "results.jsonl")
	callLogPath := filepath.Join(tempDir, "calls.log")
	t.Setenv("PROMPTER_EVAL_CALL_LOG", callLogPath)
	t.Setenv("PROMPTER_EXPECTED_PROMPT_FILE", promptPath)

	writeTestFile(t, promptPath, "enhance this prompt")
	fixtureData, err := json.Marshal(fixtureSet{Fixtures: []fixture{{
		ID:    "structured-request",
		Args:  []string{"-test.run=TestEnhanceEvalHelperProcess", "--", "refine"},
		Input: "add tests",
	}}})
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, fixturePath, string(fixtureData))

	binary, err := filepath.Abs(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	args := []string{
		"--binary", binary,
		"--fixtures", fixturePath,
		"--manifest", manifestPath,
		"--prompt", promptPath,
		"--root", "../..",
	}
	if err := run(args); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if err := run(args); err != nil {
		t.Fatalf("resume run: %v", err)
	}

	rows := readManifestRows(t, manifestPath)
	if len(rows) != 1 {
		t.Fatalf("manifest rows = %d, want 1 after resume", len(rows))
	}
	row := rows[0]
	if row.Status != "completed" || row.CompletionClassification != "completed" || row.ExitCode != 0 {
		t.Fatalf("completion fields = status %q classification %q exit %d", row.Status, row.CompletionClassification, row.ExitCode)
	}
	for name, value := range map[string]string{
		"run key": row.RunKey, "binary hash": row.BinarySHA256, "git head": row.Source.GitHead,
		"dirty diff hash": row.Source.DirtyDiffSHA256, "untracked hash": row.Source.UntrackedSHA256, "prompt hash": row.PromptSHA256,
		"fixture hash": row.FixtureSHA256, "settings hash": row.EffectiveSettingsSHA256,
		"stdout hash": row.StdoutSHA256, "stderr hash": row.StderrSHA256,
	} {
		if value == "" {
			t.Errorf("%s is empty", name)
		}
	}
	if row.BinaryPath != binary || row.PromptPath != promptPath {
		t.Fatalf("bound paths = binary %q prompt %q", row.BinaryPath, row.PromptPath)
	}
	if row.EffectiveSettings["Max output tokens"] != "8192" {
		t.Fatalf("effective max output tokens = %q, want 8192", row.EffectiveSettings["Max output tokens"])
	}
	if row.ProviderRequestID != nil || row.ProviderUsage != nil || row.CompletionReason != nil {
		t.Fatalf("unavailable provider metadata must be null: %+v", row)
	}

	callLog, err := os.ReadFile(callLogPath)
	if err != nil {
		t.Fatalf("read call log: %v", err)
	}
	if strings.Count(string(callLog), "provider-call\n") != 1 {
		t.Fatalf("provider calls = %q, want one", callLog)
	}

	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"completion_reason":null`, `"provider_request_id":null`, `"provider_usage":null`} {
		if !strings.Contains(string(raw), field) {
			t.Errorf("manifest missing explicit null %s: %s", field, raw)
		}
	}
}

func TestRunAcceptsAllowlistedModelFlag(t *testing.T) {
	t.Setenv("PROMPTER_EVAL_HELPER", "1")
	tempDir := t.TempDir()
	promptPath := filepath.Join(tempDir, "enhance.md")
	fixturePath := filepath.Join(tempDir, "fixtures.json")
	manifestPath := filepath.Join(tempDir, "results.jsonl")
	callLogPath := filepath.Join(tempDir, "calls.log")
	t.Setenv("PROMPTER_EVAL_CALL_LOG", callLogPath)
	t.Setenv("PROMPTER_EXPECTED_PROMPT_FILE", promptPath)

	writeTestFile(t, promptPath, "enhance this prompt")
	fixtureData, err := json.Marshal(fixtureSet{Fixtures: []fixture{{
		ID:   "model-missing-value",
		Args: []string{"-test.run=TestEnhanceEvalHelperProcess", "--", "refine", "--model", "fake-model"},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, fixturePath, string(fixtureData))

	binary, err := filepath.Abs(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := run([]string{
		"--binary", binary,
		"--fixtures", fixturePath,
		"--manifest", manifestPath,
		"--prompt", promptPath,
		"--root", "../..",
	}); err != nil {
		t.Fatalf("run: %v", err)
	}

	callLog, err := os.ReadFile(callLogPath)
	if err != nil {
		t.Fatalf("read call log: %v", err)
	}
	if got := strings.Count(string(callLog), "provider-call\n"); got != 1 {
		t.Fatalf("provider calls = %d, want one non-dry execution call", got)
	}
}

func TestRunRejectsFixtureDryRunOverrideBeforeProviderCall(t *testing.T) {
	t.Setenv("PROMPTER_EVAL_HELPER", "1")
	tempDir := t.TempDir()
	promptPath := filepath.Join(tempDir, "enhance.md")
	fixturePath := filepath.Join(tempDir, "fixtures.json")
	manifestPath := filepath.Join(tempDir, "results.jsonl")
	callLogPath := filepath.Join(tempDir, "calls.log")
	t.Setenv("PROMPTER_EVAL_CALL_LOG", callLogPath)
	t.Setenv("PROMPTER_EXPECTED_PROMPT_FILE", promptPath)

	writeTestFile(t, promptPath, "enhance this prompt")
	fixtureData, err := json.Marshal(fixtureSet{Fixtures: []fixture{{
		ID:   "dry-run-override",
		Args: []string{"-test.run=TestEnhanceEvalHelperProcess", "refine", "--model", "--", "--dry-run=false"},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, fixturePath, string(fixtureData))

	binary, err := filepath.Abs(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	err = run([]string{
		"--binary", binary,
		"--fixtures", fixturePath,
		"--manifest", manifestPath,
		"--prompt", promptPath,
		"--root", "../..",
	})
	if err == nil || !strings.Contains(err.Error(), `fixture "dry-run-override" sets reserved --dry-run flag`) {
		t.Fatalf("run() error = %v, want reserved-flag rejection", err)
	}
	if _, err := os.Stat(callLogPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("provider helper call log stat = %v, want absent", err)
	}
}

func TestRunRejectsMalformedOrIncompleteManifestBeforeExecution(t *testing.T) {
	for name, content := range map[string]string{
		"malformed":  "{not-json}\n",
		"incomplete": `{"schema_version":1,"run_key":"key","fixture_id":"one","status":"completed"}` + "\n",
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("PROMPTER_EVAL_HELPER", "1")
			tempDir := t.TempDir()
			promptPath := filepath.Join(tempDir, "enhance.md")
			fixturePath := filepath.Join(tempDir, "fixtures.json")
			manifestPath := filepath.Join(tempDir, "results.jsonl")
			writeTestFile(t, promptPath, "prompt")
			writeTestFile(t, fixturePath, `{"fixtures":[{"id":"one","args":["refine"],"input":"input"}]}`)
			writeTestFile(t, manifestPath, content)
			binary, err := filepath.Abs(os.Args[0])
			if err != nil {
				t.Fatal(err)
			}

			err = run([]string{"--binary", binary, "--fixtures", fixturePath, "--manifest", manifestPath, "--prompt", promptPath, "--root", "../.."})
			if err == nil || !strings.Contains(err.Error(), "manifest line 1") {
				t.Fatalf("run error = %v, want manifest rejection", err)
			}
		})
	}
}

func TestEnhanceEvalHelperProcess(t *testing.T) {
	if os.Getenv("PROMPTER_EVAL_HELPER") != "1" {
		return
	}
	if got, want := os.Getenv("PROMPTER_PROMPT_FILE"), os.Getenv("PROMPTER_EXPECTED_PROMPT_FILE"); got == "" || got != want {
		fmt.Fprintf(os.Stderr, "PROMPTER_PROMPT_FILE=%q, want %q\n", got, want)
		os.Exit(3)
	}
	if *enhanceEvalHelperDryRun {
		fmt.Fprintln(os.Stderr, "Dry run: no API call made")
		fmt.Fprintln(os.Stderr, "Provider: fake")
		fmt.Fprintln(os.Stderr, "Model: fake-model")
		fmt.Fprintln(os.Stderr, "Base URL: default")
		fmt.Fprintln(os.Stderr, "Credential source: FAKE_API_KEY")
		fmt.Fprintln(os.Stderr, "Command: refine")
		fmt.Fprintln(os.Stderr, "Stream: false")
		fmt.Fprintln(os.Stderr, "Timeout: 1m0s")
		fmt.Fprintln(os.Stderr, "Max output tokens: 8192")
		fmt.Fprintln(os.Stderr, "Max retries: 3")
		fmt.Fprintln(os.Stderr, "Effort: low")
		fmt.Fprintln(os.Stderr, "System prompt bytes: 42")
		fmt.Fprintln(os.Stderr, "Input bytes: 9")
		os.Exit(0)
	}
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		os.Exit(2)
	}
	file, err := os.OpenFile(os.Getenv("PROMPTER_EVAL_CALL_LOG"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		os.Exit(2)
	}
	_, _ = io.WriteString(file, "provider-call\n")
	_ = file.Close()
	fmt.Printf("enhanced: %s", input)
	os.Exit(0)
}

func TestEnhanceEvalHangHelperProcess(t *testing.T) {
	if os.Getenv("PROMPTER_EVAL_HANG_HELPER") != "1" {
		return
	}
	time.Sleep(time.Hour)
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readManifestRows(t *testing.T, path string) []manifestRow {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	var rows []manifestRow
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var row manifestRow
		if err := json.Unmarshal(scanner.Bytes(), &row); err != nil {
			t.Fatal(err)
		}
		rows = append(rows, row)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return rows
}
