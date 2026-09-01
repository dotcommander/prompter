package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (w *releaseWorkflow) inspectSource(ctx context.Context, fetch bool) (string, error) {
	if err := requireTools("git", "gh", "brew", "just"); err != nil {
		return "", err
	}
	if err := w.requireCleanRepository(ctx, w.root); err != nil {
		return "", err
	}
	branch, err := w.proc.capture(ctx, w.root, nil, "git", "branch", "--show-current")
	if err != nil {
		return "", err
	}
	if branch != "main" {
		return "", fmt.Errorf("source branch is %q, want main", branch)
	}
	remote, err := w.proc.capture(ctx, w.root, nil, "git", "remote", "get-url", "origin")
	if err != nil {
		return "", err
	}
	repository, err := githubRepository(remote)
	if err != nil {
		return "", err
	}
	if repository != sourceRepository {
		return "", fmt.Errorf("origin is %s, want %s", repository, sourceRepository)
	}
	versionBytes, err := os.ReadFile(filepath.Join(w.root, "cli_metadata.go"))
	if err != nil {
		return "", err
	}
	current, err := appVersion(versionBytes)
	if err != nil {
		return "", err
	}
	if fetch {
		if err := w.proc.stream(ctx, w.root, nil, "git", "fetch", "--prune", "--tags", "origin"); err != nil {
			return "", err
		}
		counts, err := w.proc.capture(ctx, w.root, nil, "git", "rev-list", "--left-right", "--count", "origin/main...HEAD")
		if err != nil {
			return "", err
		}
		fields := strings.Fields(counts)
		if len(fields) != 2 || fields[0] != "0" {
			return "", fmt.Errorf("source branch is behind or diverged from origin/main: %s", counts)
		}
	} else {
		if _, err := w.proc.capture(ctx, w.root, nil, "git", "ls-remote", "--exit-code", "origin", "refs/heads/main"); err != nil {
			return "", fmt.Errorf("verify remote main: %w", err)
		}
	}
	localTag, err := w.proc.capture(ctx, w.root, nil, "git", "tag", "--list", w.tag)
	if err != nil {
		return "", err
	}
	if localTag != "" {
		return "", fmt.Errorf("local tag %s already exists", w.tag)
	}
	remoteTag, err := w.proc.capture(ctx, w.root, nil, "git", "ls-remote", "--tags", "origin", "refs/tags/"+w.tag)
	if err != nil {
		return "", err
	}
	if remoteTag != "" {
		return "", fmt.Errorf("remote tag %s already exists", w.tag)
	}
	return current, nil
}

