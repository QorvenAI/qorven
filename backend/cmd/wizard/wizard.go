// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package wizard

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// totalSteps matches the 9-step web wizard.
const totalSteps = 9

// stepModel is the interface every step must satisfy.
type stepModel interface {
	Init() tea.Cmd
	Update(tea.Msg) (stepModel, tea.Cmd)
	View() string
	// NextDisabled returns true when the Continue button should be greyed out.
	NextDisabled() bool
	// NextLabel returns a custom label for the Continue button, or "" for default.
	NextLabel() string
}

// ── Root model ────────────────────────────────────────────────────────────────

// Wizard is the root bubbletea.Model for the setup wizard.
type Wizard struct {
	client  *Client
	state   *State
	step    int // 1-indexed
	steps   []stepModel
	errMsg  string
	width   int
	height  int
	booting bool // true while /health + /setup-check are in-flight
}

// New builds the Wizard and all step sub-models.
func New(baseURL string) *Wizard {
	client := NewClient(baseURL, "")
	state := &State{BaseURL: baseURL, WebPort: "4201"}
	w := &Wizard{
		client:  client,
		state:   state,
		step:    1,
		booting: true,
	}
	w.steps = buildSteps(client, state)
	return w
}

func buildSteps(client *Client, state *State) []stepModel {
	return []stepModel{
		newStep1Admin(client, state),
		newStep2Workspace(state),
		newStep3Prime(state),
		newStep4Provider(client, state),
		newStep5Channels(client, state),
		newStep6Voice(client, state),
		newStep7Security(state),
		newStep8TestChat(client, state),
		newStep9Summary(client, state),
	}
}

// ── booting msgs ─────────────────────────────────────────────────────────────

type bootDoneMsg struct {
	setupRequired bool
	err           error
}

func (w *Wizard) bootCmd() tea.Cmd {
	return func() tea.Msg {
		if err := w.client.Health(); err != nil {
			return bootDoneMsg{err: fmt.Errorf("cannot reach Qorven at %s — is the server running?", w.client.base)}
		}
		r, err := w.client.SetupCheck()
		if err != nil {
			return bootDoneMsg{err: fmt.Errorf("setup-check failed: %w", err)}
		}
		return bootDoneMsg{setupRequired: r.SetupRequired}
	}
}

// ── tea.Model interface ───────────────────────────────────────────────────────

func (w *Wizard) Init() tea.Cmd {
	cmds := []tea.Cmd{w.bootCmd()}
	if active := w.active(); active != nil {
		if cmd := active.Init(); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return tea.Batch(cmds...)
}

func (w *Wizard) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {

	case tea.WindowSizeMsg:
		w.width = m.Width
		w.height = m.Height
		return w, nil

	case tea.KeyMsg:
		switch m.String() {
		case "ctrl+c", "esc":
			return w, tea.Quit
		}

	case bootDoneMsg:
		w.booting = false
		if m.err != nil {
			w.errMsg = m.err.Error()
			return w, nil
		}
		if !m.setupRequired {
			// Admin already exists — skip step 1 and fetch PrimeID if possible
			w.step = 2
		}
		// Re-init the now-active step
		if s := w.active(); s != nil {
			return w, s.Init()
		}
		return w, nil

	case NavMsg:
		w.errMsg = ""
		newStep := w.step + m.Dir
		if newStep < 1 {
			newStep = 1
		}
		if newStep > totalSteps {
			return w, tea.Quit
		}
		w.step = newStep
		if s := w.active(); s != nil {
			return w, s.Init()
		}
		return w, nil

	case ErrMsg:
		w.errMsg = m.Err.Error()
		return w, nil

	case ClearErrMsg:
		w.errMsg = ""
		return w, nil
	}

	// Delegate to active step
	if s := w.active(); s != nil {
		newStep, cmd := s.Update(msg)
		w.steps[w.step-1] = newStep
		return w, cmd
	}
	return w, nil
}

func (w *Wizard) View() tea.View {
	var body string

	if w.booting {
		body = cardStyle.Render(
			mutedStyle.Render("Connecting to Qorven…"),
		)
	} else if w.errMsg != "" && w.step == 1 {
		// Fatal boot error — no step indicator, just the error
		body = cardStyle.Render(wError(w.errMsg) + "\n\n" + mutedStyle.Render("Press Ctrl+C to exit."))
	} else {
		stepBody := ""
		if s := w.active(); s != nil {
			stepBody = s.View()
		}
		body = cardStyle.Render(stepBody)
	}

	var sections []string
	sections = append(sections, renderHeader())
	if !w.booting {
		sections = append(sections, renderStepIndicator(w.step))
	}
	if w.errMsg != "" && w.step != 1 {
		sections = append(sections, errorSt.Render("  "+w.errMsg))
	}
	sections = append(sections, body)
	if !w.booting && w.step < totalSteps {
		s := w.active()
		nextDisabled := s != nil && s.NextDisabled()
		nextLabel := ""
		if s != nil {
			nextLabel = s.NextLabel()
		}
		if w.step == totalSteps-1 {
			nextLabel = "Finish →"
		}
		sections = append(sections, renderNav(w.step, totalSteps, w.step == 1, nextDisabled, nextLabel))
	}

	return tea.View{
		AltScreen: true,
		Content: lipgloss.NewStyle().
			Padding(1, 2).
			Render(lipgloss.JoinVertical(lipgloss.Left, sections...)),
	}
}

func (w *Wizard) active() stepModel {
	if w.step < 1 || w.step > len(w.steps) {
		return nil
	}
	return w.steps[w.step-1]
}

// ── Launch ────────────────────────────────────────────────────────────────────

// Run starts the wizard TUI program in full-screen mode.
func Run(baseURL string) error {
	w := New(baseURL)
	p := tea.NewProgram(w)
	_, err := p.Run()
	return err
}

// RunInline starts the wizard without alt-screen (useful in CI / logs).
func RunInline(baseURL string) error {
	w := New(baseURL)
	p := tea.NewProgram(w)
	_, err := p.Run()
	return err
}
