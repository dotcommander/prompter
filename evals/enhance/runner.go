package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

const (
	manifestSchemaVersion = 1
	defaultFixtureTimeout = 5 * time.Minute
	maxCommandOutputBytes = 16 << 20
)

var errCommandOutputLimit = errors.New("command output exceeds 16 MiB limit")

type fixtureSet struct {
	Fixtures []fixture `json:"fixtures"`
}

type fixture struct {
	ID    string   `json:"id"`
	Args  []string `json:"args"`
	Input string   `json:"input"`
}

type sourceIdentity struct {
	GitHead         string `json:"git_head"`
	DirtyDiffSHA256 string `json:"dirty_diff_sha256"`
	UntrackedSHA256 string `json:"untracked_sha256"`
	Dirty           bool   `json:"dirty"`
}

type manifestRow struct {
	SchemaVersion            int               `json:"schema_version"`
	RunKey                   string            `json:"run_key"`
	FixtureID                string            `json:"fixture_id"`
	FixtureSHA256            string            `json:"fixture_sha256"`
	Status                   string            `json:"status"`
	BinaryPath               string            `json:"binary_path"`
	BinarySHA256             string            `json:"binary_sha256"`
	Source                   sourceIdentity    `json:"source"`
	PromptPath               string            `json:"prompt_path"`
	PromptSHA256             string            `json:"prompt_sha256"`
	EffectiveSettings        map[string]string `json:"effective_settings"`
	EffectiveSettingsSHA256  string            `json:"effective_settings_sha256"`
	CommandArgs              []string          `json:"command_args"`
	StartedAt                time.Time         `json:"started_at"`
	DurationMillis           int64             `json:"duration_ms"`
	ExitCode                 int               `json:"exit_code"`
	StdoutSHA256             string            `json:"stdout_sha256"`
	StderrSHA256             string            `json:"stderr_sha256"`
	CompletionClassification string            `json:"completion_classification"`
	CompletionReason         *string           `json:"completion_reason"`
	ProviderRequestID        *string           `json:"provider_request_id"`
	ProviderUsage            any               `json:"provider_usage"`
}

type options struct {
	binary         string
	fixtures       string
	manifest       string
	prompt         string
	root           string
	fixtureTimeout time.Duration
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "enhance eval:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("enhance-eval", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var opts options
	fs.StringVar(&opts.binary, "binary", "", "absolute path to the prompter binary")
	fs.StringVar(&opts.fixtures, "fixtures", "", "fixture JSON file")
	fs.StringVar(&opts.manifest, "manifest", "", "append-only result JSONL file")
	fs.StringVar(&opts.prompt, "prompt", "", "enhancement prompt file used by the binary")
	fs.StringVar(&opts.root, "root", ".", "source repository root")
	fs.DurationVar(&opts.fixtureTimeout, "fixture-timeout", defaultFixtureTimeout, "maximum duration for each fixture subprocess")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if !filepath.IsAbs(opts.binary) {
		return errors.New("--binary must be an absolute path")
	}
	for name, value := range map[string]string{"--fixtures": opts.fixtures, "--manifest": opts.manifest, "--prompt": opts.prompt} {
		if value == "" {
			return fmt.Errorf("%s is required", name)
		}
	}
	if opts.fixtureTimeout <= 0 {
		return errors.New("--fixture-timeout must be positive")
	}
	root, err := filepath.Abs(opts.root)
	if err != nil {
		return fmt.Errorf("resolve source root: %w", err)
	}
	opts.root = root

	return execute(context.Background(), opts)
}

