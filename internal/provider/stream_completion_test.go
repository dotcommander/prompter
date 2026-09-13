package provider

import (
	"errors"
	"testing"
)

func TestStreamCompletionError(t *testing.T) {
	t.Parallel()

	terminal := errors.New("terminal failure")
	for _, test := range []struct {
		name           string
		wrote          bool
		completed      bool
		terminalErr    error
		wantTerminal   bool
		wantCompletion bool
		wantPartial    bool
		wantError      string
	}{
		{name: "terminal error takes precedence", terminalErr: terminal, wantTerminal: true},
		{name: "missing terminal without output", wantCompletion: true},
		{name: "missing terminal after output", wrote: true, wantCompletion: true, wantPartial: true},
		{name: "completed without output", completed: true, wantError: "test: stream produced no output"},
		{name: "completed with output", wrote: true, completed: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := streamCompletionError("test", 42, test.wrote, test.completed, test.terminalErr)
			if test.wantTerminal {
				if !errors.Is(err, terminal) {
					t.Fatalf("streamCompletionError error = %v, want terminal error", err)
				}
				return
			}
			if test.wantCompletion {
				var completion *CompletionError
				if !errors.As(err, &completion) {
					t.Fatalf("streamCompletionError error = %T %v, want CompletionError", err, err)
				}
				if completion.partial != test.wantPartial {
					t.Fatalf("CompletionError.partial = %t, want %t", completion.partial, test.wantPartial)
				}
				return
			}
			if test.wantError == "" {
				if err != nil {
					t.Fatalf("streamCompletionError error = %v, want nil", err)
				}
				return
			}
			if err == nil || err.Error() != test.wantError {
				t.Fatalf("streamCompletionError error = %v, want %q", err, test.wantError)
			}
		})
	}
}
