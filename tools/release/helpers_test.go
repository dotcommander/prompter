package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{input: "0.4.0", want: "0.4.0"},
		{input: "v1.2.3", want: "1.2.3"},
		{input: "VERSION=2.0.1", want: "2.0.1"},
		{input: "1.2", wantErr: true},
		{input: "01.2.3", wantErr: true},
		{input: "1.2.3-beta", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			t.Parallel()
			got, err := normalizeVersion(test.input)
			if test.wantErr {
				if err == nil {
					t.Fatalf("normalizeVersion(%q) succeeded, want error", test.input)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("normalizeVersion(%q) = %q, want %q", test.input, got, test.want)
			}
		})
	}
}

func TestParseOptionsRequiresPublishForResume(t *testing.T) {
	t.Parallel()

	opts, err := parseOptions([]string{"0.4.0"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.publish || opts.resume || opts.version != "0.4.0" {
		t.Fatalf("parseOptions dry-run = %+v", opts)
	}
	if _, err := parseOptions([]string{"0.4.0", "--resume"}); err == nil {
		t.Fatal("parseOptions accepted --resume without --publish")
	}
	opts, err = parseOptions([]string{"--resume", "v0.4.0", "--publish"})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.publish || !opts.resume || opts.version != "0.4.0" {
		t.Fatalf("parseOptions publish resume = %+v", opts)
	}
}

func TestReplaceAppVersion(t *testing.T) {
	t.Parallel()

	input := []byte("package main\n\nconst AppVersion = \"0.3.0\"\n")
	updated, err := replaceAppVersion(input, "0.3.0", "0.4.0")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(updated), `const AppVersion = "0.4.0"`) {
		t.Fatalf("updated AppVersion = %s", updated)
	}
	idempotent, err := replaceAppVersion(updated, "0.3.0", "0.4.0")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(idempotent, updated) {
		t.Fatal("idempotent AppVersion update changed bytes")
	}
}

func TestBuildStarterManifestIsSortedAndVerifiable(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	for name, content := range map[string]string{"zeta.md": "z\n", "alpha.md": "a\n", "ignore.txt": "x"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	manifest, err := buildStarterManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(manifest)), "\n")
	if len(lines) != 2 || !strings.HasSuffix(lines[0], "  alpha.md") || !strings.HasSuffix(lines[1], "  zeta.md") {
		t.Fatalf("manifest is not sorted: %q", lines)
	}
	if err := verifyManifest(dir, manifest); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateFormulaChangesOnlyURLAndChecksum(t *testing.T) {
	t.Parallel()

	input := []byte(`class Prompter < Formula
  url "https://example.test/v0.3.0.tar.gz"
  sha256 "old"
  license "MIT"
end
`)
	updated, err := updateFormula(input, "https://example.test/v0.4.0.tar.gz", strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	want := `class Prompter < Formula
  url "https://example.test/v0.4.0.tar.gz"
  sha256 "` + strings.Repeat("a", 64) + `"
  license "MIT"
end
`
	if string(updated) != want {
		t.Fatalf("updated formula:\n%s\nwant:\n%s", updated, want)
	}
	if strings.Contains(string(updated), "version ") {
		t.Fatal("formula update added a redundant version stanza")
	}
}

func TestVerifyReleaseArchive(t *testing.T) {
	t.Parallel()

	starter := map[string][]byte{"alpha.md": []byte("alpha\n"), "beta.md": []byte("beta\n")}
	var manifest strings.Builder
	for _, name := range []string{"alpha.md", "beta.md"} {
		hash := sha256.Sum256(starter[name])
		manifest.WriteString(hex.EncodeToString(hash[:]) + "  " + name + "\n")
	}
	files := map[string][]byte{
		"cli_metadata.go":               []byte("package main\n\nconst AppVersion = \"0.4.0\"\n"),
		"prompts/critique.md":           []byte("critique"),
		"prompts/enhance.md":            []byte("enhance"),
		"prompts/rewrite.md":            []byte("rewrite"),
		"prompts/starter-v0.4.0.sha256": []byte(manifest.String()),
		"prompts/starter/alpha.md":      starter["alpha.md"],
		"prompts/starter/beta.md":       starter["beta.md"],
	}
	archive := makeArchive(t, "prompter-0.4.0/", files)
	builtinHashes := make(map[string]string)
	for _, name := range []string{"prompts/critique.md", "prompts/enhance.md", "prompts/rewrite.md"} {
		hash := sha256.Sum256(files[name])
		builtinHashes[name] = hex.EncodeToString(hash[:])
	}
	if err := verifyReleaseArchive(archive, "0.4.0", []byte(manifest.String()), builtinHashes); err != nil {
		t.Fatal(err)
	}
	builtinHashes["prompts/enhance.md"] = strings.Repeat("0", 64)
	if err := verifyReleaseArchive(archive, "0.4.0", []byte(manifest.String()), builtinHashes); err == nil {
		t.Fatal("verifyReleaseArchive accepted changed built-in prompt bytes")
	}
}

func makeArchive(t *testing.T, prefix string, files map[string][]byte) []byte {
	t.Helper()

	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	for name, content := range files {
		header := &tar.Header{Name: prefix + name, Mode: 0o644, Size: int64(len(content)), Typeflag: tar.TypeReg}
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if _, err := tarWriter.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