func execute(ctx context.Context, opts options) error {
	manifestPath, err := filepath.Abs(opts.manifest)
	if err != nil {
		return fmt.Errorf("resolve manifest path: %w", err)
	}
	if err := rejectManifestInSource(opts.root, manifestPath); err != nil {
		return err
	}
	lock, err := acquireManifestLock(manifestPath)
	if err != nil {
		return err
	}
	defer lock.release()

	binaryHash, err := hashFile(opts.binary)
	if err != nil {
		return fmt.Errorf("hash binary: %w", err)
	}
	promptPath, err := filepath.Abs(opts.prompt)
	if err != nil {
		return fmt.Errorf("resolve prompt path: %w", err)
	}
	promptHash, err := hashFile(promptPath)
	if err != nil {
		return fmt.Errorf("hash prompt: %w", err)
	}
	source, err := inspectSource(ctx, opts.root, manifestPath)
	if err != nil {
		return err
	}
	fixtures, err := readFixtures(opts.fixtures)
	if err != nil {
		return err
	}
	for _, fixture := range fixtures {
		if err := validateFixtureArgs(fixture.Args, opts.binary); err != nil {
			return fmt.Errorf("fixture %q: %w", fixture.ID, err)
		}
	}
	completed, err := readCompleted(manifestPath)
	if err != nil {
		return err
	}

	manifest, err := os.OpenFile(manifestPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open manifest: %w", err)
	}
	defer func() { _ = manifest.Close() }()
	encoder := json.NewEncoder(manifest)

	for _, fixture := range fixtures {
		fixtureData, err := json.Marshal(fixture)
		if err != nil {
			return fmt.Errorf("encode fixture %q: %w", fixture.ID, err)
		}
		fixtureHash := hashBytes(fixtureData)
		dryArgs := injectDryRunFlag(fixture.Args)
		dryEnv := evaluationEnv("", "", promptPath)
		dryCtx, cancelDry := context.WithTimeout(ctx, opts.fixtureTimeout)
		_, dryStderr, dryExit, err := invoke(dryCtx, opts.binary, dryArgs, fixture.Input, dryEnv)
		cancelDry()
		if err != nil {
			return fmt.Errorf("fixture %q dry run: %w", fixture.ID, err)
		}
		if dryExit != 0 {
			return fmt.Errorf("fixture %q dry run exited %d (stderr_sha256=%s)", fixture.ID, dryExit, hashBytes(dryStderr))
		}
		effective, err := parseEffectiveSettings(dryStderr)
		if err != nil {
			return fmt.Errorf("fixture %q dry run: %w", fixture.ID, err)
		}
		// Repeat the provider-free dry run with only the selected provider's
		// credential environment so credential-source identity matches execution.
		dryEnv = evaluationEnv(effective["Provider"], effective["Credential source"], promptPath)
		dryCtx, cancelDry = context.WithTimeout(ctx, opts.fixtureTimeout)
		_, dryStderr, dryExit, err = invoke(dryCtx, opts.binary, dryArgs, fixture.Input, dryEnv)
		cancelDry()
		if err != nil {
			return fmt.Errorf("fixture %q credential-aware dry run: %w", fixture.ID, err)
		}
		if dryExit != 0 {
			return fmt.Errorf("fixture %q credential-aware dry run exited %d (stderr_sha256=%s)", fixture.ID, dryExit, hashBytes(dryStderr))
		}
		effective, err = parseEffectiveSettings(dryStderr)
		if err != nil {
			return fmt.Errorf("fixture %q credential-aware dry run: %w", fixture.ID, err)
		}
		effectiveData, err := json.Marshal(effective)
		if err != nil {
			return fmt.Errorf("encode fixture %q effective settings: %w", fixture.ID, err)
		}
		effectiveHash := hashBytes(effectiveData)
		runKey := hashStrings(binaryHash, source.GitHead, source.DirtyDiffSHA256, source.UntrackedSHA256, promptHash, fixtureHash, effectiveHash)
		if completed[runKey] {
			continue
		}
		commandEnv := evaluationEnv(effective["Provider"], effective["Credential source"], promptPath)

		started := time.Now().UTC()
		fixtureCtx, cancelFixture := context.WithTimeout(ctx, opts.fixtureTimeout)
		stdout, stderr, exitCode, err := invoke(fixtureCtx, opts.binary, fixture.Args, fixture.Input, commandEnv)
		cancelFixture()
		duration := time.Since(started)
		if err != nil {
			return fmt.Errorf("fixture %q execution: %w", fixture.ID, err)
		}
		classification, reason := classifyCompletion(exitCode, stderr)
		row := manifestRow{
			SchemaVersion:            manifestSchemaVersion,
			RunKey:                   runKey,
			FixtureID:                fixture.ID,
			FixtureSHA256:            fixtureHash,
			Status:                   classification,
			BinaryPath:               opts.binary,
			BinarySHA256:             binaryHash,
			Source:                   source,
			PromptPath:               promptPath,
			PromptSHA256:             promptHash,
			EffectiveSettings:        effective,
			EffectiveSettingsSHA256:  effectiveHash,
			CommandArgs:              sanitizedFixtureArgs(fixture.Args),
			StartedAt:                started,
			DurationMillis:           duration.Milliseconds(),
			ExitCode:                 exitCode,
			StdoutSHA256:             hashBytes(stdout),
			StderrSHA256:             hashBytes(stderr),
			CompletionClassification: classification,
			CompletionReason:         reason,
			ProviderRequestID:        nil,
			ProviderUsage:            nil,
		}
		if err := encoder.Encode(row); err != nil {
			return fmt.Errorf("append fixture %q manifest row: %w", fixture.ID, err)
		}
		if err := manifest.Sync(); err != nil {
			return fmt.Errorf("sync fixture %q manifest row: %w", fixture.ID, err)
		}
		if classification == "completed" {
			completed[runKey] = true
		}
	}
	return nil
}

