// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package wizard

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// ── Step 2 — Name your assistant (replaces dry workspace-name prompt) ─────────
//
// This screen is the user's first "meeting" with their agent.
// It asks for a name in Prime's voice, then stores WorkspaceName too.

type step2Workspace struct {
	state     *State
	nameInput textinput.Model
	wsInput   textinput.Model
	focused   int // 0=agent name, 1=workspace name
}

func newStep2Workspace(state *State) *step2Workspace {
	nameInp := textinput.New()
	nameInp.Placeholder = "Nova"
	nameInp.SetWidth(32)
	nameInp.Focus()

	wsInp := textinput.New()
	wsInp.Placeholder = "My Workspace"
	wsInp.SetValue("My Workspace")
	wsInp.SetWidth(32)

	return &step2Workspace{state: state, nameInput: nameInp, wsInput: wsInp}
}

func (s *step2Workspace) Init() tea.Cmd { return textinput.Blink }

func (s *step2Workspace) Update(msg tea.Msg) (stepModel, tea.Cmd) {
	switch m := msg.(type) {
	case tea.KeyMsg:
		switch m.String() {
		case "tab", "down":
			s.nameInput.Blur()
			s.wsInput.Blur()
			s.focused = (s.focused + 1) % 2
			if s.focused == 0 {
				s.nameInput.Focus()
			} else {
				s.wsInput.Focus()
			}
			return s, textinput.Blink
		case "shift+tab", "up":
			s.nameInput.Blur()
			s.wsInput.Blur()
			s.focused = (s.focused + 1) % 2
			if s.focused == 0 {
				s.nameInput.Focus()
			} else {
				s.wsInput.Focus()
			}
			return s, textinput.Blink
		case "enter":
			if s.focused == 0 {
				s.nameInput.Blur()
				s.wsInput.Focus()
				s.focused = 1
				return s, textinput.Blink
			}
			s.save()
			return s, Next()
		}
	}
	var cmd tea.Cmd
	if s.focused == 0 {
		s.nameInput, cmd = s.nameInput.Update(msg)
	} else {
		s.wsInput, cmd = s.wsInput.Update(msg)
	}
	return s, cmd
}

func (s *step2Workspace) save() {
	name := s.nameInput.Value()
	if name == "" {
		name = "Nova"
	}
	s.state.PrimeName = name

	ws := s.wsInput.Value()
	if ws == "" {
		ws = "My Workspace"
	}
	s.state.WorkspaceName = ws
}

func (s *step2Workspace) View() string {
	greeting := ""
	if s.state.DisplayName != "" {
		greeting = "Hi " + s.state.DisplayName + "! "
	}

	title := titleStyle.Render("Meet your assistant")
	sub := mutedStyle.Render(greeting + "I'm your new AI agent. What would you like to call me?")

	nameLbl := mutedStyle.Render("Assistant name")
	if s.focused == 0 {
		nameLbl = lipgloss.NewStyle().Foreground(cPrimary).Bold(true).Render("Assistant name")
	}

	wsLbl := mutedStyle.Render("Workspace name")
	if s.focused == 1 {
		wsLbl = lipgloss.NewStyle().Foreground(cPrimary).Bold(true).Render("Workspace name")
	}

	hint := mutedStyle.Render("Tab to switch  •  Enter to continue")

	return title + "\n" + sub + "\n\n" +
		nameLbl + "\n" + s.nameInput.View() + "\n\n" +
		wsLbl + "\n" + s.wsInput.View() + "\n\n" +
		hint
}

func (s *step2Workspace) NextDisabled() bool { return false }
func (s *step2Workspace) NextLabel() string  { return "" }
