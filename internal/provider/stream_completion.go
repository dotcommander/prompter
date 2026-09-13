package provider

import "fmt"

func streamCompletionError(providerName string, maxOutputTokens int, wrote, completed bool, terminalErr error) error {
	if terminalErr != nil {
		return terminalErr
	}
	if !completed {
		return newCompletionError(providerName, "missing_terminal_status", maxOutputTokens, wrote)
	}
	if !wrote {
		return fmt.Errorf("%s: stream produced no output", providerName)
	}
	return nil
}