type manifestLock struct {
	path string
}

func acquireManifestLock(manifestPath string) (manifestLock, error) {
	lockPath := manifestPath + ".lock"
	if err := os.Mkdir(lockPath, 0o700); err != nil {
		if errors.Is(err, os.ErrExist) {
			return manifestLock{}, fmt.Errorf("manifest is locked by another evaluator: %s", lockPath)
		}
		return manifestLock{}, fmt.Errorf("create manifest lock: %w", err)
	}
	return manifestLock{path: lockPath}, nil
}

func (l manifestLock) release() {
	_ = os.Remove(l.path)
}

func rejectManifestInSource(root, manifestPath string) error {
	rootResolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve source root: %w", err)
	}
	parentResolved, err := filepath.EvalSymlinks(filepath.Dir(manifestPath))
	if err != nil {
		return fmt.Errorf("resolve manifest directory: %w", err)
	}
	resolvedManifest := filepath.Join(parentResolved, filepath.Base(manifestPath))
	if !strings.EqualFold(filepath.VolumeName(rootResolved), filepath.VolumeName(resolvedManifest)) {
		return nil
	}
	relative, err := filepath.Rel(rootResolved, resolvedManifest)
	if err != nil {
		return fmt.Errorf("compare manifest and source paths: %w", err)
	}
	if relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))) {
		return errors.New("--manifest must be outside the source repository")
	}
	return nil
}

func validateFixtureArgs(args []string, binary string) error {
	commandIndex, command := fixtureCommand(args)
	if command != "refine" {
		return errors.New("only the refine command is allowed")
	}
	if commandIndex != 0 {
		if !strings.HasSuffix(filepath.Base(binary), ".test") || commandIndex < 1 {
			return errors.New("refine must be the first argument")
		}
		for _, arg := range args[:commandIndex] {
			if arg != "--" && !strings.HasPrefix(arg, "-test.") {
				return errors.New("arguments before refine are not allowed")
			}
		}
	}
	for i := commandIndex + 1; i < len(args); i++ {
		arg := args[i]
		if arg == "-v" || arg == "--verbose" {
			continue
		}
		if !strings.HasPrefix(arg, "-") {
			return fmt.Errorf("positional argument %q is not allowed", arg)
		}
		name, value, inline := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		if name != "provider" && name != "p" && name != "model" && name != "m" && name != "style" && name != "s" {
			return fmt.Errorf("argument %q is not allowed", arg)
		}
		if inline {
			if strings.TrimSpace(value) == "" {
				return fmt.Errorf("argument %q has an empty value", arg)
			}
			continue
		}
		if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" || strings.HasPrefix(args[i+1], "-") {
			return fmt.Errorf("argument %q requires a value", arg)
		}
		i++
	}
	return nil
}

func sanitizedFixtureArgs(args []string) []string {
	commandIndex, _ := fixtureCommand(args)
	result := []string{"refine"}
	for i := commandIndex + 1; i < len(args); i++ {
		arg := args[i]
		if arg == "-v" || arg == "--verbose" {
			result = append(result, arg)
			continue
		}
		name, _, inline := strings.Cut(arg, "=")
		if inline {
			result = append(result, name+"=<redacted>")
			continue
		}
		result = append(result, arg, "<redacted>")
		i++
	}
	return result
}

