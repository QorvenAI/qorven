// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package wizard

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// ── Step 3 — Personalise Prime (icon + personality + language) ────────────────

// primeIcons is the palette the user scrolls through to pick an avatar.
var primeIcons = []struct {
	emoji string
	label string
}{
	{"✨", "Spark"},
	{"🤖", "Robot"},
	{"🧠", "Brain"},
	{"⚡", "Flash"},
	{"🦊", "Fox"},
	{"🌊", "Wave"},
	{"🔥", "Fire"},
	{"🎯", "Focus"},
	{"🚀", "Rocket"},
	{"🦋", "Libre"},
	{"🌿", "Sage"},
	{"🏔", "Peak"},
}

var primePersonalities = []struct {
	label string
	desc  string
	value string
}{
	{"Professional", "Precise, structured, direct", "formal and structured"},
	{"Curious", "Asks questions, explores deeply", "curious and inquisitive"},
	{"Creative", "Lateral thinking, unexpected angles", "creative and imaginative"},
	{"Technical", "Code-first, details over summaries", "concise and code-focused"},
	{"Friendly", "Warm, patient, conversational", "friendly and conversational"},
}

var primeLanguages = []string{
	"English", "Spanish", "French", "German",
	"Hindi", "Chinese", "Japanese", "Korean",
	"Arabic", "Portuguese",
}

// sub-screens within step 3
const (
	primeScreenIcon        = iota // pick emoji
	primeScreenPersonality        // pick working style
	primeScreenLanguage           // pick language + custom role
)

type step3Prime struct {
	state       *State
	subScreen   int
	iconIdx     int
	personIdx   int
	langIdx     int
	customInput textinput.Model
	showCustom  bool
}

func newStep3Prime(state *State) *step3Prime {
	custom := textinput.New()
	custom.Placeholder = "Describe what " + primeName(state) + " should be good at…"
	custom.SetWidth(52)

	return &step3Prime{
		state:       state,
		customInput: custom,
		personIdx:   0, // Professional default
	}
}

func primeName(state *State) string {
	if state.PrimeName != "" {
		return state.PrimeName
	}
	return "your assistant"
}

func (s *step3Prime) Init() tea.Cmd { return textinput.Blink }

func (s *step3Prime) Update(msg tea.Msg) (stepModel, tea.Cmd) {
	switch m := msg.(type) {
	case tea.KeyMsg:
		switch s.subScreen {
		case primeScreenIcon:
			return s.updateIcon(m)
		case primeScreenPersonality:
			return s.updatePersonality(m)
		case primeScreenLanguage:
			return s.updateLanguage(m)
		}
	}
	if s.showCustom {
		var cmd tea.Cmd
		s.customInput, cmd = s.customInput.Update(msg)
		return s, cmd
	}
	return s, nil
}

func (s *step3Prime) updateIcon(m tea.KeyMsg) (stepModel, tea.Cmd) {
	cols := 4 // icons per row
	switch m.String() {
	case "left", "h":
		if s.iconIdx > 0 {
			s.iconIdx--
		}
	case "right", "l":
		if s.iconIdx < len(primeIcons)-1 {
			s.iconIdx++
		}
	case "up", "k":
		if s.iconIdx >= cols {
			s.iconIdx -= cols
		}
	case "down", "j":
		if s.iconIdx+cols < len(primeIcons) {
			s.iconIdx += cols
		}
	case "enter", " ":
		s.subScreen = primeScreenPersonality
	}
	return s, nil
}

func (s *step3Prime) updatePersonality(m tea.KeyMsg) (stepModel, tea.Cmd) {
	switch m.String() {
	case "up", "k":
		if s.personIdx > 0 {
			s.personIdx--
		}
	case "down", "j":
		if s.personIdx < len(primePersonalities)-1 {
			s.personIdx++
		}
	case "enter", " ":
		s.subScreen = primeScreenLanguage
		s.customInput.Placeholder = "Describe what " + primeName(s.state) + " should be good at…"
		s.customInput.Blur()
	}
	return s, nil
}

func (s *step3Prime) updateLanguage(m tea.KeyMsg) (stepModel, tea.Cmd) {
	switch m.String() {
	case "left", "h", "[":
		if s.langIdx > 0 {
			s.langIdx--
		}
	case "right", "l", "]":
		if s.langIdx < len(primeLanguages)-1 {
			s.langIdx++
		}
	case "tab":
		if !s.showCustom {
			s.showCustom = true
			s.customInput.Focus()
			return s, textinput.Blink
		}
		s.customInput.Blur()
		s.showCustom = false
	case "enter":
		if s.showCustom {
			s.customInput.Blur()
		}
		s.save()
		return s, Next()
	}
	if s.showCustom {
		var cmd tea.Cmd
		s.customInput, cmd = s.customInput.Update(m)
		return s, cmd
	}
	return s, nil
}

