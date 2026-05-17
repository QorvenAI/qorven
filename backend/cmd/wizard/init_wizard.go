// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package wizard

import (
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// ── Init wizard — DB bootstrap (no running server required) ──────────────────
//
// RunInit launches a 2-screen TUI that collects:
//   Screen 1: PostgreSQL connection fields
//   Screen 2: LLM provider + API key
//
// It returns an InitResult with the collected values so the caller
// can perform the actual DB operations and file writes.

// InitResult is returned by RunInit after the user completes the form.
type InitResult struct {
	// Database
	DBHost     string
	DBPort     string
	DBName     string
	DBUser     string
	DBPassword string
	DBSSLMode  string

	// Provider
	Provider string
	APIKey   string
	APIBase  string

	Cancelled bool
}

// ── Init model ────────────────────────────────────────────────────────────────

const (
	initScreenDB = iota
	initScreenProvider
)

var initProviders = []struct {
	id   string
	name string
	base string
}{
	{id: "deepseek", name: "DeepSeek  (recommended — fast, cheap)", base: "https://api.deepseek.com/v1"},
	{id: "openai", name: "OpenAI", base: "https://api.openai.com/v1"},
	{id: "gemini", name: "Gemini", base: "https://generativelanguage.googleapis.com/v1beta/openai"},
	{id: "anthropic", name: "Anthropic", base: "https://api.anthropic.com/v1"},
	{id: "groq", name: "Groq", base: "https://api.groq.com/openai/v1"},
	{id: "ollama", name: "Ollama (local — no key needed)", base: "http://localhost:11434/v1"},
	{id: "custom", name: "Custom (OpenAI-compatible)", base: ""},
}

type initModel struct {
	screen int

	// Screen 1 — DB fields
	dbInputs  [6]textinput.Model // host, port, dbname, user, password, sslmode
	dbFocused int

	// Screen 2 — Provider
	provIdx    int
	apiKeyInp  textinput.Model
	apiBaseInp textinput.Model
	provFocused int // 0=list, 1=apikey, 2=apibase

	spinner  spinner.Model
	result   *InitResult
	quitting bool
}

type initDoneMsg struct{ result InitResult }

func newInitModel() *initModel {
	labels := []struct{ placeholder, def string }{
		{"localhost", "localhost"},
		{"5432", "5432"},
		{"qorven", "qorven"},
		{"postgres", "postgres"},
		{"(leave empty if none)", ""},
		{"disable", "disable"},
	}
	var inputs [6]textinput.Model
	for i, l := range labels {
		inp := textinput.New()
		inp.Placeholder = l.placeholder
		inp.SetValue(l.def)
		inp.SetWidth(36)
		if i == 4 { // password
			inp.EchoMode = textinput.EchoPassword
			inp.EchoCharacter = '•'
			inp.SetValue("")
		}
		inputs[i] = inp
	}
	inputs[0].Focus()

	apiKey := textinput.New()
	apiKey.Placeholder = "paste API key here"
	apiKey.EchoMode = textinput.EchoPassword
	apiKey.EchoCharacter = '•'
	apiKey.SetWidth(44)

	apiBase := textinput.New()
	apiBase.Placeholder = "https://..."
	apiBase.SetWidth(44)

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(cPrimary)

	return &initModel{
		dbInputs:   inputs,
		apiKeyInp:  apiKey,
		apiBaseInp: apiBase,
		spinner:    sp,
	}
}

func (m *initModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m *initModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case initDoneMsg:
		r := msg.result
		m.result = &r
		return m, tea.Quit

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.quitting = true
			return m, tea.Quit
		}

		if m.screen == initScreenDB {
			return m.updateDB(msg)
		}
		return m.updateProvider(msg)
	}

	var cmds []tea.Cmd
	if m.screen == initScreenDB {
		var cmd tea.Cmd
		m.dbInputs[m.dbFocused], cmd = m.dbInputs[m.dbFocused].Update(msg)
		cmds = append(cmds, cmd)
	} else {
		switch m.provFocused {
		case 1:
			var cmd tea.Cmd
			m.apiKeyInp, cmd = m.apiKeyInp.Update(msg)
			cmds = append(cmds, cmd)
		case 2:
			var cmd tea.Cmd
			m.apiBaseInp, cmd = m.apiBaseInp.Update(msg)
			cmds = append(cmds, cmd)
		}
	}
	return m, tea.Batch(cmds...)
}