func evaluationEnv(providerName, credentialSource, promptPath string) []string {
	allowed := map[string]bool{
		"HOME": true, "USERPROFILE": true, "APPDATA": true,
		"XDG_CONFIG_HOME":   true,
		"PROMPTER_PROVIDER": true, "PROMPTER_EFFORT": true,
		"PROMPTER_TIMEOUT": true, "PROMPTER_MAX_OUTPUT_TOKENS": true,
		"PROMPTER_MAX_RETRIES": true,
		"PROMPTER_EVAL_HELPER": true, "PROMPTER_EVAL_CALL_LOG": true,
		"PROMPTER_EXPECTED_PROMPT_FILE": true,
	}
	for _, name := range []string{"openai", "cerebras", "deepseek", "groq", "openrouter", "zai", "gemini", "omlx"} {
		prefix := strings.ToUpper(name)
		for _, suffix := range []string{"MODEL", "BASE_URL", "KEY_ENV"} {
			allowed[prefix+"_"+suffix] = true
			allowed["PROMPTER_"+prefix+"_"+suffix] = true
		}
	}
	for _, name := range []string{"PROMPTER_GEMINI_PROJECT_ID", "PROMPTER_GEMINI_LOCATION", "GOOGLE_CLOUD_PROJECT", "GCP_PROJECT", "GEMINI_PROJECT_ID", "GEMINI_LOCATION"} {
		allowed[name] = true
	}
	providerPrefix := strings.ToUpper(providerName)
	if providerPrefix != "" {
		allowed[providerPrefix+"_API_KEY"] = true
		allowed["PROMPTER_"+providerPrefix+"_API_KEY"] = true
	}
	if providerName == "gemini" {
		for _, name := range []string{"GOOGLE_APPLICATION_CREDENTIALS", "CLOUDSDK_CONFIG"} {
			allowed[name] = true
		}
	}
	credentialSource = strings.TrimPrefix(credentialSource, "config-or-")
	if credentialSource != "" && credentialSource != "google-adc" {
		allowed[credentialSource] = true
	}
	env := []string{"PROMPTER_PROMPT_FILE=" + promptPath, "PROMPTER_DEFAULT_COPY=false"}
	for _, item := range os.Environ() {
		name, _, _ := strings.Cut(item, "=")
		if allowed[name] {
			env = append(env, item)
		}
	}
	if runtime.GOOS == "windows" {
		for _, name := range []string{"SYSTEMROOT", "COMSPEC", "PATHEXT"} {
			if value, ok := os.LookupEnv(name); ok {
				env = append(env, name+"="+value)
			}
		}
	}
	return env
}

// injectDryRunFlag places the guard immediately after the command. Later
// placement can let a value-taking command flag consume the guard instead.
func injectDryRunFlag(args []string) []string {
	if len(args) == 0 {
		return []string{"--dry-run"}
	}
	result := make([]string, 0, len(args)+1)
	result = append(result, args[0], "--dry-run")
	return append(result, args[1:]...)
}

func readFixtures(path string) ([]fixture, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read fixtures: %w", err)
	}
	var set fixtureSet
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&set); err != nil {
		return nil, fmt.Errorf("decode fixtures: %w", err)
	}
	if len(set.Fixtures) == 0 {
		return nil, errors.New("fixtures file contains no fixtures")
	}
	seen := make(map[string]bool, len(set.Fixtures))
	for i := range set.Fixtures {
		f := &set.Fixtures[i]
		f.ID = strings.TrimSpace(f.ID)
		if f.ID == "" {
			return nil, fmt.Errorf("fixture %d has an empty id", i)
		}
		if seen[f.ID] {
			return nil, fmt.Errorf("duplicate fixture id %q", f.ID)
		}
		if len(f.Args) == 0 {
			return nil, fmt.Errorf("fixture %q has no command arguments", f.ID)
		}
		if fixtureSetsDryRun(f.Args) {
			return nil, fmt.Errorf("fixture %q sets reserved --dry-run flag", f.ID)
		}
		seen[f.ID] = true
	}
	return set.Fixtures, nil
}

var fixtureLLMValueFlags = map[string]bool{
	"provider": true,
	"p":        true,
	"model":    true,
	"m":        true,
	"base-url": true,
	"file":     true,
	"f":        true,
	"output":   true,
	"o":        true,
}

var fixtureImageValueFlags = map[string]bool{
	"file":       true,
	"f":          true,
	"output":     true,
	"o":          true,
	"profile":    true,
	"count":      true,
	"categories": true,
	"seed":       true,
}

