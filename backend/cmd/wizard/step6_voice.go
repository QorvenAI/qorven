// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package wizard

import (
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// ── Step 6 — Voice ────────────────────────────────────────────────────────────

var ttsDrivers = []string{"none", "openai", "elevenlabs", "kokoro"}
var sttDrivers = []string{"none", "openai", "deepgram", "local"}

const (
	voiceFocusTTS = iota
	voiceFocusSTT
	voiceFocusTTSKey
	voiceFocusSTTKey
)

type voiceSaveResultMsg struct{ err error }

type step6Voice struct {
	client     *Client
	state      *State
	ttsIdx     int
	sttIdx     int
	ttsKeyInp  textinput.Model
	sttKeyInp  textinput.Model
	focused    int
	spinner    spinner.Model
	saving     bool
	saved      bool
	errMsg     string
}

func newStep6Voice(client *Client, state *State) *step6Voice {
	ttsKey := textinput.New()
	ttsKey.Placeholder = "TTS API key"
	ttsKey.EchoMode = textinput.EchoPassword
	ttsKey.EchoCharacter = '*'
	ttsKey.SetWidth(44)

	sttKey := textinput.New()
	sttKey.Placeholder = "STT API key"
	sttKey.EchoMode = textinput.EchoPassword
	sttKey.EchoCharacter = '*'
	sttKey.SetWidth(44)

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(cPrimary)

	return &step6Voice{
		client:    client,
		state:     state,
		ttsKeyInp: ttsKey,
		sttKeyInp: sttKey,
		spinner:   sp,
	}
}

func (s *step6Voice) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, s.spinner.Tick)
}

func (s *step6Voice) saveCmd() tea.Cmd {
	tts := ttsDrivers[s.ttsIdx]
	stt := sttDrivers[s.sttIdx]
	ttsKey := s.ttsKeyInp.Value()
	sttKey := s.sttKeyInp.Value()
	return func() tea.Msg {
		var errs []string
		if tts != "none" {
			if err := s.client.CreateVoiceProvider(VoiceProviderReq{
				Name: tts + "-tts", Kind: "tts", Driver: tts,
				APIKey: ttsKey, IsDefault: true, Enabled: true,
			}); err != nil {
				errs = append(errs, "TTS: "+err.Error())
			}
		}
		if stt != "none" {
			if err := s.client.CreateVoiceProvider(VoiceProviderReq{
				Name: stt + "-stt", Kind: "stt", Driver: stt,
				APIKey: sttKey, IsDefault: true, Enabled: true,
			}); err != nil {
				errs = append(errs, "STT: "+err.Error())
			}
		}
		if len(errs) > 0 {
			return voiceSaveResultMsg{err: combineErrors(errs)}
		}
		return voiceSaveResultMsg{}
	}
}

func (s *step6Voice) Update(msg tea.Msg) (stepModel, tea.Cmd) {
	switch m := msg.(type) {

	case voiceSaveResultMsg:
		s.saving = false
		if m.err != nil {
			s.errMsg = m.err.Error()
			return s, nil
		}
		s.saved = true
		s.state.TTSDriver = ttsDrivers[s.ttsIdx]
		s.state.STTDriver = sttDrivers[s.sttIdx]
		s.state.TTSKey = s.ttsKeyInp.Value()
		s.state.STTKey = s.sttKeyInp.Value()
		return s, Next()

	case tea.KeyMsg:
		if s.saving {
			var cmd tea.Cmd
			s.spinner, cmd = s.spinner.Update(msg)
			return s, cmd
		}
		switch m.String() {
		case "tab", "down":
			s.ttsKeyInp.Blur()
			s.sttKeyInp.Blur()
			s.focused = (s.focused + 1) % s.numFields()
			s.focusActive()
			return s, textinput.Blink
		case "shift+tab", "up":
			s.ttsKeyInp.Blur()
			s.sttKeyInp.Blur()
			s.focused = (s.focused + s.numFields() - 1) % s.numFields()
			s.focusActive()
			return s, textinput.Blink
		case "left", "h":
			switch s.focused {
			case voiceFocusTTS:
				if s.ttsIdx > 0 {
					s.ttsIdx--
				}
			case voiceFocusSTT:
				if s.sttIdx > 0 {
					s.sttIdx--
				}
			}
		case "right", "l":
			switch s.focused {
			case voiceFocusTTS:
				if s.ttsIdx < len(ttsDrivers)-1 {
					s.ttsIdx++
				}
			case voiceFocusSTT:
				if s.sttIdx < len(sttDrivers)-1 {
					s.sttIdx++
				}
			}
		case "enter":
			if s.focused == voiceFocusTTS || s.focused == voiceFocusSTT {
				// advance focus to key field if needed
				s.ttsKeyInp.Blur()
				s.sttKeyInp.Blur()
				s.focused = (s.focused + 1) % s.numFields()
				s.focusActive()
				return s, textinput.Blink
			}
			// Submit from key field
			return s, s.submit()
		case "ctrl+n":
			return s, s.submit()
		}
	}

	if s.saving {
		var cmd tea.Cmd
		s.spinner, cmd = s.spinner.Update(msg)
		return s, cmd
	}

	var cmds []tea.Cmd
	if s.focused == voiceFocusTTSKey {
		var cmd tea.Cmd
		s.ttsKeyInp, cmd = s.ttsKeyInp.Update(msg)
		cmds = append(cmds, cmd)
	}
	if s.focused == voiceFocusSTTKey {
		var cmd tea.Cmd
		s.sttKeyInp, cmd = s.sttKeyInp.Update(msg)
		cmds = append(cmds, cmd)
	}
	return s, tea.Batch(cmds...)
}

