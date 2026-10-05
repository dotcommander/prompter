package provider

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// gcloudSystemCandidates lists common gcloud install locations that are
// frequently absent from the minimal PATH inherited by GUI-launched IDEs and
// other non-login subprocesses. PATH lookup still wins, so an explicit gcloud
// on PATH is always preferred.
var gcloudSystemCandidates = []string{
	"/opt/homebrew/bin/gcloud",               // Homebrew (Apple Silicon)
	"/usr/local/bin/gcloud",                  // Homebrew (Intel) / installer symlink
	"/usr/local/google-cloud-sdk/bin/gcloud", // macOS installer
	"/usr/lib/google-cloud-sdk/bin/gcloud",   // Debian/Ubuntu package
}

// gcloudDefaultCandidates returns system locations plus the user-local
// installer default (~/google-cloud-sdk/bin/gcloud).
func gcloudDefaultCandidates() []string {
	candidates := make([]string, 0, len(gcloudSystemCandidates)+1)
	candidates = append(candidates, gcloudSystemCandidates...)
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, "google-cloud-sdk", "bin", "gcloud"))
	}
	if runtime.GOOS == "windows" {
		for _, root := range []string{os.Getenv("LOCALAPPDATA"), os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)")} {
			if root != "" {
				candidates = append(candidates, filepath.Join(root, "Google", "Cloud SDK", "google-cloud-sdk", "bin", "gcloud"))
			}
		}
	}
	return candidates
}

// resolveGcloud returns the gcloud executable to use: the first gcloud found
// on PATH, else the first executable candidate. The returned error names every
// searched location so a missing-binary failure is distinguishable from a
// gcloud runtime failure.
func resolveGcloud(candidates []string) (string, error) {
	if path, err := exec.LookPath("gcloud"); err == nil {
		return path, nil
	}
	searched := make([]string, 0, len(candidates)+1)
	searched = append(searched, "PATH")
	for _, candidate := range candidates {
		searched = append(searched, candidate)
		if path := resolveGcloudCandidate(candidate, runtime.GOOS, os.Getenv("PATHEXT")); path != "" {
			return path, nil
		}
	}
	return "", fmt.Errorf("gcloud executable not found (searched %s)", strings.Join(searched, ", "))
}

// runGcloudADC resolves an application-default access token via gcloud. A
// failed gcloud invocation reports its exit status and stderr tail; a missing
// gcloud names every searched location.
func runGcloudADC(ctx context.Context, candidates []string) (string, error) {
	gcloudPath, err := resolveGcloud(candidates)
	if err != nil {
		return "", err
	}
	cmd := gcloudCommand(ctx, gcloudPath, "auth", "application-default", "print-access-token", "--scopes="+vertexADCScope, "--quiet")
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			detail := gcloudStderrDetail(exitErr.Stderr)
			if detail == "" {
				return "", fmt.Errorf("gcloud application-default print-access-token failed (exit %d) without diagnostic output", exitErr.ProcessState.ExitCode())
			}
			return "", fmt.Errorf("gcloud application-default print-access-token failed (exit %d): %s", exitErr.ProcessState.ExitCode(), detail)
		}
		return "", fmt.Errorf("gcloud application-default print-access-token: %w", err)
	}
	token := strings.TrimSpace(string(out))
	if token == "" {
		return "", errors.New("gcloud application-default print-access-token returned an empty token")
	}
	return token, nil
}

// gcloudStderrDetail collapses gcloud stderr to a single bounded line for
// embedding in authentication errors.
func gcloudStderrDetail(stderr []byte) string {
	const detailLimit = 300
	msg := strings.Join(strings.Fields(string(stderr)), " ")
	if msg == "" {
		return ""
	}
	if len(msg) > detailLimit {
		msg = msg[:detailLimit] + "..."
	}
	return msg
}

// googleADCAccessToken resolves a Google ADC bearer token for Vertex AI.
func googleADCAccessToken(ctx context.Context) (string, error) {
	return runGcloudADC(ctx, gcloudDefaultCandidates())
}

// resolveGcloudCandidate applies native executable rules; platform is explicit
// so Windows extension discovery can also be covered by portable fixtures.
func resolveGcloudCandidate(candidate, platform, pathExt string) string {
	if platform != "windows" {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return candidate
		}
		return ""
	}
	if pathExt == "" {
		pathExt = ".COM;.EXE;.BAT;.CMD"
	}
	extensions := strings.Split(strings.ToLower(pathExt), ";")
	ext := strings.ToLower(filepath.Ext(candidate))
	for _, extension := range extensions {
		extension = strings.TrimSpace(extension)
		if extension == "" || !strings.HasPrefix(extension, ".") {
			continue
		}
		path := candidate
		if ext == "" {
			path += extension
		} else if ext != extension {
			continue
		}
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path
		}
	}
	return ""
}
