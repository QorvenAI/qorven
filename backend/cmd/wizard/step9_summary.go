// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package wizard

import (
	"strings"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// ── Step 9 — Summary & Finalize ───────────────────────────────────────────────

type finalizeResultMsg struct{ err error }

type step9Summary struct {
	client  *Client
	state   *State
	spinner spinner.Model
	busy    bool
	done    bool
	errMsg  string
}

func newStep9Summary(client *Client, state *State) *step9Summary {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(cPrimary)

	return &step9Summary{client: client, state: state, spinner: sp}
}

func (s *step9Summary) Init() tea.Cmd {
	return s.spinner.Tick
}

func (s *step9Summary) finalizeCmd() tea.Cmd {
	state := s.state
	client := s.client
	return func() tea.Msg {
		req := FinalizeReq{
			InstanceName: state.WorkspaceName,
			PrimeName:    state.PrimeName,
			PrimeIcon:    state.PrimeIcon,
			Style:        state.PrimeStyle,
			Language:     state.PrimeLanguage,
			TLSMode:      state.AccessMode,
			TLSDomain:    state.TLSDomain,
			WebPort:      state.WebPort,
		}
		err := client.Finalize(req)
		return finalizeResultMsg{err: err}
	}
}

func (s *step9Summary) Update(msg tea.Msg) (stepModel, tea.Cmd) {
	switch m := msg.(type) {

	case finalizeResultMsg:
		s.busy = false
		if m.err != nil {
			s.errMsg = m.err.Error()
			return s, nil
		}
		s.done = true
		return s, Next()

	case tea.KeyMsg:
		if s.busy {
			var cmd tea.Cmd
			s.spinner, cmd = s.spinner.Update(msg)
			return s, cmd
		}
		switch m.String() {
		case "enter", "f":
			if !s.done {
				s.busy = true
				s.errMsg = ""
				return s, tea.Batch(s.finalizeCmd(), s.spinner.Tick)
			}
		}
	}

	if s.busy {
		var cmd tea.Cmd
		s.spinner, cmd = s.spinner.Update(msg)
		return s, cmd
	}
	return s, nil
}

func (s *step9Summary) View() string {
	primeName := s.state.PrimeName
	if primeName == "" {
		primeName = "Prime"
	}
	primeIcon := s.state.PrimeIcon
	if primeIcon == "" {
		primeIcon = "✨"
	}
	workspace := s.state.WorkspaceName
	if workspace == "" {
		workspace = "My Workspace"
	}

	// Big "meet your assistant" hero block
	heroName := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Render(primeName)
	heroIcon := lipgloss.NewStyle().Render(primeIcon)
	heroStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#A78BFA")).Render(s.state.PrimeStyle)

	intro := ""
	if s.state.DisplayName != "" {
		intro = lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF")).
			Render("Hi "+s.state.DisplayName+"! ") + "\n"
	}
	intro += "I'm " + heroIcon + " " + heroName + lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF")).Render(" — ready to work for you.")
	if heroStyle != "" {
		intro += "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280")).Render("Style: ") + heroStyle
	}

	heroBanner := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#7C3AED")).
		Padding(1, 3).
		Render(intro)

	// Summary rows
	row := func(k, v string) string {
		if v == "" {
			return ""
		}
		return mutedStyle.Render(pad(k, 18)) +
			lipgloss.NewStyle().Foreground(cFgNormal).Render(v) + "\n"
	}

	accessLabel := "Direct IP (HTTP)"
	switch s.state.AccessMode {
	case "auto":
		accessLabel = "HTTPS"
		if s.state.TLSDomain != "" {
			accessLabel += " (" + s.state.TLSDomain + ")"
		}
	case "reverse-proxy":
		accessLabel = "Tailscale"
	}

	providerNames := make([]string, len(s.state.AddedProviders))
	for i, p := range s.state.AddedProviders {
		providerNames[i] = p.DisplayName
	}
	providerStr := strings.Join(providerNames, ", ")
	if providerStr == "" {
		providerStr = "—  (add after install)"
	}

	channelStr := strings.Join(s.state.ConnectedChannels, ", ")
	if channelStr == "" {
		channelStr = "none"
	}

	details := row("Workspace", workspace) +
		row("Admin", "@"+s.state.Username) +
		row("Language", s.state.PrimeLanguage) +
		row("Providers", providerStr) +
		row("Channels", channelStr) +
		row("Access", accessLabel)

	detailCard := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(cBorder).
		Padding(1, 2).
		Render(details)

	var status string
	if s.busy {
		status = s.spinner.View() + " Launching " + primeIcon + " " + primeName + "…"
	} else if s.errMsg != "" {
		status = wError(s.errMsg) + "\n" + mutedStyle.Render("Press F or Enter to retry")
	} else {
		status = mutedStyle.Render("Press Enter to launch the dashboard and meet ") +
			lipgloss.NewStyle().Foreground(lipgloss.Color("#A78BFA")).Render(primeIcon+" "+primeName) +
			mutedStyle.Render(".")
	}

	return heroBanner + "\n\n" + detailCard + "\n\n" + status
}

func (s *step9Summary) NextDisabled() bool { return s.busy }
func (s *step9Summary) NextLabel() string  { return "Finish →" }

func pad(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}
