package usage_test

import (
	"os"
	"strings"
	"testing"

	"github.com/dotcommander/prompter/internal/provider"
)

func TestProviderDocsListRegisteredProviders(t *testing.T) {
	t.Parallel()

	providerDoc, err := os.ReadFile("../docs/providers.md")
	if err != nil {
		t.Fatalf("read providers.md: %v", err)
	}
	agentDoc, err := os.ReadFile("../AGENTS.md")
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}

	for _, name := range provider.KnownNames() {
		if !strings.Contains(string(providerDoc), name) {
			t.Errorf("docs/providers.md missing provider %q", name)
		}
		if !strings.Contains(strings.ToLower(string(agentDoc)), name) {
			t.Errorf("AGENTS.md missing provider %q", name)
		}
	}
}
