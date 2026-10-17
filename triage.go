package main

import (
	"fmt"
	"slices"
	"strings"
	"unicode"

	"github.com/dotcommander/prompter/internal/provider"
)

// -----------------------------------------------------------------------------
// L0 deterministic triage
//
// Zero-LLM, zero-dependency input sizing adapted from the non-autoregressive
// routing architecture: a conservative token estimate drives (1) optional
// input-sized provider routing and (2) a fail-fast context-window preflight
// that runs before any provider call.
// -----------------------------------------------------------------------------

const routingModeAuto = "auto"

// estimateTokens returns a conservative deterministic token estimate without
// loading a tokenizer: the larger of the whitespace-collapsed word count and
// one quarter of the byte length (~4 bytes per token for English text).
func estimateTokens(s string) int {
	words := 0
	inWord := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			inWord = false
			continue
		}
		if !inWord {
			words++
			inWord = true
		}
	}
	charEstimate := (len(s) + 3) / 4
	return max(words, charEstimate)
}

// routingEnabled reports whether auto routing owns provider selection for this
// invocation. The -p flag and any explicit provider selection (PROMPTER_PROVIDER
// or a config-file provider key) always win over routing_mode=auto; ambient
// provider detection (for example, an API key variable selecting the default)
// does not block routing.
func routingEnabled(mode, flagProvider, providerOrigin string) bool {
	if flagProvider != "" || mode != routingModeAuto {
		return false
	}
	switch providerOrigin {
	case "config file":
		return false
	default:
		return !strings.HasPrefix(providerOrigin, "environment: PROMPTER_PROVIDER")
	}
}

// routingDecision is the outcome of the L0 cascade for one input.
type routingDecision struct {
	Provider string
	Detail   string
}

// decideRoute applies the lexical cascade to the bounded input: estimated
// tokens within the local budget stay on the local tier; anything larger
// escalates to the frontier provider. Ambiguity escalates upward.
func decideRoute(localProvider, frontierProvider string, localMaxTokens int, input string) (routingDecision, error) {
	for _, name := range []string{localProvider, frontierProvider} {
		if !slices.Contains(provider.KnownNames(), name) {
			return routingDecision{}, fmt.Errorf("routing: %q is not a known provider (valid: %s)", name, provider.KnownNamesString())
		}
	}
	estimate := estimateTokens(boundPromptInput(input))
	if estimate > localMaxTokens {
		return routingDecision{
			Provider: frontierProvider,
			Detail:   fmt.Sprintf("%d est input tokens > %d local budget", estimate, localMaxTokens),
		}, nil
	}
	return routingDecision{
		Provider: localProvider,
		Detail:   fmt.Sprintf("%d est input tokens <= %d local budget", estimate, localMaxTokens),
	}, nil
}

// preflightContextWindow fails fast when the estimated prompt size cannot fit
// the configured provider context window. A zero or absent window skips the
// gate instead of guessing a limit; the estimate covers the system prompt,
// the bounded input envelope, and max_output_tokens.
func preflightContextWindow(window, maxOutputTokens int, systemPrompt, input string, providerName string) error {
	if window <= 0 {
		return nil
	}
	estimate := estimateTokens(systemPrompt) + estimateTokens(boundPromptInput(input))
	if estimate+maxOutputTokens > window {
		return fmt.Errorf("%s context window exceeded: ~%d est prompt tokens + %d max output tokens > %d window tokens; shorten the input, lower max_output_tokens, or correct %s context_window", providerName, estimate, maxOutputTokens, window, providerName)
	}
	return nil
}