// fixtureSetsDryRun mirrors the command argument grammar far enough to reserve
// the evaluator-owned dry-run guard. A value-taking flag consumes its next
// token even when that token is "--"; only a genuine boundary ends scanning.
func fixtureSetsDryRun(args []string) bool {
	commandIndex, command := fixtureCommand(args)
	if commandIndex < 0 {
		return false
	}
	for i := commandIndex + 1; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			return false
		}
		name, _, _ := strings.Cut(arg, "=")
		if name == "--dry-run" || name == "-dry-run" {
			return true
		}
		flagName, _, hasInlineValue := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		if !hasInlineValue && fixtureFlagTakesValue(command, flagName) {
			i++
		}
	}
	return false
}

func fixtureCommand(args []string) (int, string) {
	for i, arg := range args {
		switch arg {
		case "refine", "critique", "rewrite", "apply", "image", "browse", "configure", "models":
			return i, arg
		}
	}
	return -1, ""
}

func fixtureFlagTakesValue(command, name string) bool {
	switch command {
	case "refine":
		return fixtureLLMValueFlags[name] || name == "style" || name == "s"
	case "critique", "apply":
		return fixtureLLMValueFlags[name]
	case "rewrite":
		return fixtureLLMValueFlags[name] || name == "mode"
	case "image":
		return fixtureImageValueFlags[name]
	default:
		return false
	}
}