func (m *initModel) updateDB(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "tab", "down":
		m.dbInputs[m.dbFocused].Blur()
		m.dbFocused = (m.dbFocused + 1) % len(m.dbInputs)
		m.dbInputs[m.dbFocused].Focus()
		return m, textinput.Blink
	case "shift+tab", "up":
		m.dbInputs[m.dbFocused].Blur()
		m.dbFocused = (m.dbFocused + len(m.dbInputs) - 1) % len(m.dbInputs)
		m.dbInputs[m.dbFocused].Focus()
		return m, textinput.Blink
	case "enter":
		if m.dbFocused < len(m.dbInputs)-1 {
			m.dbInputs[m.dbFocused].Blur()
			m.dbFocused++
			m.dbInputs[m.dbFocused].Focus()
			return m, textinput.Blink
		}
		// Last field — advance to screen 2
		m.screen = initScreenProvider
		m.provFocused = 0
		return m, textinput.Blink
	}
	var cmd tea.Cmd
	m.dbInputs[m.dbFocused], cmd = m.dbInputs[m.dbFocused].Update(msg)
	return m, cmd
}

func (m *initModel) updateProvider(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.provFocused == 0 && m.provIdx > 0 {
			m.provIdx--
			m.syncProviderBase()
		}
	case "down", "j":
		if m.provFocused == 0 && m.provIdx < len(initProviders)-1 {
			m.provIdx++
			m.syncProviderBase()
		}
	case "tab":
		m.apiKeyInp.Blur()
		m.apiBaseInp.Blur()
		m.provFocused = (m.provFocused + 1) % m.numProvFields()
		m.applyProvFocus()
		return m, textinput.Blink
	case "shift+tab":
		m.apiKeyInp.Blur()
		m.apiBaseInp.Blur()
		n := m.numProvFields()
		m.provFocused = (m.provFocused + n - 1) % n
		m.applyProvFocus()
		return m, textinput.Blink
	case "enter":
		if m.provFocused < m.numProvFields()-1 {
			m.apiKeyInp.Blur()
			m.apiBaseInp.Blur()
			m.provFocused++
			m.applyProvFocus()
			return m, textinput.Blink
		}
		// Finish
		return m, m.finishCmd()
	}

	var cmds []tea.Cmd
	switch m.provFocused {
	case 1:
		var cmd tea.Cmd
		m.apiKeyInp, cmd = m.apiKeyInp.Update(msg)
		cmds = append(cmds, cmd)
	case 2:
		var cmd tea.Cmd
		m.apiBaseInp, cmd = m.apiBaseInp.Update(msg)
		cmds = append(cmds, cmd)
	}
	return m, tea.Batch(cmds...)
}

func (m *initModel) syncProviderBase() {
	p := initProviders[m.provIdx]
	if p.id != "custom" {
		m.apiBaseInp.SetValue(p.base)
	}
}

func (m *initModel) numProvFields() int {
	p := initProviders[m.provIdx]
	n := 1 // list always
	if p.id != "ollama" {
		n++ // api key
	}
	if p.id == "ollama" || p.id == "custom" {
		n++ // api base
	}
	return n
}

func (m *initModel) applyProvFocus() {
	switch m.provFocused {
	case 1:
		p := initProviders[m.provIdx]
		if p.id != "ollama" {
			m.apiKeyInp.Focus()
		} else {
			m.apiBaseInp.Focus()
		}
	case 2:
		m.apiBaseInp.Focus()
	}
}

func (m *initModel) finishCmd() tea.Cmd {
	p := initProviders[m.provIdx]
	host := orDefault(m.dbInputs[0].Value(), "localhost")
	port := orDefault(m.dbInputs[1].Value(), "5432")
	dbname := orDefault(m.dbInputs[2].Value(), "qorven")
	user := orDefault(m.dbInputs[3].Value(), "postgres")
	password := m.dbInputs[4].Value()
	sslmode := orDefault(m.dbInputs[5].Value(), "disable")
	apiKey := m.apiKeyInp.Value()
	apiBase := m.apiBaseInp.Value()
	if apiBase == "" {
		apiBase = p.base
	}
	return func() tea.Msg {
		return initDoneMsg{result: InitResult{
			DBHost: host, DBPort: port, DBName: dbname,
			DBUser: user, DBPassword: password, DBSSLMode: sslmode,
			Provider: p.id, APIKey: apiKey, APIBase: apiBase,
		}}
	}
}

