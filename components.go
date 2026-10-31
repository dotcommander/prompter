package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"sort"
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
	data := defaultComponentsJSON
	if trimmed := strings.TrimSpace(path); trimmed != "" {
		read, err := readBoundedFile(trimmed)
		if err == nil {
			data = read
		} else if !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("read components file %s: %w", trimmed, err)
		}
	}

	var lib ComponentLibrary
	if err := json.Unmarshal([]byte(data), &lib); err != nil {
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
		available := make(map[string]struct{})
		for _, modifier := range lib.Modifiers {
			available[modifier.Category] = struct{}{}
		}
		seen := make(map[string]struct{}, len(categories))
		for _, category := range categories {
			category = strings.TrimSpace(category)
			if category == "" {
				return nil, fmt.Errorf("categories must not contain empty values")
			}
			if _, ok := available[category]; !ok {
				return nil, fmt.Errorf("unknown category %q (valid: %s)", category, strings.Join(sortedCategoryNames(available), ", "))
			}
			if _, ok := seen[category]; ok {
				return nil, fmt.Errorf("duplicate category %q", category)
			}
			seen[category] = struct{}{}
			profile.Selections = append(profile.Selections, ModifierSelection{Category: category, Count: 1, MinWeight: 0.3})
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

func sortedCategoryNames(categories map[string]struct{}) []string {
	names := make([]string, 0, len(categories))
	for name := range categories {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
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
