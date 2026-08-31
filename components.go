package main

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
)

type ComponentLibrary struct {
	Subjects  []PromptSubject  `json:"subjects"`
	Modifiers []PromptModifier `json:"modifiers"`
	Artists   []PromptArtist   `json:"artists"`
	Platforms []PromptPlatform `json:"platforms"`
}

type PromptSubject struct {
	Subject  string `json:"subject"`
	Category string `json:"category,omitempty"`
}

type PromptModifier struct {
	Text         string   `json:"text"`
	Category     string   `json:"category"`
	SlotPosition int      `json:"slot_position"`
	Weight       float64  `json:"weight"`
	Compatible   []string `json:"compatible_categories,omitempty"`
}

type PromptArtist struct {
	Name       string   `json:"name"`
	StyleTags  []string `json:"style_tags,omitempty"`
	Categories []string `json:"categories,omitempty"`
	Weight     float64  `json:"weight"`
}

type PromptPlatform struct {
	Name   string  `json:"name"`
	Phrase string  `json:"phrase"`
	Weight float64 `json:"weight"`
}

type AssemblyProfile struct {
	Name            string
	Selections      []ModifierSelection
	IncludeArtist   bool
	IncludePlatform bool
}

type ModifierSelection struct {
	Category  string
	Count     int
	MinWeight float64
}

type AssembledPrompt struct {
	Subject    *PromptSubject   `json:"subject,omitempty"`
	Modifiers  []PromptModifier `json:"modifiers,omitempty"`
	Artist     *PromptArtist    `json:"artist,omitempty"`
	Platform   *PromptPlatform  `json:"platform,omitempty"`
	FullPrompt string           `json:"full_prompt"`
	Profile    string           `json:"profile"`
}

func defaultAssemblyProfile() AssemblyProfile {
	return AssemblyProfile{
		Name: "default",
		Selections: []ModifierSelection{
			{Category: "quality", Count: 2, MinWeight: 0.5},
			{Category: "lighting", Count: 1, MinWeight: 0.3},
			{Category: "style", Count: 1, MinWeight: 0.5},
		},
		IncludeArtist:   true,
		IncludePlatform: true,
	}
}

func minimalAssemblyProfile() AssemblyProfile {
	return AssemblyProfile{
		Name: "minimal",
		Selections: []ModifierSelection{
			{Category: "quality", Count: 1, MinWeight: 0.7},
			{Category: "style", Count: 1, MinWeight: 0.6},
		},
		IncludeArtist:   false,
		IncludePlatform: false,
	}
}

func maximalAssemblyProfile() AssemblyProfile {
	return AssemblyProfile{
		Name: "maximal",
		Selections: []ModifierSelection{
			{Category: "quality", Count: 3, MinWeight: 0.3},
			{Category: "lighting", Count: 2, MinWeight: 0.2},
			{Category: "style", Count: 2, MinWeight: 0.3},
			{Category: "render", Count: 1, MinWeight: 0.4},
			{Category: "composition", Count: 1, MinWeight: 0.3},
		},
		IncludeArtist:   true,
		IncludePlatform: true,
	}
}

func assemblyProfile(name string) (AssemblyProfile, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "default":
		return defaultAssemblyProfile(), nil
	case "minimal":
		return minimalAssemblyProfile(), nil
	case "maximal":
		return maximalAssemblyProfile(), nil
	default:
		return AssemblyProfile{}, fmt.Errorf("unknown assembly profile %q (valid: default, minimal, maximal)", name)
	}
}

func loadComponentLibrary(path string) (*ComponentLibrary, error) {
	data := []byte(defaultComponentsJSON)
	if strings.TrimSpace(path) != "" {
		read, err := os.ReadFile(path)
		if err == nil {
			data = read
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("read components file %s: %w", path, err)
		}
	}

	var lib ComponentLibrary
	if err := json.Unmarshal(data, &lib); err != nil {
		return nil, fmt.Errorf("parse components: %w", err)
	}
	if len(lib.Modifiers) == 0 {
		return nil, fmt.Errorf("component library has no modifiers")
	}
	return &lib, nil
}

func assemblePrompt(lib *ComponentLibrary, query string, profile AssemblyProfile, categories []string, seed string) (*AssembledPrompt, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("empty subject")
	}
	if lib == nil {
		return nil, fmt.Errorf("component library is nil")
	}
	subject := lib.matchSubject(query)
	if subject == nil {
		subject = &PromptSubject{Subject: query}
	}

	result := &AssembledPrompt{Subject: subject, Profile: profile.Name}
	if len(categories) > 0 {
		profile.Selections = make([]ModifierSelection, 0, len(categories))
		for _, category := range categories {
			category = strings.TrimSpace(category)
			if category == "" {
				continue
			}
			profile.Selections = append(profile.Selections, ModifierSelection{Category: category, Count: 1, MinWeight: 0.3})
		}
		if len(profile.Selections) == 0 {
			return nil, fmt.Errorf("no valid categories")
		}
		result.Profile = "custom"
	}

	for _, selection := range profile.Selections {
		result.Modifiers = append(result.Modifiers, lib.pickModifiers(selection, subject.Category, seed, query)...)
	}
	if profile.IncludeArtist {
		result.Artist = lib.pickArtist(subject.Category, seed, query)
	}
	if profile.IncludePlatform {
		result.Platform = lib.pickPlatform(seed, query)
	}
	result.FullPrompt = buildAssembledPromptString(result)
	return result, nil
}