func (s *step6Voice) submit() tea.Cmd {
	tts := ttsDrivers[s.ttsIdx]
	stt := sttDrivers[s.sttIdx]
	// If both none, skip immediately
	if tts == "none" && stt == "none" {
		return Next()
	}
	s.saving = true
	s.errMsg = ""
	return tea.Batch(s.saveCmd(), s.spinner.Tick)
}

func (s *step6Voice) numFields() int {
	n := 2 // TTS selector + STT selector
	if ttsDrivers[s.ttsIdx] != "none" && ttsDrivers[s.ttsIdx] != "local" {
		n++
	}
	if sttDrivers[s.sttIdx] != "none" && sttDrivers[s.sttIdx] != "local" {
		n++
	}
	return n
}

func (s *step6Voice) focusActive() {
	// Recalculate which logical field maps to focused index
	switch s.focused {
	case voiceFocusTTS:
		// no text input
	case voiceFocusSTT:
		// no text input
	case voiceFocusTTSKey:
		if ttsDrivers[s.ttsIdx] != "none" {
			s.ttsKeyInp.Focus()
		}
	case voiceFocusSTTKey:
		if sttDrivers[s.sttIdx] != "none" {
			s.sttKeyInp.Focus()
		}
	}
}

func (s *step6Voice) View() string {
	title := titleStyle.Render("Voice (optional)")
	sub := mutedStyle.Render("Skip if you don't need text-to-speech or speech-to-text.")

	// TTS row
	ttsLabel := mutedStyle.Render("Text-to-Speech")
	if s.focused == voiceFocusTTS {
		ttsLabel = lipgloss.NewStyle().Foreground(cPrimary).Bold(true).Render("Text-to-Speech")
	}
	ttsRow := renderDriverPicker(ttsDrivers, s.ttsIdx, s.focused == voiceFocusTTS)

	// TTS key
	ttsKeyRow := ""
	if ttsDrivers[s.ttsIdx] != "none" && ttsDrivers[s.ttsIdx] != "local" {
		lbl := mutedStyle.Render("TTS API Key")
		if s.focused == voiceFocusTTSKey {
			lbl = lipgloss.NewStyle().Foreground(cPrimary).Bold(true).Render("TTS API Key")
		}
		ttsKeyRow = "\n" + lbl + "\n" + s.ttsKeyInp.View()
	}

	// STT row
	sttLabel := mutedStyle.Render("Speech-to-Text")
	if s.focused == voiceFocusSTT {
		sttLabel = lipgloss.NewStyle().Foreground(cPrimary).Bold(true).Render("Speech-to-Text")
	}
	sttRow := renderDriverPicker(sttDrivers, s.sttIdx, s.focused == voiceFocusSTT)

	// STT key
	sttKeyRow := ""
	if sttDrivers[s.sttIdx] != "none" && sttDrivers[s.sttIdx] != "local" {
		lbl := mutedStyle.Render("STT API Key")
		if s.focused == voiceFocusSTTKey {
			lbl = lipgloss.NewStyle().Foreground(cPrimary).Bold(true).Render("STT API Key")
		}
		sttKeyRow = "\n" + lbl + "\n" + s.sttKeyInp.View()
	}

	var status string
	if s.saving {
		status = s.spinner.View() + " saving…"
	} else if s.errMsg != "" {
		status = wError(s.errMsg)
	} else {
		status = mutedStyle.Render("Tab = move  •  ← → = pick driver  •  Enter / Continue = save & proceed")
	}

	_ = ttsLabel
	_ = sttLabel

	return title + "\n" + sub + "\n\n" +
		ttsLabel + "\n" + ttsRow + ttsKeyRow + "\n\n" +
		sttLabel + "\n" + sttRow + sttKeyRow + "\n\n" +
		status
}

func renderDriverPicker(drivers []string, selIdx int, focused bool) string {
	var items []string
	for i, d := range drivers {
		if i == selIdx {
			items = append(items, lipgloss.NewStyle().Foreground(cPrimary).Bold(true).Render("["+d+"]"))
		} else {
			items = append(items, mutedStyle.Render(d))
		}
	}
	hint := mutedStyle.Render("  ← →")
	if focused {
		hint = lipgloss.NewStyle().Foreground(cPrimary).Render("  ← →")
	}
	return lipgloss.JoinHorizontal(lipgloss.Center, append(items, hint)...)
}

func (s *step6Voice) NextDisabled() bool { return s.saving }
func (s *step6Voice) NextLabel() string  { return "" }

func combineErrors(errs []string) error {
	if len(errs) == 0 {
		return nil
	}
	msg := errs[0]
	for _, e := range errs[1:] {
		msg += "; " + e
	}
	return &simpleError{msg}
}

type simpleError struct{ msg string }

func (e *simpleError) Error() string { return e.msg }
