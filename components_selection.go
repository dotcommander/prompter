package main

import (
	"crypto/sha256"
	"encoding/binary"
	"slices"
	"strings"
)

type scoredChoice[T any] struct {
	value T
	name  string
	score float64
}

func scoreChoice[T any](value T, seed, query, scope, name string, weight float64) scoredChoice[T] {
	return scoredChoice[T]{value: value, name: name, score: stableChoiceScore(seed, query, scope, name) * max(weight, 0.01)}
}

func compareScoredChoice[T any](a, b scoredChoice[T]) int {
	switch {
	case a.score > b.score:
		return -1
	case a.score < b.score:
		return 1
	default:
		return strings.Compare(a.name, b.name)
	}
}

func (lib *ComponentLibrary) pickModifiers(selection ModifierSelection, subjectCategory, seed, query string) []PromptModifier {
	var candidates []scoredChoice[PromptModifier]
	for _, modifier := range lib.Modifiers {
		if modifier.Category != selection.Category || modifier.Weight < selection.MinWeight {
			continue
		}
		if len(modifier.Compatible) > 0 && subjectCategory != "" && !slices.Contains(modifier.Compatible, subjectCategory) {
			continue
		}
		candidates = append(candidates, scoreChoice(modifier, seed, query, selection.Category, modifier.Text, modifier.Weight))
	}
	if len(candidates) == 0 {
		return nil
	}
	slices.SortFunc(candidates, compareScoredChoice[PromptModifier])
	count := min(selection.Count, len(candidates))
	if count == 0 {
		return nil
	}
	result := make([]PromptModifier, count)
	for i := range result {
		result[i] = candidates[i].value
	}
	return result
}

// A unique best choice needs no sorting. An equal top key can represent distinct
// records; sort the scored entries then to preserve SortFunc's duplicate ordering.
func bestScoredChoice[T any](candidates []scoredChoice[T]) *T {
	if len(candidates) == 0 {
		return nil
	}
	best := 0
	tied := false
	for i := 1; i < len(candidates); i++ {
		switch compareScoredChoice(candidates[i], candidates[best]) {
		case -1:
			best = i
			tied = false
		case 0:
			tied = true
		}
	}
	if tied {
		slices.SortFunc(candidates, compareScoredChoice[T])
		best = 0
	}
	value := candidates[best].value
	return &value
}

func (lib *ComponentLibrary) pickArtist(subjectCategory, seed, query string) *PromptArtist {
	var candidates []scoredChoice[PromptArtist]
	for _, artist := range lib.Artists {
		if len(artist.Categories) == 0 || subjectCategory == "" || slices.Contains(artist.Categories, subjectCategory) {
			candidates = append(candidates, scoreChoice(artist, seed, query, "artist", artist.Name, artist.Weight))
		}
	}
	return bestScoredChoice(candidates)
}

func (lib *ComponentLibrary) pickPlatform(seed, query string) *PromptPlatform {
	var candidates []scoredChoice[PromptPlatform]
	for _, platform := range lib.Platforms {
		candidates = append(candidates, scoreChoice(platform, seed, query, "platform", platform.Name, platform.Weight))
	}
	return bestScoredChoice(candidates)
}

func stableChoiceScore(seed, query, scope, value string) float64 {
	h := sha256.Sum256([]byte(seed + "\x00" + query + "\x00" + scope + "\x00" + value))
	n := binary.BigEndian.Uint64(h[:8])
	return float64(n) / float64(^uint64(0))
}
