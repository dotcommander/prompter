package main

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// Keep the former sorting comparator here as the parity oracle, including its
// weight floor, score tie-break and unstable ordering of duplicate records.
func referenceChoice(seed, query, scope, a, b string, weightA, weightB float64) int {
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

func referenceModifiers(lib *ComponentLibrary, selection ModifierSelection, category, seed, query string) []PromptModifier {
	var candidates []PromptModifier
	for _, m := range lib.Modifiers {
		if m.Category != selection.Category || m.Weight < selection.MinWeight ||
			(len(m.Compatible) > 0 && category != "" && !slices.Contains(m.Compatible, category)) {
			continue
		}
		candidates = append(candidates, m)
	}
	if len(candidates) == 0 {
		return nil
	}
	slices.SortFunc(candidates, func(a, b PromptModifier) int {
		return referenceChoice(seed, query, selection.Category, a.Text, b.Text, a.Weight, b.Weight)
	})
	return append([]PromptModifier(nil), candidates[:min(selection.Count, len(candidates))]...)
}

func referenceArtist(lib *ComponentLibrary, category, seed, query string) *PromptArtist {
	var candidates []PromptArtist
	for _, a := range lib.Artists {
		if len(a.Categories) == 0 || category == "" || slices.Contains(a.Categories, category) {
			candidates = append(candidates, a)
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	slices.SortFunc(candidates, func(a, b PromptArtist) int {
		return referenceChoice(seed, query, "artist", a.Name, b.Name, a.Weight, b.Weight)
	})
	return &candidates[0]
}

func referencePlatform(lib *ComponentLibrary, seed, query string) *PromptPlatform {
	if len(lib.Platforms) == 0 {
		return nil
	}
	candidates := append([]PromptPlatform(nil), lib.Platforms...)
	slices.SortFunc(candidates, func(a, b PromptPlatform) int {
		return referenceChoice(seed, query, "platform", a.Name, b.Name, a.Weight, b.Weight)
	})
	return &candidates[0]
}

func TestSelectionMatchesSortingReference(t *testing.T) {
	t.Parallel()
	lib := &ComponentLibrary{}
	for i := range 48 {
		category := []string{"quality", "lighting", "style"}[i%3]
		compatible := []string(nil)
		if i%4 == 0 {
			compatible = []string{"portrait"}
		}
		lib.Modifiers = append(lib.Modifiers, PromptModifier{
			Text: fmt.Sprintf("modifier-%02d", i%31), Category: category,
			SlotPosition: i%10 + 1, Weight: float64(i%9) / 10,
			Compatible: compatible,
		})
	}
	for i := range 32 {
		categories := []string(nil)
		if i%3 == 0 {
			categories = []string{"landscape"}
		}
		lib.Artists = append(lib.Artists, PromptArtist{
			Name: fmt.Sprintf("artist-%02d", i%23), Weight: float64(i%7) / 5,
			Categories: categories, StyleTags: []string{fmt.Sprintf("tag-%d", i)},
		})
	}
	for i := range 24 {
		lib.Platforms = append(lib.Platforms, PromptPlatform{
			Name: fmt.Sprintf("platform-%02d", i%17), Phrase: fmt.Sprintf("phrase-%d", i),
			Weight: float64(i%6) / 4,
		})
	}
	// Identical scoring keys with distinct metadata exercise unstable-sort parity.
	lib.Modifiers = append(lib.Modifiers, PromptModifier{Text: "same", Category: "quality", Weight: 100, SlotPosition: 2}, PromptModifier{Text: "same", Category: "quality", Weight: 100, SlotPosition: 3})
	lib.Artists = append(lib.Artists, PromptArtist{Name: "same", Weight: 100, StyleTags: []string{"one"}}, PromptArtist{Name: "same", Weight: 100, StyleTags: []string{"two"}})
	lib.Platforms = append(lib.Platforms, PromptPlatform{Name: "same", Weight: 100, Phrase: "one"}, PromptPlatform{Name: "same", Weight: 100, Phrase: "two"})
	before := fmt.Sprintf("%#v", lib)

	for i := range 16 {
		seed, query := fmt.Sprintf("seed-%d", i), fmt.Sprintf("subject-%d", i%5)
		for _, category := range []string{"", "portrait", "landscape"} {
			for _, selection := range []ModifierSelection{
				{Category: "quality", Count: 0, MinWeight: 0},
				{Category: "quality", Count: 4, MinWeight: 0.3},
				{Category: "lighting", Count: 50, MinWeight: 0.5},
				{Category: "style", Count: 2, MinWeight: 0.9},
			} {
				got, want := lib.pickModifiers(selection, category, seed, query), referenceModifiers(lib, selection, category, seed, query)
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("modifiers seed=%s category=%s selection=%+v: got %+v, want %+v", seed, category, selection, got, want)
				}
			}
			if got, want := lib.pickArtist(category, seed, query), referenceArtist(lib, category, seed, query); !reflect.DeepEqual(got, want) {
				t.Fatalf("artist seed=%s category=%s: got %+v, want %+v", seed, category, got, want)
			}
		}
		if got, want := lib.pickPlatform(seed, query), referencePlatform(lib, seed, query); !reflect.DeepEqual(got, want) {
			t.Fatalf("platform seed=%s: got %+v, want %+v", seed, got, want)
		}
	}
	if after := fmt.Sprintf("%#v", lib); after != before {
		t.Fatal("selection mutated the component library")
	}
}

func TestSelectionDuplicateTies(t *testing.T) {
	t.Parallel()
	lib := &ComponentLibrary{}
	for i := range 20 {
		lib.Modifiers = append(lib.Modifiers, PromptModifier{Text: "same", Category: "quality", Weight: 1, SlotPosition: i})
		lib.Artists = append(lib.Artists, PromptArtist{Name: "same", Weight: 1, StyleTags: []string{fmt.Sprint(i)}})
		lib.Platforms = append(lib.Platforms, PromptPlatform{Name: "same", Weight: 1, Phrase: fmt.Sprint(i)})
	}
	for i := range 4 {
		seed := fmt.Sprint(i)
		selection := ModifierSelection{Category: "quality", Count: 20}
		if got, want := lib.pickModifiers(selection, "", seed, "subject"), referenceModifiers(lib, selection, "", seed, "subject"); !reflect.DeepEqual(got, want) {
			t.Fatalf("duplicate modifiers: got %+v, want %+v", got, want)
		}
		if got, want := lib.pickArtist("", seed, "subject"), referenceArtist(lib, "", seed, "subject"); !reflect.DeepEqual(got, want) {
			t.Fatalf("duplicate artist: got %+v, want %+v", got, want)
		}
		if got, want := lib.pickPlatform(seed, "subject"), referencePlatform(lib, seed, "subject"); !reflect.DeepEqual(got, want) {
			t.Fatalf("duplicate platform: got %+v, want %+v", got, want)
		}
	}
}

func TestSelectionEmptyLibraryMatchesReference(t *testing.T) {
	t.Parallel()
	lib := &ComponentLibrary{}
	selection := ModifierSelection{Category: "quality", Count: 1}
	if got, want := lib.pickModifiers(selection, "", "seed", "query"), referenceModifiers(lib, selection, "", "seed", "query"); !reflect.DeepEqual(got, want) {
		t.Fatalf("modifiers: got %+v, want %+v", got, want)
	}
	if got, want := lib.pickArtist("", "seed", "query"), referenceArtist(lib, "", "seed", "query"); !reflect.DeepEqual(got, want) {
		t.Fatalf("artist: got %+v, want %+v", got, want)
	}
	if got, want := lib.pickPlatform("seed", "query"), referencePlatform(lib, "seed", "query"); !reflect.DeepEqual(got, want) {
		t.Fatalf("platform: got %+v, want %+v", got, want)
	}
}