func (s *step3Prime) save() {
	s.state.PrimeIcon = primeIcons[s.iconIdx].emoji
	s.state.PrimeStyle = primePersonalities[s.personIdx].value
	s.state.PrimeLanguage = primeLanguages[s.langIdx]
	if s.showCustom && s.customInput.Value() != "" {
		s.state.PrimeRoleDesc = s.customInput.Value()
	}
}

func (s *step3Prime) View() string {
	name := primeName(s.state)

	switch s.subScreen {
	case primeScreenIcon:
		return s.viewIcon(name)
	case primeScreenPersonality:
		return s.viewPersonality(name)
	default:
		return s.viewLanguage(name)
	}
}

func (s *step3Prime) viewIcon(name string) string {
	title := titleStyle.Render("Pick " + name + "'s icon")
	sub := mutedStyle.Render("Arrow keys to move  •  Enter to choose")

	cols := 4
	var rows string
	for i, ic := range primeIcons {
		selected := i == s.iconIdx
		col := i % cols

		var cell string
		if selected {
			cell = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#FFFFFF")).
				Background(lipgloss.Color("#7C3AED")).
				Padding(0, 1).
				Render(ic.emoji + " " + ic.label)
		} else {
			cell = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#9CA3AF")).
				Padding(0, 1).
				Render(ic.emoji + " " + ic.label)
		}

		rows += lipgloss.NewStyle().Width(16).Render(cell)
		if col == cols-1 || i == len(primeIcons)-1 {
			rows += "\n"
		}
	}

	selected := primeIcons[s.iconIdx]
	preview := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#7C3AED")).
		Padding(0, 2).
		Render(selected.emoji + "  " +
			lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F9FAFB")).Render(name) +
			lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF")).Render("  "+selected.label))

	return title + "\n" + sub + "\n\n" + rows + "\n" + preview
}

func (s *step3Prime) viewPersonality(name string) string {
	icon := primeIcons[s.iconIdx].emoji
	title := titleStyle.Render(icon + "  How would you describe " + name + "'s style?")
	sub := mutedStyle.Render("↑ ↓ to pick  •  Enter to confirm")

	var items string
	for i, p := range primePersonalities {
		if i == s.personIdx {
			items += lipgloss.NewStyle().Foreground(lipgloss.Color("#7C3AED")).Bold(true).
				Render("▶  "+p.label) + "\n"
			items += lipgloss.NewStyle().Foreground(lipgloss.Color("#A78BFA")).
				Render("     "+p.desc) + "\n\n"
		} else {
			items += lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF")).
				Render("   "+p.label) + "\n"
			items += lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280")).
				Render("   "+p.desc) + "\n\n"
		}
	}

	return title + "\n" + sub + "\n\n" + items
}

func (s *step3Prime) viewLanguage(name string) string {
	icon := primeIcons[s.iconIdx].emoji
	title := titleStyle.Render(icon + "  Last thing — " + name + "'s language")
	sub := mutedStyle.Render("← → to change  •  Tab to add a custom role prompt  •  Enter to finish")

	langRow := lipgloss.NewStyle().Foreground(lipgloss.Color("#7C3AED")).Bold(true).
		Render("Language: ") +
		lipgloss.NewStyle().Foreground(lipgloss.Color("#F9FAFB")).Bold(true).
			Render(primeLanguages[s.langIdx])

	nav := ""
	if s.langIdx > 0 {
		nav += lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280")).
			Render("  ← "+primeLanguages[s.langIdx-1]+"  ")
	}
	nav += lipgloss.NewStyle().Foreground(lipgloss.Color("#7C3AED")).Bold(true).
		Render("["+primeLanguages[s.langIdx]+"]")
	if s.langIdx < len(primeLanguages)-1 {
		nav += lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280")).
			Render("  "+primeLanguages[s.langIdx+1]+" →")
	}

	var customSection string
	if s.showCustom {
		customSection = "\n\n" +
			lipgloss.NewStyle().Foreground(lipgloss.Color("#7C3AED")).Bold(true).
				Render("Custom role (optional)") + "\n" +
			s.customInput.View()
	} else {
		customSection = "\n\n" +
			lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280")).
				Render("Tab to add a custom role description (optional)")
	}

	_ = langRow
	return title + "\n" + sub + "\n\n" + nav + customSection
}

func (s *step3Prime) NextDisabled() bool { return false }
func (s *step3Prime) NextLabel() string  { return "" }
