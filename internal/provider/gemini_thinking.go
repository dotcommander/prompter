package provider

import (
	"fmt"
	"strings"
)

// geminiThinkingLevels maps known Gemini models to the thinkingLevel values
// they accept on generateContent. Unknown models fail open so new releases
// keep working until this table catches up.
// Source: https://aistudio.google.com/docs/thinking#thinking-levels
var geminiThinkingLevels = map[string][]string{
	"gemini-3.8-flash":             {"low", "medium", "high"},
	"gemini-3.7-flash":             {"low", "medium", "high"},
	"gemini-3.6-flash":             {"minimal", "low", "medium", "high"},
	"gemini-3.5-flash":             {"minimal", "low", "medium", "high"},
	"gemini-3.5-flash-lite":        {"minimal", "low", "medium", "high"},
	"gemini-3.1-pro-preview":       {"low", "medium", "high"},
	"gemini-3-flash-preview":       {"minimal", "low", "medium", "high"},
	"gemini-3-pro-preview":         {"low", "high"},
	"gemini-2.5-pro":               {"low", "medium", "high"},
	"gemini-2.5-flash":             {"low", "medium", "high"},
	"gemini-2.5-flash-lite":        {"low", "medium", "high"},
	"gemini-robotics-er-2-preview": {"minimal", "low", "medium", "high"},
}

// validateGeminiThinkingLevel rejects efforts that the model's documented
// thinkingLevel set does not include. The Gemini API expects lowercase level
// strings; effort is normalized here because CallRequest bypasses config
// validation in tests and future callers.
func validateGeminiThinkingLevel(model, effort string) error {
	level := strings.ToLower(strings.TrimSpace(effort))
	normalizedModel := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(model)), "models/")
	supported, known := geminiThinkingLevels[normalizedModel]
	if !known {
		return nil
	}
	for _, candidate := range supported {
		if candidate == level {
			return nil
		}
	}
	return fmt.Errorf("gemini: model %q does not support thinking level %q (supported: %s)", normalizedModel, level, strings.Join(supported, ", "))
}
