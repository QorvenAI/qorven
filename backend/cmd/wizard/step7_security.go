// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package wizard

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// ── Step 7 — Security & Access ────────────────────────────────────────────────

// accessMode values map to backend tls_mode
// direct   → "disabled"
// domain   → "auto"
// tailscale → "reverse-proxy"

type step7Security struct {
	state       *State
	selIdx      int // 0=direct, 1=domain, 2=tailscale
	domainInput textinput.Model
	portInput   textinput.Model
	focusField  int // 0=option selector, 1=domain, 2=port
}

var accessOptions = []struct {
	id    string
	label string
	hint  string
}{
	{id: "direct", label: "Direct IP (no TLS)", hint: "Access via http://IP:PORT — no certificate required"},
	{id: "domain", label: "Custom Domain + HTTPS", hint: "Point a domain here and Let's Encrypt issues a cert automatically"},
	{id: "tailscale", label: "Tailscale (private network)", hint: "Zero-config private access over Tailscale — no port exposure needed"},
}

func newStep7Security(state *State) *step7Security {
	dom := textinput.New()
	dom.Placeholder = "qorven.example.com"
	dom.SetWidth(40)

	port := textinput.New()
	port.Placeholder = "4201"
	port.SetValue("4201")
	port.SetWidth(10)

	return &step7Security{
		state:       state,
		domainInput: dom,
		portInput:   port,
	}
}

func (s *step7Security) Init() tea.Cmd { return textinput.Blink }

func (s *step7Security) Update(msg tea.Msg) (stepModel, tea.Cmd) {
	switch m := msg.(type) {
	case tea.KeyMsg:
		switch m.String() {
		case "up", "k":
			if s.focusField == 0 && s.selIdx > 0 {
				s.selIdx--
				s.domainInput.Blur()
				s.portInput.Blur()
			}
		case "down", "j":
			if s.focusField == 0 && s.selIdx < len(accessOptions)-1 {
				s.selIdx++
				s.domainInput.Blur()
				s.portInput.Blur()
			}
		case "tab":
			s.domainInput.Blur()
			s.portInput.Blur()
			maxFields := s.numFields()
			s.focusField = (s.focusField + 1) % maxFields
			s.applyFocus()
			return s, textinput.Blink
		case "shift+tab":
			s.domainInput.Blur()
			s.portInput.Blur()
			maxFields := s.numFields()
			s.focusField = (s.focusField + maxFields - 1) % maxFields
			s.applyFocus()
			return s, textinput.Blink
		case "enter":
			s.save()
			return s, Next()
		}
	}

	var cmds []tea.Cmd
	if s.focusField == 1 && s.selIdx == 1 {
		var cmd tea.Cmd
		s.domainInput, cmd = s.domainInput.Update(msg)
		cmds = append(cmds, cmd)
	}
	if s.focusField == 2 || (s.focusField == 1 && s.selIdx == 0) {
		var cmd tea.Cmd
		s.portInput, cmd = s.portInput.Update(msg)
		cmds = append(cmds, cmd)
	}
	return s, tea.Batch(cmds...)
}

func (s *step7Security) numFields() int {
	switch s.selIdx {
	case 1: // domain: selector + domain input + port
		return 3
	case 0: // direct: selector + port
		return 2
	default: // tailscale: selector only
		return 1
	}
}

func (s *step7Security) applyFocus() {
	switch {
	case s.focusField == 1 && s.selIdx == 1:
		s.domainInput.Focus()
	case s.focusField == 1 && s.selIdx == 0:
		s.portInput.Focus()
	case s.focusField == 2 && s.selIdx == 1:
		s.portInput.Focus()
	}
}

func (s *step7Security) save() {
	opt := accessOptions[s.selIdx]
	switch opt.id {
	case "direct":
		s.state.AccessMode = "disabled"
	case "domain":
		s.state.AccessMode = "auto"
		s.state.TLSDomain = s.domainInput.Value()
	case "tailscale":
		s.state.AccessMode = "reverse-proxy"
	}
	if p := s.portInput.Value(); p != "" {
		s.state.WebPort = p
	}
}

func (s *step7Security) View() string {
	title := titleStyle.Render("Security & Access")
	sub := mutedStyle.Render("How will users reach Qorven? You can change this later in Settings.")

	var cards []string
	for i, opt := range accessOptions {
		isSelected := i == s.selIdx
		isFocused := isSelected && s.focusField == 0

		var border lipgloss.Border
		var card lipgloss.Style
		if isSelected {
			border = lipgloss.RoundedBorder()
			card = lipgloss.NewStyle().Border(border).BorderForeground(cPrimary).Padding(0, 1).Width(60)
		} else {
			border = lipgloss.NormalBorder()
			card = lipgloss.NewStyle().Border(border).BorderForeground(cBorder).Padding(0, 1).Width(60)
		}

		labelSt := mutedStyle
		if isSelected {
			labelSt = lipgloss.NewStyle().Foreground(cPrimary).Bold(true)
		}
		prefix := "  "
		if isFocused {
			prefix = "► "
		} else if isSelected {
			prefix = "● "
		}

		content := labelSt.Render(prefix+opt.label) + "\n" +
			mutedStyle.Render("  "+opt.hint)

		// Extra fields when this option is selected
		if isSelected {
			switch opt.id {
			case "direct":
				portFocused := s.focusField == 1
				portLbl := mutedStyle.Render("  Port")
				if portFocused {
					portLbl = lipgloss.NewStyle().Foreground(cPrimary).Bold(true).Render("  Port")
				}
				content += "\n\n" + warnSt.Render("  ⚠  HTTP only — no encryption") +
					"\n" + portLbl + "\n  " + s.portInput.View()

			case "domain":
				domFocused := s.focusField == 1
				portFocused := s.focusField == 2
				domLbl := mutedStyle.Render("  Domain")
				if domFocused {
					domLbl = lipgloss.NewStyle().Foreground(cPrimary).Bold(true).Render("  Domain")
				}
				portLbl := mutedStyle.Render("  Port")
				if portFocused {
					portLbl = lipgloss.NewStyle().Foreground(cPrimary).Bold(true).Render("  Port")
				}
				content += "\n\n" + domLbl + "\n  " + s.domainInput.View() +
					"\n" + portLbl + "\n  " + s.portInput.View() +
					"\n" + mutedStyle.Render("  Let's Encrypt certificate issued on first start")

			case "tailscale":
				content += "\n\n" + wSuccess("No public port exposure required")
			}
		}

		cards = append(cards, card.Render(content))
	}

	hint := mutedStyle.Render("↑ ↓ to select  •  Tab for fields  •  Enter / Continue to proceed")

	result := title + "\n" + sub + "\n\n"
	for _, c := range cards {
		result += c + "\n"
	}
	return result + "\n" + hint
}

func (s *step7Security) NextDisabled() bool { return false }
func (s *step7Security) NextLabel() string  { return "" }
