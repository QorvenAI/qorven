// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package wizard

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// ── Step 8 — Test Chat ────────────────────────────────────────────────────────

type chatMsg struct {
	role    string // "user" | "assistant"
	content string
}

type chatChunkMsg struct{ chunk string }
type chatDoneMsg struct{ err error }

type step8TestChat struct {
	client   *Client
	state    *State
	input    textinput.Model
	viewport viewport.Model
	spinner  spinner.Model
	msgs     []chatMsg
	pending  string // streaming assistant message in progress
	sending  bool
	passed   bool
	errMsg   string
	ready    bool // viewport initialised
}

func newStep8TestChat(client *Client, state *State) *step8TestChat {
	inp := textinput.New()
	inp.Placeholder = "Say something to Prime…"
	inp.SetWidth(60)
	inp.Focus()

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(cPrimary)

	vp := viewport.New(viewport.WithWidth(70), viewport.WithHeight(12))

	return &step8TestChat{
		client:   client,
		state:    state,
		input:    inp,
		viewport: vp,
		spinner:  sp,
	}
}

func (s *step8TestChat) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, s.spinner.Tick)
}

// sendCmdSimple uses a non-streaming POST and returns the full response.
func (s *step8TestChat) sendCmdSimple(text string) tea.Cmd {
	primeID := s.state.PrimeID
	token := s.state.AuthToken
	base := s.state.BaseURL
	client := s.client
	return func() tea.Msg {
		if primeID == "" {
			// Try to resolve prime
			agents, err := client.ListAgents()
			if err != nil || len(agents) == 0 {
				return chatDoneMsg{err: fmt.Errorf("prime agent not found — go back and add a provider first")}
			}
			for _, a := range agents {
				if a.AgentKey == "chief" || a.AgentKey == "prime" {
					primeID = a.ID
					break
				}
			}
			if primeID == "" {
				return chatDoneMsg{err: fmt.Errorf("prime agent not found")}
			}
		}
		url := strings.TrimRight(base, "/") + "/v1/agents/" + primeID + "/chat"
		body := fmt.Sprintf(`{"message":%q}`, text)
		req, err := http.NewRequest("POST", url, strings.NewReader(body))
		if err != nil {
			return chatDoneMsg{err: err}
		}
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}

		cl := &http.Client{Timeout: 60 * time.Second}
		resp, err := cl.Do(req)
		if err != nil {
			return chatDoneMsg{err: err}
		}
		defer resp.Body.Close()
		data, _ := io.ReadAll(resp.Body)
		if resp.StatusCode >= 400 {
			return chatDoneMsg{err: fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))}
		}
		// Extract "reply" or "content" from JSON
		raw := string(data)
		for _, key := range []string{`"reply":"`, `"content":"`, `"message":"`} {
			if idx := strings.Index(raw, key); idx >= 0 {
				rest := raw[idx+len(key):]
				end := strings.Index(rest, `"`)
				if end >= 0 {
					reply := rest[:end]
					reply = strings.ReplaceAll(reply, `\n`, "\n")
					return chatChunkMsg{chunk: reply}
				}
			}
		}
		return chatDoneMsg{}
	}
}

func (s *step8TestChat) Update(msg tea.Msg) (stepModel, tea.Cmd) {
	var cmds []tea.Cmd

	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		w := m.Width - 6
		if w < 30 {
			w = 30
		}
		s.viewport.SetWidth(w)
		s.viewport.SetHeight(12)
		s.ready = true

	case chatChunkMsg:
		s.pending += m.chunk
		s.updateViewport()

	case chatDoneMsg:
		s.sending = false
		if m.err != nil {
			s.errMsg = m.err.Error()
			s.msgs = append(s.msgs, chatMsg{role: "assistant", content: "Error: " + m.err.Error()})
		} else {
			if s.pending != "" {
				s.msgs = append(s.msgs, chatMsg{role: "assistant", content: s.pending})
			}
			s.passed = true
			s.state.TestChatPassed = true
		}
		s.pending = ""
		s.updateViewport()
		return s, nil

	case tea.KeyMsg:
		switch m.String() {
		case "ctrl+n":
			return s, Next()
		case "enter":
			if !s.sending {
				text := strings.TrimSpace(s.input.Value())
				if text != "" {
					s.msgs = append(s.msgs, chatMsg{role: "user", content: text})
					s.input.SetValue("")
					s.sending = true
					s.errMsg = ""
					s.pending = ""
					s.updateViewport()
					cmds = append(cmds, s.sendCmdSimple(text), s.spinner.Tick)
					return s, tea.Batch(cmds...)
				}
			}
		}
	}

	if s.sending {
		var cmd tea.Cmd
		s.spinner, cmd = s.spinner.Update(msg)
		cmds = append(cmds, cmd)
	}

	var cmd tea.Cmd
	s.input, cmd = s.input.Update(msg)
	cmds = append(cmds, cmd)

	s.viewport, cmd = s.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return s, tea.Batch(cmds...)
}

func (s *step8TestChat) updateViewport() {
	var lines []string
	for _, m := range s.msgs {
		if m.role == "user" {
			lines = append(lines, lipgloss.NewStyle().Foreground(cPrimary).Render("You: ")+m.content)
		} else {
			lines = append(lines, lipgloss.NewStyle().Foreground(cEmerald).Render("Prime: ")+m.content)
		}
		lines = append(lines, "")
	}
	if s.pending != "" {
		lines = append(lines, lipgloss.NewStyle().Foreground(cEmerald).Render("Prime: ")+s.pending+"▌")
	}
	s.viewport.SetContent(strings.Join(lines, "\n"))
	s.viewport.GotoBottom()
}

func (s *step8TestChat) View() string {
	title := titleStyle.Render("Test chat with Prime")
	sub := mutedStyle.Render("Send a message to verify your LLM provider is working.")

	chatBox := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(cBorder).
		Padding(0, 1).
		Render(s.viewport.View())

	var status string
	if s.sending {
		status = s.spinner.View() + " Prime is thinking…"
	} else if s.passed {
		status = wSuccess("Chat is working! Press Continue to proceed.")
	} else if s.errMsg != "" {
		status = wError(s.errMsg)
	} else {
		status = mutedStyle.Render("Type a message and press Enter")
	}

	hint := mutedStyle.Render("Enter = send  •  Ctrl+N / Continue = skip/proceed")

	return title + "\n" + sub + "\n\n" +
		chatBox + "\n" +
		s.input.View() + "\n\n" +
		status + "\n\n" +
		hint
}

func (s *step8TestChat) NextDisabled() bool { return false }
func (s *step8TestChat) NextLabel() string  { return "" }