func (w *releaseWorkflow) sourcePreflight(ctx context.Context) error {
	current, err := w.inspectSource(ctx, true)
	if err != nil {
		return err
	}
	if compareVersions(w.version, current) <= 0 {
		return fmt.Errorf("target version %s must be newer than AppVersion %s", w.version, current)
	}
	if err := w.proc.stream(ctx, w.root, nil, "gh", "auth", "status"); err != nil {
		return err
	}
	start, err := w.proc.capture(ctx, w.root, nil, "git", "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	remoteBase, err := w.proc.capture(ctx, w.root, nil, "git", "rev-parse", "origin/main")
	if err != nil {
		return err
	}
	w.state.PreviousVersion = current
	w.state.SourceStart = start
	w.state.SourceRemoteBase = remoteBase
	return nil
}

func (w *releaseWorkflow) sourcePrepare(context.Context) error {
	versionPath := filepath.Join(w.root, "cli_metadata.go")
	content, err := os.ReadFile(versionPath)
	if err != nil {
		return err
	}
	updated, err := replaceAppVersion(content, w.state.PreviousVersion, w.version)
	if err != nil {
		return err
	}
	if err := writeFileAtomic(versionPath, updated, 0o644); err != nil {
		return fmt.Errorf("write AppVersion: %w", err)
	}

	manifest, err := buildStarterManifest(filepath.Join(w.root, "prompts", "starter"))
	if err != nil {
		return fmt.Errorf("generate starter manifest: %w", err)
	}
	manifestPath := w.manifestPath()
	if existing, readErr := os.ReadFile(manifestPath); readErr == nil {
		if string(existing) != string(manifest) {
			return fmt.Errorf("existing %s does not match current starter prompts", filepath.Base(manifestPath))
		}
		return nil
	} else if !os.IsNotExist(readErr) {
		return readErr
	}
	if err := writeFileAtomic(manifestPath, manifest, 0o644); err != nil {
		return fmt.Errorf("write starter manifest: %w", err)
	}
	return nil
}

func (w *releaseWorkflow) sourceVerify(ctx context.Context) error {
	manifest, err := os.ReadFile(w.manifestPath())
	if err != nil {
		return err
	}
	if err := verifyManifest(filepath.Join(w.root, "prompts", "starter"), manifest); err != nil {
		return err
	}
	qaErr := w.proc.stream(ctx, w.root, nil, "just", "qa")
	cleanErr := w.proc.stream(ctx, w.root, nil, "just", "clean")
	if qaErr != nil || cleanErr != nil {
		return errors.Join(qaErr, cleanErr)
	}
	if err := w.expectReleasePreparationPaths(ctx); err != nil {
		return err
	}
	if err := w.proc.stream(ctx, w.root, nil, "git", "diff", "--check"); err != nil {
		return err
	}
	return w.proc.stream(ctx, w.root, nil, "git", "diff", "--", "cli_metadata.go", filepath.ToSlash(filepath.Join("prompts", filepath.Base(w.manifestPath()))))
}

func (w *releaseWorkflow) sourceCommit(ctx context.Context) error {
	clean, err := w.repositoryClean(ctx, w.root)
	if err != nil {
		return err
	}
	if clean {
		if err := w.validateReleaseCommit(ctx); err == nil {
			commit, captureErr := w.proc.capture(ctx, w.root, nil, "git", "rev-parse", "HEAD")
			w.state.ReleaseCommit = commit
			return captureErr
		}
		return errors.New("source tree is clean but the release commit is not present")
	}
	manifestRel := filepath.ToSlash(filepath.Join("prompts", filepath.Base(w.manifestPath())))
	if err := w.proc.stream(ctx, w.root, nil, "git", "--literal-pathspecs", "add", "--", "cli_metadata.go", manifestRel); err != nil {
		return err
	}
	if err := w.proc.stream(ctx, w.root, nil, "git", "diff", "--cached", "--check"); err != nil {
		return err
	}
	if err := w.proc.stream(ctx, w.root, nil, "git", "diff", "--cached"); err != nil {
		return err
	}
	if err := w.proc.stream(ctx, w.root, nil, "git", "commit", "-m", "chore(release): set version "+w.version,
		"-m", "Record the current starter prompt checksums for future upgrade detection."); err != nil {
		return err
	}
	commit, err := w.proc.capture(ctx, w.root, nil, "git", "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	w.state.ReleaseCommit = commit
	return w.requireCleanRepository(ctx, w.root)
}

func (w *releaseWorkflow) validateReleaseCommit(ctx context.Context) error {
	subject, err := w.proc.capture(ctx, w.root, nil, "git", "log", "-1", "--format=%s")
	if err != nil {
		return err
	}
	if subject != "chore(release): set version "+w.version {
		return fmt.Errorf("HEAD subject is %q", subject)
	}
	versionContent, err := w.proc.capture(ctx, w.root, nil, "git", "show", "HEAD:cli_metadata.go")
	if err != nil {
		return err
	}
	version, err := appVersion([]byte(versionContent))
	if err != nil || version != w.version {
		return errors.New("HEAD does not contain the target AppVersion")
	}
	manifestRel := filepath.ToSlash(filepath.Join("prompts", filepath.Base(w.manifestPath())))
	_, err = w.proc.capture(ctx, w.root, nil, "git", "show", "HEAD:"+manifestRel)
	return err
}

func (w *releaseWorkflow) sourceTag(ctx context.Context) error {
	localTag, err := w.proc.capture(ctx, w.root, nil, "git", "tag", "--list", w.tag)
	if err != nil {
		return err
	}
	if localTag != "" {
		target, err := w.proc.capture(ctx, w.root, nil, "git", "rev-parse", w.tag+"^{}")
		if err != nil || target != w.state.ReleaseCommit {
			return fmt.Errorf("existing tag %s does not target release commit", w.tag)
		}
		return nil
	}
	return w.proc.stream(ctx, w.root, nil, "git", "tag", "-a", w.tag, w.state.ReleaseCommit, "-m", "Release "+w.tag)
}

func (w *releaseWorkflow) sourcePublish(ctx context.Context) error {
	remoteHead, remoteTag, err := w.sourceRemoteState(ctx)
	if err != nil {
		return err
	}
	if remoteHead == w.state.ReleaseCommit && remoteTag == w.state.ReleaseCommit {
		return nil
	}
	if err := w.proc.stream(ctx, w.root, nil, "git", "fetch", "--prune", "--tags", "origin"); err != nil {
		return err
	}
	currentBase, err := w.proc.capture(ctx, w.root, nil, "git", "rev-parse", "origin/main")
	if err != nil {
		return err
	}
	if currentBase != w.state.SourceRemoteBase {
		return fmt.Errorf("origin/main drifted from %s to %s", shortOID(w.state.SourceRemoteBase), shortOID(currentBase))
	}
	if remoteTag != "" {
		return fmt.Errorf("remote tag %s appeared during release", w.tag)
	}
	if err := w.proc.stream(ctx, w.root, nil, "git", "push", "--atomic", "origin", "main", "refs/tags/"+w.tag); err != nil {
		return err
	}
	remoteHead, remoteTag, err = w.sourceRemoteState(ctx)
	if err != nil {
		return err
	}
	if remoteHead != w.state.ReleaseCommit || remoteTag != w.state.ReleaseCommit {
		return errors.New("remote source branch or tag does not match release commit")
	}
	return nil
}