func (m *initModel) View() tea.View {
	if m.quitting {
		return tea.View{Content: ""}
	}
	header := renderHeader()
	var content string
	if m.screen == initScreenDB {
		content = m.viewDB(header)
	} else {
		content = m.viewProvider(header)
	}
	return tea.View{AltScreen: true, Content: content}
}

var dbLabels = []string{"Host", "Port", "Database", "Username", "Password", "SSL mode"}

func (m *initModel) viewDB(header string) string {
	title := titleStyle.Render("Database connection")
	sub := mutedStyle.Render("PostgreSQL credentials. Leave defaults if using local socket auth.")

	var fields string
	for i, inp := range m.dbInputs {
		lbl := mutedStyle.Render(dbLabels[i])
		if i == m.dbFocused {
			lbl = lipgloss.NewStyle().Foreground(cPrimary).Bold(true).Render(dbLabels[i])
		}
		fields += lbl + "\n" + inp.View() + "\n\n"
	}

	hint := mutedStyle.Render("Tab / ↓ to move  •  Enter to advance  •  Esc to cancel")
	body := cardStyle.Render(title + "\n" + sub + "\n\n" + fields + hint)
	return lipgloss.NewStyle().Padding(1, 2).Render(
		lipgloss.JoinVertical(lipgloss.Left, header, body),
	)
}

func (m *initModel) viewProvider(header string) string {
	title := titleStyle.Render("LLM Provider")
	sub := mutedStyle.Render("Which provider will power your agents?")

	var provList string
	for i, p := range initProviders {
		if i == m.provIdx {
			provList += lipgloss.NewStyle().Foreground(cPrimary).Bold(true).Render("► "+p.name) + "\n"
		} else {
			provList += mutedStyle.Render("  "+p.name) + "\n"
		}
	}

	p := initProviders[m.provIdx]
	var extraFields string

	if p.id != "ollama" {
		keyLbl := mutedStyle.Render("API Key")
		if m.provFocused == 1 {
			keyLbl = lipgloss.NewStyle().Foreground(cPrimary).Bold(true).Render("API Key")
		}
		extraFields += "\n" + keyLbl + "\n" + m.apiKeyInp.View() + "\n"
	}
	if p.id == "ollama" || p.id == "custom" {
		baseLbl := mutedStyle.Render("API Base URL")
		fieldIdx := 1
		if p.id == "custom" {
			fieldIdx = 2
		}
		if m.provFocused == fieldIdx {
			baseLbl = lipgloss.NewStyle().Foreground(cPrimary).Bold(true).Render("API Base URL")
		}
		extraFields += "\n" + baseLbl + "\n" + m.apiBaseInp.View() + "\n"
	}

	hint := mutedStyle.Render("↑ ↓ = pick provider  •  Tab = next field  •  Enter = proceed")

	body := cardStyle.Render(title + "\n" + sub + "\n\n" + provList + extraFields + "\n" + hint)
	return lipgloss.NewStyle().Padding(1, 2).Render(
		lipgloss.JoinVertical(lipgloss.Left, header, body),
	)
}

// ── Public entry points ───────────────────────────────────────────────────────

// RunInit launches the init TUI and blocks until the user completes or cancels.
// Returns the collected values so the caller can drive DB operations.
func RunInit() (InitResult, error) {
	m := newInitModel()
	p := tea.NewProgram(m)
	final, err := p.Run()
	if err != nil {
		return InitResult{}, err
	}
	im, ok := final.(*initModel)
	if !ok || im.quitting {
		return InitResult{Cancelled: true}, nil
	}
	if im.result != nil {
		return *im.result, nil
	}
	return InitResult{Cancelled: true}, nil
}

// RunInitInline is the same without alt-screen.
func RunInitInline() (InitResult, error) {
	m := newInitModel()
	p := tea.NewProgram(m)
	final, err := p.Run()
	if err != nil {
		return InitResult{}, err
	}
	im, ok := final.(*initModel)
	if !ok || im.quitting {
		return InitResult{Cancelled: true}, nil
	}
	if im.result != nil {
		return *im.result, nil
	}
	return InitResult{Cancelled: true}, nil
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
