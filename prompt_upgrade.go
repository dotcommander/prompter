package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type promptState string

const (
	promptCurrent    promptState = "current"
	promptCustom     promptState = "custom"
	promptCandidate  promptState = "candidate"
	promptMissing    promptState = "missing"
	promptUpgradable promptState = "upgrade"
)

type promptUpgrade struct {
	name    string
	state   promptState
	current []byte
}

type promptChecksumCatalog map[string]map[string]struct{}

func runPromptMaintenance(w io.Writer, vaultDir, action string, dryRun bool) error {
	upgrades, err := inspectPromptVault(vaultDir)
	if err != nil {
		return err
	}
	for _, item := range upgrades {
		originalState := item.state
		if action == "upgrade" && !dryRun {
			if err := applyPromptUpgrade(vaultDir, &item); err != nil {
				return err
			}
		}
		label := string(originalState)
		if action == "upgrade" {
			switch originalState {
			case promptMissing:
				label = "install"
			case promptUpgradable:
				label = "stage"
			}
			if dryRun && (originalState == promptMissing || originalState == promptUpgradable) {
				label = "would " + label
			}
		}
		fmt.Fprintf(w, "%-13s %s\n", label, item.name)
	}
	return nil
}

func inspectPromptVault(vaultDir string) ([]promptUpgrade, error) {
	legacy, err := loadReleasedPromptChecksums()
	if err != nil {
		return nil, err
	}
	return inspectPromptVaultWithChecksums(vaultDir, legacy)
}

func inspectPromptVaultWithChecksums(vaultDir string, legacy promptChecksumCatalog) ([]promptUpgrade, error) {
	entries, err := starterFS.ReadDir("prompts/starter")
	if err != nil {
		return nil, fmt.Errorf("read embedded starter prompts: %w", err)
	}
	result := make([]promptUpgrade, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		current, err := starterFS.ReadFile("prompts/starter/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read embedded starter prompt %s: %w", entry.Name(), err)
		}
		item := promptUpgrade{name: entry.Name(), current: current, state: promptMissing}
		path := filepath.Join(vaultDir, entry.Name())
		info, statErr := os.Lstat(path)
		switch {
		case os.IsNotExist(statErr):
		case statErr != nil:
			return nil, fmt.Errorf("inspect prompt %s: %w", path, statErr)
		case info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular():
			item.state = promptCustom
		default:
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return nil, fmt.Errorf("read prompt %s: %w", path, readErr)
			}
			hash := sha256.Sum256(data)
			observedHash := hashHex(hash)
			switch {
			case observedHash == checksum(current):
				item.state = promptCurrent
			case legacy.contains(entry.Name(), observedHash):
				item.state = promptUpgradable
			default:
				item.state = promptCustom
			}
			if item.state == promptUpgradable || item.state == promptCustom {
				candidate := promptCandidatePath(path, current)
				present, candidateErr := promptCandidatePresent(candidate, current)
				if candidateErr != nil {
					return nil, fmt.Errorf("inspect prompt candidate %s: %w", candidate, candidateErr)
				}
				if present {
					item.state = promptCandidate
				}
			}
		}
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].name < result[j].name })
	return result, nil
}

func applyPromptUpgrade(vaultDir string, item *promptUpgrade) error {
	path := filepath.Join(vaultDir, item.name)
	switch item.state {
	case promptCurrent, promptCandidate:
		return nil
	case promptMissing:
		if err := os.MkdirAll(vaultDir, 0o755); err != nil {
			return fmt.Errorf("create prompt vault %s: %w", vaultDir, err)
		}
		created, err := writeStarterExclusive(path, item.current, os.OpenFile)
		if err != nil {
			return fmt.Errorf("install prompt %s: %w", path, err)
		}
		if !created {
			return fmt.Errorf("install prompt %s: destination changed during upgrade", path)
		}
		item.state = promptCurrent
		return nil
	case promptUpgradable, promptCustom:
		candidate := promptCandidatePath(path, item.current)
		created, err := writeStarterExclusive(candidate, item.current, os.OpenFile)
		if err != nil {
			return fmt.Errorf("write prompt candidate %s: %w", candidate, err)
		}
		if created {
			item.name += " (new version: " + filepath.Base(candidate) + ")"
		} else {
			present, err := promptCandidatePresent(candidate, item.current)
			if err != nil {
				return fmt.Errorf("inspect prompt candidate %s: %w", candidate, err)
			}
			if !present {
				return fmt.Errorf("prompt candidate %s changed during upgrade", candidate)
			}
		}
		item.state = promptCandidate
		return nil
	default:
		return fmt.Errorf("unknown prompt state %q", item.state)
	}
}

func promptCandidatePath(path string, content []byte) string {
	hash := checksum(content)
	return path + ".new." + hash[:12]
}

func promptCandidatePresent(path string, current []byte) (bool, error) {
	info, err := os.Lstat(path)
	switch {
	case os.IsNotExist(err):
		return false, nil
	case err != nil:
		return false, err
	case !info.Mode().IsRegular():
		return false, fmt.Errorf("candidate is not a regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	if checksum(data) != checksum(current) {
		return false, fmt.Errorf("candidate contains different content")
	}
	return true, nil
}

func loadReleasedPromptChecksums() (promptChecksumCatalog, error) {
	entries, err := starterFS.ReadDir("prompts")
	if err != nil {
		return nil, fmt.Errorf("read embedded prompt checksums: %w", err)
	}
	catalog := make(promptChecksumCatalog)
	manifestCount := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, "starter-v") || !strings.HasSuffix(name, ".sha256") {
			continue
		}
		raw, err := starterFS.ReadFile("prompts/" + name)
		if err != nil {
			return nil, fmt.Errorf("read embedded prompt checksums %s: %w", name, err)
		}
		checksums, err := parsePromptChecksums(string(raw))
		if err != nil {
			return nil, fmt.Errorf("parse embedded prompt checksums %s: %w", name, err)
		}
		for promptName, hash := range checksums {
			catalog.add(promptName, hash)
		}
		manifestCount++
	}
	if manifestCount == 0 {
		return nil, fmt.Errorf("no embedded prompt checksum catalogs")
	}
	return catalog, nil
}

func (catalog promptChecksumCatalog) add(name, hash string) {
	if catalog[name] == nil {
		catalog[name] = make(map[string]struct{})
	}
	catalog[name][hash] = struct{}{}
}

func (catalog promptChecksumCatalog) contains(name, hash string) bool {
	_, ok := catalog[name][hash]
	return ok
}

func parsePromptChecksums(raw string) (map[string]string, error) {
	checksums := make(map[string]string)
	for lineNumber, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			return nil, fmt.Errorf("invalid starter checksum line %d", lineNumber+1)
		}
		if _, err := hex.DecodeString(fields[0]); err != nil || len(fields[0]) != sha256.Size*2 {
			return nil, fmt.Errorf("invalid starter checksum on line %d", lineNumber+1)
		}
		if _, exists := checksums[fields[1]]; exists {
			return nil, fmt.Errorf("duplicate starter checksum on line %d", lineNumber+1)
		}
		checksums[fields[1]] = fields[0]
	}
	return checksums, nil
}

func checksum(data []byte) string {
	hash := sha256.Sum256(data)
	return hashHex(hash)
}

func hashHex(hash [sha256.Size]byte) string {
	return hex.EncodeToString(hash[:])
}