func readCompleted(path string) (map[string]bool, error) {
	completed := make(map[string]bool)
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return completed, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open existing manifest: %w", err)
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	line := 0
	for scanner.Scan() {
		line++
		var row manifestRow
		if err := json.Unmarshal(scanner.Bytes(), &row); err != nil {
			return nil, fmt.Errorf("decode manifest line %d: %w", line, err)
		}
		if err := validateManifestRow(row); err != nil {
			return nil, fmt.Errorf("manifest line %d is incomplete or incompatible", line)
		}
		if row.Status == "completed" {
			completed[row.RunKey] = true
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	return completed, nil
}

func validateManifestRow(row manifestRow) error {
	validStatus := row.Status == "completed" || row.Status == "incomplete" || row.Status == "failed"
	if row.SchemaVersion != manifestSchemaVersion || row.RunKey == "" || row.FixtureID == "" || row.FixtureSHA256 == "" ||
		!validStatus || row.Status != row.CompletionClassification || row.BinaryPath == "" || row.BinarySHA256 == "" ||
		row.Source.GitHead == "" || row.Source.DirtyDiffSHA256 == "" || row.Source.UntrackedSHA256 == "" ||
		row.PromptPath == "" || row.PromptSHA256 == "" || len(row.EffectiveSettings) == 0 || row.EffectiveSettingsSHA256 == "" ||
		len(row.CommandArgs) == 0 || row.StartedAt.IsZero() || row.StdoutSHA256 == "" || row.StderrSHA256 == "" {
		return errors.New("missing required manifest field")
	}
	return nil
}

func inspectSource(ctx context.Context, root, manifestPath string) (sourceIdentity, error) {
	head, err := gitOutput(ctx, root, "rev-parse", "HEAD")
	if err != nil {
		return sourceIdentity{}, fmt.Errorf("resolve git HEAD: %w", err)
	}
	diff, err := gitOutputBytes(ctx, root, "diff", "--binary", "HEAD", "--", ".")
	if err != nil {
		return sourceIdentity{}, fmt.Errorf("read dirty diff: %w", err)
	}
	status, err := gitOutputBytes(ctx, root, "status", "--porcelain=v1", "--untracked-files=normal")
	if err != nil {
		return sourceIdentity{}, fmt.Errorf("read git status: %w", err)
	}
	untrackedHash, err := hashUntracked(ctx, root, manifestPath)
	if err != nil {
		return sourceIdentity{}, err
	}
	return sourceIdentity{
		GitHead:         strings.TrimSpace(head),
		DirtyDiffSHA256: hashBytes(diff),
		UntrackedSHA256: untrackedHash,
		Dirty:           len(bytes.TrimSpace(status)) > 0,
	}, nil
}

func hashUntracked(ctx context.Context, root, manifestPath string) (string, error) {
	data, err := gitOutputBytes(ctx, root, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return "", fmt.Errorf("list untracked source files: %w", err)
	}
	manifestAbs, err := filepath.Abs(manifestPath)
	if err != nil {
		return "", fmt.Errorf("resolve manifest path: %w", err)
	}
	paths := strings.Split(strings.TrimSuffix(string(data), "\x00"), "\x00")
	if len(paths) == 1 && paths[0] == "" {
		paths = nil
	}
	sort.Strings(paths)
	h := sha256.New()
	for _, relative := range paths {
		path := filepath.Join(root, filepath.FromSlash(relative))
		pathAbs, err := filepath.Abs(path)
		if err != nil {
			return "", err
		}
		if pathAbs == manifestAbs {
			continue
		}
		info, err := os.Lstat(path)
		if err != nil {
			return "", fmt.Errorf("inspect untracked source %q: %w", relative, err)
		}
		_, _ = io.WriteString(h, relative)
		_, _ = h.Write([]byte{0})
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return "", fmt.Errorf("read untracked symlink %q: %w", relative, err)
			}
			_, _ = io.WriteString(h, target)
		} else {
			file, err := os.Open(path)
			if err != nil {
				return "", fmt.Errorf("open untracked source %q: %w", relative, err)
			}
			if _, err := io.Copy(h, file); err != nil {
				_ = file.Close()
				return "", fmt.Errorf("hash untracked source %q: %w", relative, err)
			}
			if err := file.Close(); err != nil {
				return "", fmt.Errorf("close untracked source %q: %w", relative, err)
			}
		}
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func gitOutput(ctx context.Context, root string, args ...string) (string, error) {
	out, err := gitOutputBytes(ctx, root, args...)
	return string(out), err
}

func gitOutputBytes(ctx context.Context, root string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return out, nil
}

func invoke(ctx context.Context, binary string, args []string, input string, env []string) ([]byte, []byte, int, error) {
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Stdin = strings.NewReader(input)
	cmd.Env = env
	stdout := limitedBuffer{limit: maxCommandOutputBytes}
	stderr := limitedBuffer{limit: maxCommandOutputBytes}
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		return stdout.Bytes(), stderr.Bytes(), 0, nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return stdout.Bytes(), stderr.Bytes(), -1, ctxErr
	}
	if errors.Is(err, errCommandOutputLimit) {
		return stdout.Bytes(), stderr.Bytes(), -1, errCommandOutputLimit
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return stdout.Bytes(), stderr.Bytes(), exitErr.ExitCode(), nil
	}
	return nil, nil, -1, err
}

type limitedBuffer struct {
	buffer bytes.Buffer
	limit  int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	remaining := b.limit - b.buffer.Len()
	if remaining <= 0 {
		return 0, errCommandOutputLimit
	}
	if len(p) > remaining {
		_, _ = b.buffer.Write(p[:remaining])
		return remaining, errCommandOutputLimit
	}
	return b.buffer.Write(p)
}

func (b *limitedBuffer) Bytes() []byte {
	return b.buffer.Bytes()
}

func parseEffectiveSettings(stderr []byte) (map[string]string, error) {
	allowed := map[string]bool{
		"Provider": true, "Model": true, "Command": true, "Mode": true,
		"Prompt": true, "Style": true, "Stream": true, "Timeout": true,
		"Max output tokens": true, "Effort": true, "System prompt bytes": true,
		"Input bytes": true, "Base URL": true, "Credential source": true,
		"Project ID": true, "Location": true, "Max retries": true,
	}
	settings := make(map[string]string)
	scanner := bufio.NewScanner(bytes.NewReader(stderr))
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), ":")
		key = strings.TrimSpace(key)
		if ok && allowed[key] {
			settings[key] = strings.TrimSpace(value)
		}
	}
	for _, required := range []string{"Provider", "Model", "Command", "Base URL", "Credential source", "Max output tokens", "Max retries"} {
		if settings[required] == "" {
			return nil, fmt.Errorf("missing required effective setting %q", required)
		}
	}
	return settings, nil
}

func classifyCompletion(exitCode int, stderr []byte) (string, *string) {
	if exitCode == 0 {
		return "completed", nil
	}
	message := strings.ToLower(string(stderr))
	for _, reason := range []string{"max_output_tokens", "max_tokens", "missing_terminal_status"} {
		if strings.Contains(message, reason) {
			return "incomplete", stringPointer(reason)
		}
	}
	if strings.Contains(message, "output token limit") || strings.Contains(message, "partial output") {
		return "incomplete", stringPointer("output_token_limit")
	}
	return "failed", nil
}

func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func hashStrings(values ...string) string {
	h := sha256.New()
	for _, value := range values {
		_, _ = io.WriteString(h, value)
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func stringPointer(value string) *string {
	return &value
}
