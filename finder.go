package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sahilm/fuzzy"
)

// Search weights - higher = more important
const (
	weightName        = 1000
	weightAlias       = 500
	weightPath        = 300
	weightDescription = 100
	weightBody        = 10
)

// fuzzyScore returns the fuzzy match score for query against target, or -1 if no match.
func fuzzyScore(query, target string) int {
	if matches := fuzzy.Find(query, []string{strings.ToLower(target)}); len(matches) > 0 {
		return matches[0].Score
	}
	return -1
}

// weightedRank calculates a weighted score for how well a prompt matches a query.
// Higher score = better match. Returns -1 if no match.
func weightedRank(query string, entry PromptEntry) int {
	if query == "" {
		return 0
	}

	query = strings.ToLower(query)
	score := -1

	// Check name (highest weight)
	if s := fuzzyScore(query, entry.Name); s >= 0 {
		score = max(score, weightName+s)
	}

	// Check aliases
	for _, alias := range entry.Aliases {
		if s := fuzzyScore(query, alias); s >= 0 {
			score = max(score, weightAlias+s)
		}
	}

	// Check path (e.g., "code/arch" or "think")
	// Extract relative path components for matching
	pathParts := strings.Split(entry.Path, string(filepath.Separator))
	if len(pathParts) > 1 {
		// Get directory portion (e.g., "think" from ".../prompts.d/think/ultrathink.md")
		for i := len(pathParts) - 2; i >= 0 && i >= len(pathParts)-3; i-- {
			part := pathParts[i]
			if part != "prompts.d" && part != "" {
				if s := fuzzyScore(query, part); s >= 0 {
					score = max(score, weightPath+s)
				}
			}
		}
	}

	// Check description
	if s := fuzzyScore(query, entry.Description); s >= 0 {
		score = max(score, weightDescription+s)
	}

	// Check body (lowest weight, only first 500 chars for performance)
	body := entry.Content
	if len(body) > 500 {
		body = body[:500]
	}
	if s := fuzzyScore(query, body); s >= 0 {
		score = max(score, weightBody+s)
	}

	return score
}

type rankedEntry struct {
	entry PromptEntry
	score int
	index int
}

type finderModel struct {
	entries      []PromptEntry
	dirs         []string
	input        textinput.Model
	filtered     []rankedEntry
	cursor       int
	scrollOffset int
	height       int
	width        int
	selected     *PromptEntry
	canceled     bool
}

func newFinderModel(entries []PromptEntry, dirs []string) *finderModel {
	ti := textinput.New()
	ti.Prompt = "> "
	ti.Placeholder = "Type to search..."
	ti.Focus()

	m := &finderModel{
		entries: entries,
		dirs:    dirs,
		input:   ti,
	}
	m.filterEntries()
	return m
}

func (m *finderModel) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, tea.RequestWindowSize)
}

func (m *finderModel) filterEntries() {
	query := strings.TrimSpace(m.input.Value())
	if query == "" {
		m.filtered = make([]rankedEntry, len(m.entries))
		for i, entry := range m.entries {
			m.filtered[i] = rankedEntry{
				entry: entry,
				score: 0,
				index: i,
			}
		}
		return
	}

	var ranked []rankedEntry
	for i, entry := range m.entries {
		score := weightedRank(query, entry)
		if score >= 0 {
			ranked = append(ranked, rankedEntry{
				entry: entry,
				score: score,
				index: i,
			})
		}
	}

	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		return ranked[i].index < ranked[j].index
	})

	m.filtered = ranked
	if m.cursor >= len(m.filtered) {
		m.cursor = max(0, len(m.filtered)-1)
	}
}

func (m *finderModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.canceled = true
			return m, tea.Quit

		case "enter":
			if len(m.filtered) > 0 && m.cursor >= 0 && m.cursor < len(m.filtered) {
				entry := m.filtered[m.cursor].entry
				m.selected = &entry
			}
			return m, tea.Quit

		case "up", "ctrl+p", "ctrl+k":
			if len(m.filtered) > 0 {
				m.cursor--
				if m.cursor < 0 {
					m.cursor = len(m.filtered) - 1
				}
				m.adjustScroll()
			}
			return m, nil

		case "down", "ctrl+n", "ctrl+j":
			if len(m.filtered) > 0 {
				m.cursor++
				if m.cursor >= len(m.filtered) {
					m.cursor = 0
				}
				m.adjustScroll()
			}
			return m, nil

		case "pgup":
			if len(m.filtered) > 0 {
				m.cursor = max(0, m.cursor-5)
				m.adjustScroll()
			}
			return m, nil

		case "pgdown":
			if len(m.filtered) > 0 {
				m.cursor = min(len(m.filtered)-1, m.cursor+5)
				m.adjustScroll()
			}
			return m, nil
		}
	}

	prevVal := m.input.Value()
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if m.input.Value() != prevVal {
		m.filterEntries()
		m.cursor = 0
		m.scrollOffset = 0
	}
	return m, cmd
}