func (lib *ComponentLibrary) matchSubject(query string) *PromptSubject {
	lower := strings.ToLower(query)
	for i := range lib.Subjects {
		if strings.EqualFold(lib.Subjects[i].Subject, query) || strings.Contains(lower, strings.ToLower(lib.Subjects[i].Subject)) {
			subject := lib.Subjects[i]
			subject.Subject = query
			return &subject
		}
	}
	return nil
}

func (lib *ComponentLibrary) pickModifiers(selection ModifierSelection, subjectCategory, seed, query string) []PromptModifier {
	var candidates []PromptModifier
	for _, modifier := range lib.Modifiers {
		if modifier.Category != selection.Category || modifier.Weight < selection.MinWeight {
			continue
		}
		if len(modifier.Compatible) > 0 && subjectCategory != "" && !slices.Contains(modifier.Compatible, subjectCategory) {
			continue
		}
		candidates = append(candidates, modifier)
	}
	if len(candidates) == 0 {
		return nil
	}
	sortModifiers(candidates, selection.Category, seed, query)
	count := min(selection.Count, len(candidates))
	return append([]PromptModifier(nil), candidates[:count]...)
}

func (lib *ComponentLibrary) pickArtist(subjectCategory, seed, query string) *PromptArtist {
	var candidates []PromptArtist
	for _, artist := range lib.Artists {
		if len(artist.Categories) == 0 || subjectCategory == "" || slices.Contains(artist.Categories, subjectCategory) {
			candidates = append(candidates, artist)
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	slices.SortFunc(candidates, func(a, b PromptArtist) int {
		return compareWeightedChoice(seed, query, "artist", a.Name, b.Name, a.Weight, b.Weight)
	})
	return &candidates[0]
}

func (lib *ComponentLibrary) pickPlatform(seed, query string) *PromptPlatform {
	if len(lib.Platforms) == 0 {
		return nil
	}
	candidates := append([]PromptPlatform(nil), lib.Platforms...)
	slices.SortFunc(candidates, func(a, b PromptPlatform) int {
		return compareWeightedChoice(seed, query, "platform", a.Name, b.Name, a.Weight, b.Weight)
	})
	return &candidates[0]
}

func sortModifiers(modifiers []PromptModifier, category, seed, query string) {
	slices.SortFunc(modifiers, func(a, b PromptModifier) int {
		return compareWeightedChoice(seed, query, category, a.Text, b.Text, a.Weight, b.Weight)
	})
}

func compareWeightedChoice(seed, query, scope, a, b string, weightA, weightB float64) int {
	scoreA := stableChoiceScore(seed, query, scope, a) * max(weightA, 0.01)
	scoreB := stableChoiceScore(seed, query, scope, b) * max(weightB, 0.01)
	switch {
	case scoreA > scoreB:
		return -1
	case scoreA < scoreB:
		return 1
	default:
		return strings.Compare(a, b)
	}
}

func stableChoiceScore(seed, query, scope, value string) float64 {
	h := sha256.Sum256([]byte(seed + "\x00" + query + "\x00" + scope + "\x00" + value))
	n := binary.BigEndian.Uint64(h[:8])
	return float64(n) / float64(^uint64(0))
}

func buildAssembledPromptString(assembled *AssembledPrompt) string {
	type part struct {
		text string
		slot int
	}
	var parts []part
	if assembled.Subject != nil && assembled.Subject.Subject != "" {
		parts = append(parts, part{text: assembled.Subject.Subject, slot: 1})
	}
	for _, modifier := range assembled.Modifiers {
		parts = append(parts, part{text: modifier.Text, slot: modifier.SlotPosition})
	}
	if assembled.Artist != nil && assembled.Artist.Name != "" {
		parts = append(parts, part{text: "by " + assembled.Artist.Name, slot: 9})
	}
	if assembled.Platform != nil && assembled.Platform.Phrase != "" {
		parts = append(parts, part{text: assembled.Platform.Phrase, slot: 10})
	}
	slices.SortFunc(parts, func(a, b part) int {
		if a.slot != b.slot {
			return a.slot - b.slot
		}
		return strings.Compare(a.text, b.text)
	})
	texts := make([]string, 0, len(parts))
	for _, p := range parts {
		texts = append(texts, p.text)
	}
	return strings.Join(texts, ", ")
}

func componentStats(lib *ComponentLibrary) map[string]int {
	stats := map[string]int{
		"subjects":  len(lib.Subjects),
		"modifiers": len(lib.Modifiers),
		"artists":   len(lib.Artists),
		"platforms": len(lib.Platforms),
	}
	for _, modifier := range lib.Modifiers {
		stats["category:"+modifier.Category]++
	}
	return stats
}
