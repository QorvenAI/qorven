// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package wizard

import (
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// ── Step 1 — Admin account ────────────────────────────────────────────────────

type step1Admin struct {
	client  *Client
	state   *State
	inputs  [3]textinput.Model // fullname, username, password
	focused int
	spinner spinner.Model
	busy    bool
	done    bool
	errMsg  string
}

func newStep1Admin(client *Client, state *State) *step1Admin {
	fullname := textinput.New()
	fullname.Placeholder = "Your full name"
	fullname.Focus()
	fullname.SetWidth(36)

	username := textinput.New()
	username.Placeholder = "admin"
	username.SetWidth(36)

	password := textinput.New()
	password.Placeholder = "min 8 characters"
	password.EchoMode = textinput.EchoPassword
	password.EchoCharacter = '•'
	password.SetWidth(36)

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(cPrimary)

	return &step1Admin{
		client:  client,
		state:   state,
		inputs:  [3]textinput.Model{fullname, username, password},
		spinner: sp,
	}
}

// ── createAdminMsg ────────────────────────────────────────────────────────────

type createAdminResultMsg struct{ token string; err error }

func (s *step1Admin) createAdminCmd() tea.Cmd {
	fullname := s.inputs[0].Value()
	username := s.inputs[1].Value()
	password := s.inputs[2].Value()
	return func() tea.Msg {
		if err := s.client.CreateAdmin(username, password, fullname); err != nil {
			return createAdminResultMsg{err: err}
		}
		token, err := s.client.Login(username, password)
		return createAdminResultMsg{token: token, err: err}
	}
}

// ── stepModel interface ───────────────────────────────────────────────────────

func (s *step1Admin) Init() tea.Cmd {
	return textinput.Blink
}

func (s *step1Admin) Update(msg tea.Msg) (stepModel, tea.Cmd) {
	switch m := msg.(type) {

	case createAdminResultMsg:
		s.busy = false
		if m.err != nil {
			s.errMsg = m.err.Error()
			return s, nil
		}
		s.state.AuthToken = m.token
		s.state.Username = s.inputs[1].Value()
		s.state.Password = s.inputs[2].Value()
		s.state.DisplayName = s.inputs[0].Value()
		s.client.SetToken(m.token)
		s.done = true
		return s, Next()

	case tea.KeyMsg:
		if s.busy {
			return s, nil
		}
		switch m.String() {
		case "tab", "down":
			s.inputs[s.focused].Blur()
			s.focused = (s.focused + 1) % 3
			s.inputs[s.focused].Focus()
			return s, textinput.Blink
		case "shift+tab", "up":
			s.inputs[s.focused].Blur()
			s.focused = (s.focused + 2) % 3
			s.inputs[s.focused].Focus()
			return s, textinput.Blink
		case "enter":
			if s.focused < 2 {
				s.inputs[s.focused].Blur()
				s.focused++
				s.inputs[s.focused].Focus()
				return s, textinput.Blink
			}
			return s, s.submit()
		}
	}

	if s.busy {
		var cmd tea.Cmd
		s.spinner, cmd = s.spinner.Update(msg)
		return s, cmd
	}

	var cmd tea.Cmd
	s.inputs[s.focused], cmd = s.inputs[s.focused].Update(msg)
	return s, cmd
}

func (s *step1Admin) submit() tea.Cmd {
	if s.inputs[1].Value() == "" {
		s.errMsg = "Username is required"
		return nil
	}
	if len(s.inputs[2].Value()) < 8 {
		s.errMsg = "Password must be at least 8 characters"
		return nil
	}
	s.errMsg = ""
	s.busy = true
	return tea.Batch(s.createAdminCmd(), s.spinner.Tick)
}

func (s *step1Admin) View() string {
	if s.done {
		return wSuccess("Admin account created — signed in as @" + s.state.Username)
	}

	title := titleStyle.Render("Create admin account")
	sub := mutedStyle.Render("This is the login you'll use to sign in to Qorven.")

	labels := []string{"Full name", "Username", "Password"}
	var fields string
	for i, inp := range s.inputs {
		lbl := mutedStyle.Render(labels[i])
		if i == s.focused && !s.busy {
			lbl = lipgloss.NewStyle().Foreground(cPrimary).Bold(true).Render(labels[i])
		}
		fields += lbl + "\n" + inp.View() + "\n\n"
	}

	var status string
	if s.busy {
		status = s.spinner.View() + " Creating account…"
	} else if s.errMsg != "" {
		status = wError(s.errMsg)
	} else {
		status = mutedStyle.Render("Press Tab to move between fields, Enter to submit.")
	}

	return title + "\n" + sub + "\n\n" + fields + status
}

func (s *step1Admin) NextDisabled() bool { return s.busy }
func (s *step1Admin) NextLabel() string  { return "" }