func (m *finderModel) maxVisible() int {
	maxVis := 10
	if m.height > 8 {
		maxVis = min(20, m.height-6)
	}
	return maxVis
}

func (m *finderModel) adjustScroll() {
	maxVis := m.maxVisible()
	if m.cursor < m.scrollOffset {
		m.scrollOffset = m.cursor
	}
	if m.cursor >= m.scrollOffset+maxVis {
		m.scrollOffset = m.cursor - maxVis + 1
	}
}

func (m *finderModel) View() tea.View {
	if m.canceled || m.selected != nil {
		v := tea.NewView("")
		v.AltScreen = true
		return v
	}

	sourceDesc := strings.Join(m.dirs, ", ")
	if sourceDesc == "" {
		sourceDesc = "~/.config/prompter/prompts.d"
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	descStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	itemStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	noMatchStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Italic(true)

	var sb strings.Builder
	sb.WriteString(titleStyle.Render(fmt.Sprintf("Search Prompts (%d available)", len(m.entries))))
	sb.WriteString("\n")
	sb.WriteString(descStyle.Render(fmt.Sprintf("Source: %s\nControls: Type to search  •  ↑/↓ navigate  •  Enter copy & print  •  Esc / Ctrl+C quit", sourceDesc)))
	sb.WriteString("\n\n")
	sb.WriteString(m.input.View())
	sb.WriteString("\n\n")

	if len(m.entries) == 0 {
		sb.WriteString(noMatchStyle.Render("  No prompt files found in: " + sourceDesc))
		sb.WriteString("\n\n")
		sb.WriteString(dimStyle.Render("  Tip: Add .md prompt files to that directory to browse, copy, and run them."))
		sb.WriteString("\n")
	} else if len(m.filtered) == 0 {
		sb.WriteString(noMatchStyle.Render("  No matching prompts"))
		sb.WriteString("\n")
	} else {
		m.adjustScroll()
		maxVis := m.maxVisible()
		end := min(len(m.filtered), m.scrollOffset+maxVis)

		for i := m.scrollOffset; i < end; i++ {
			entry := m.filtered[i].entry
			label := entry.Name
			if entry.Description != "" && entry.Description != entry.Name {
				label += dimStyle.Render("  •  " + entry.Description)
			}

			if i == m.cursor {
				sb.WriteString(selectedStyle.Render("❯ " + label))
			} else {
				sb.WriteString(itemStyle.Render("  " + label))
			}
			sb.WriteString("\n")
		}

		if len(m.filtered) > maxVis {
			sb.WriteString(descStyle.Render(fmt.Sprintf("  [%d/%d]", m.cursor+1, len(m.filtered))))
			sb.WriteString("\n")
		}
	}

	v := tea.NewView(sb.String())
	v.AltScreen = true
	v.Cursor = m.input.Cursor()
	return v
}

// RunFinder shows the fuzzy finder and returns the selected prompt.
// Returns nil if the user cancelled (Ctrl+C, Esc).
func RunFinder(entries []PromptEntry, dirs ...string) (*PromptEntry, error) {
	return runFinder(entries, dirs, func(m tea.Model) (tea.Model, error) {
		p := tea.NewProgram(m, tea.WithInput(os.Stdin), tea.WithOutput(os.Stderr))
		return p.Run()
	})
}

type finderRunner func(tea.Model) (tea.Model, error)

func runFinder(entries []PromptEntry, dirs []string, run finderRunner) (*PromptEntry, error) {
	if len(entries) == 0 {
		return nil, fmt.Errorf("no prompts found")
	}

	model := newFinderModel(entries, dirs)
	finalModel, err := run(model)
	if err != nil {
		return nil, fmt.Errorf("finder: %w", err)
	}

	fm, ok := finalModel.(*finderModel)
	if !ok {
		return nil, fmt.Errorf("finder: unexpected model type %T", finalModel)
	}

	if fm.canceled {
		return nil, nil
	}

	return fm.selected, nil
}
