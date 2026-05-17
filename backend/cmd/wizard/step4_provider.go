// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package wizard

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// ── Step 4 — LLM provider ─────────────────────────────────────────────────────

// Fallback catalog shown while fetching or if /v1/providers/catalog fails.
var fallbackProviders = []catalogEntry{
	{ID: "openai",     Name: "OpenAI",          AuthType: "api_key",         DefaultAPIBase: "https://api.openai.com/v1"},
	{ID: "anthropic",  Name: "Anthropic",        AuthType: "api_key",         DefaultAPIBase: "https://api.anthropic.com/v1"},
	{ID: "gemini",     Name: "Gemini",           AuthType: "api_key",         DefaultAPIBase: "https://generativelanguage.googleapis.com/v1beta/openai"},
	{ID: "deepseek",   Name: "DeepSeek",         AuthType: "api_key",         DefaultAPIBase: "https://api.deepseek.com/v1"},
	{ID: "groq",       Name: "Groq",             AuthType: "api_key",         DefaultAPIBase: "https://api.groq.com/openai/v1"},
	{ID: "mistral",    Name: "Mistral",          AuthType: "api_key",         DefaultAPIBase: "https://api.mistral.ai/v1"},
	{ID: "xai",        Name: "xAI (Grok)",       AuthType: "api_key",         DefaultAPIBase: "https://api.x.ai/v1"},
	{ID: "ollama",     Name: "Ollama (local)",   AuthType: "none",            DefaultAPIBase: "http://localhost:11434/v1"},
	{ID: "bedrock",    Name: "AWS Bedrock",      AuthType: "aws_credentials", DefaultAPIBase: ""},
	{ID: "openrouter", Name: "OpenRouter",       AuthType: "api_key",         DefaultAPIBase: "https://openrouter.ai/api/v1"},
	{ID: "together",   Name: "Together AI",      AuthType: "api_key",         DefaultAPIBase: "https://api.together.xyz/v1"},
	{ID: "cohere",     Name: "Cohere",           AuthType: "api_key",         DefaultAPIBase: "https://api.cohere.ai/compatibility/v1"},
}

type catalogEntry struct {
	ID             string
	Name           string
	AuthType       string // api_key | aws_credentials | none
	DefaultAPIBase string
	DefaultModel   string
	Models         []string
}

// Field focus in the credential form
const (
	focusCatalog = iota
	focusAPIKey
	focusAPIBase
	focusAWSAccess
	focusAWSSecret
	focusRegion
)

type step4Provider struct {
	client  *Client
	state   *State
	catalog []catalogEntry
	selIdx  int // selected provider index in catalog
	// credential inputs
	apiKeyInput    textinput.Model
	apiBaseInput   textinput.Model
	awsAccessInput textinput.Model
	awsSecretInput textinput.Model
	regionInput    textinput.Model
	focused        int
	// test state
	spinner  spinner.Model
	testing  bool
	tested   bool
	testErr  string
	testSample string
	models  []string
	// primary model selection
	modelIdx int
	// added providers
	added []AddedProvider
	// catalog fetch state
	catalogLoaded bool
}

type catalogLoadedMsg struct{ entries []catalogEntry }
type providerTestResultMsg struct {
	providerDBID string
	models       []string
	sample       string
	err          error
}

func newStep4Provider(client *Client, state *State) *step4Provider {
	apiKey := textinput.New()
	apiKey.Placeholder = "paste your API key"
	apiKey.EchoMode = textinput.EchoPassword
	apiKey.EchoCharacter = '•'
	apiKey.SetWidth(44)

	apiBase := textinput.New()
	apiBase.Placeholder = "https://api.example.com/v1"
	apiBase.SetWidth(44)

	awsAccess := textinput.New()
	awsAccess.Placeholder = "AKIAIOSFODNN7EXAMPLE"
	awsAccess.SetWidth(44)

	awsSecret := textinput.New()
	awsSecret.Placeholder = "wJalrXUtnFEMI/K7MDENG/..."
	awsSecret.EchoMode = textinput.EchoPassword
	awsSecret.EchoCharacter = '•'
	awsSecret.SetWidth(44)

	region := textinput.New()
	region.Placeholder = "us-east-1"
	region.SetValue("us-east-1")
	region.SetWidth(20)

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(cPrimary)

	return &step4Provider{
		client:         client,
		state:          state,
		catalog:        fallbackProviders,
		apiKeyInput:    apiKey,
		apiBaseInput:   apiBase,
		awsAccessInput: awsAccess,
		awsSecretInput: awsSecret,
		regionInput:    region,
		spinner:        sp,
	}
}

func (s *step4Provider) Init() tea.Cmd {
	return tea.Batch(
		s.loadCatalogCmd(),
		textinput.Blink,
		s.spinner.Tick,
	)
}

func (s *step4Provider) loadCatalogCmd() tea.Cmd {
	return func() tea.Msg {
		entries, err := s.client.ProviderCatalog()
		if err != nil {
			return catalogLoadedMsg{entries: nil}
		}
		var out []catalogEntry
		skipCategory := map[string]bool{"search": true, "voice": true, "data": true, "embeddings": true, "media": true}
		for _, m := range entries {
			if skipCategory[m.Category] {
				continue
			}
			out = append(out, catalogEntry{
				ID: m.ID, Name: m.Name, AuthType: m.AuthType,
				DefaultAPIBase: m.DefaultAPIBase, DefaultModel: m.DefaultModel,
				Models: m.Models,
			})
		}
		return catalogLoadedMsg{entries: out}
	}
}

func (s *step4Provider) testProviderCmd() tea.Cmd {
	entry := s.catalog[s.selIdx]
	req := ProviderTestReq{
		Name:         entry.ID,
		ProviderType: providerType(entry.ID, entry.AuthType),
		APIBase:      s.apiBaseInput.Value(),
		APIKey:       s.apiKeyInput.Value(),
		Region:       s.regionInput.Value(),
		AWSAccessKey: s.awsAccessInput.Value(),
		AWSSecretKey: s.awsSecretInput.Value(),
	}
	if req.APIBase == "" {
		req.APIBase = entry.DefaultAPIBase
	}
	return func() tea.Msg {
		res, err := s.client.TestProvider(req)
		if err != nil {
			return providerTestResultMsg{err: err}
		}
		// Try to find existing or create
		existing, _ := s.client.ListProviders()
		var dbID string
		for _, p := range existing {
			if p.Name == entry.ID {
				dbID = p.ID
				break
			}
		}
		if dbID == "" {
			created, cerr := s.client.CreateProvider(CreateProviderReq{
				Name: entry.ID, DisplayName: entry.Name,
				ProviderType: req.ProviderType,
				APIBase:      req.APIBase, APIKey:  req.APIKey,
				AWSAccessKey: req.AWSAccessKey, AWSSecretKey: req.AWSSecretKey,
				Region:  req.Region, Enabled: true,
			})
			if cerr != nil {
				return providerTestResultMsg{err: cerr}
			}
			dbID = created.ID
		}
		models := res.Models
		if len(models) == 0 && dbID != "" {
			mm, _ := s.client.ProviderModels(dbID)
			models = mm
		}
		return providerTestResultMsg{
			providerDBID: dbID,
			models:       models,
			sample:       res.Sample,
		}
	}
}

func (s *step4Provider) Update(msg tea.Msg) (stepModel, tea.Cmd) {
	switch m := msg.(type) {

	case catalogLoadedMsg:
		if len(m.entries) > 0 {
			s.catalog = m.entries
		}
		s.catalogLoaded = true
		s.resetInputs()
		return s, nil

	case providerTestResultMsg:
		s.testing = false
		if m.err != nil {
			s.testErr = m.err.Error()
			return s, nil
		}
		entry := s.catalog[s.selIdx]
		s.testErr = ""
		s.tested = true
		s.testSample = m.sample
		s.models = m.models
		s.modelIdx = 0
		s.state.ProviderDBID = m.providerDBID
		// Add to added list if not already there
		already := false
		for _, a := range s.added {
			if a.ID == entry.ID {
				already = true
				break
			}
		}
		if !already {
			s.added = append(s.added, AddedProvider{
				ID: entry.ID, DisplayName: entry.Name,
				ProviderDBID: m.providerDBID,
			})
			s.state.AddedProviders = s.added
		}
		if len(s.models) > 0 {
			s.state.PrimaryModel = s.models[0]
		}
		return s, nil

	case tea.KeyMsg:
		if s.testing {
			return s, nil
		}
		switch m.String() {
		case "left", "h":
			if s.focused == focusCatalog && s.selIdx > 0 {
				s.selIdx--
				s.resetInputs()
				s.tested = false
				s.testErr = ""
			}
		case "right", "l":
			if s.focused == focusCatalog && s.selIdx < len(s.catalog)-1 {
				s.selIdx++
				s.resetInputs()
				s.tested = false
				s.testErr = ""
			}
		case "tab", "down":
			s.blurAll()
			s.focused = s.nextFocusable()
			s.focusActive()
			return s, textinput.Blink
		case "shift+tab", "up":
			s.blurAll()
			s.focused = s.prevFocusable()
			s.focusActive()
			return s, textinput.Blink
		case "enter", " ":
			if s.focused == focusCatalog {
				s.tested = false
				s.testErr = ""
				s.resetInputs()
			}
		case "t":
			// "t" shortcut to test from anywhere
			return s, s.startTest()
		case "ctrl+n":
			if s.tested {
				return s, Next()
			}
		case "[":
			if s.tested && s.modelIdx > 0 {
				s.modelIdx--
				if s.modelIdx < len(s.models) {
					s.state.PrimaryModel = s.models[s.modelIdx]
				}
			}
		case "]":
			if s.tested && s.modelIdx < len(s.models)-1 {
				s.modelIdx++
				s.state.PrimaryModel = s.models[s.modelIdx]
			}
		}
	}

	if s.testing {
		var cmd tea.Cmd
		s.spinner, cmd = s.spinner.Update(msg)
		return s, cmd
	}

	var cmds []tea.Cmd
	switch s.focused {
	case focusAPIKey:
		var cmd tea.Cmd
		s.apiKeyInput, cmd = s.apiKeyInput.Update(msg)
		cmds = append(cmds, cmd)
	case focusAPIBase:
		var cmd tea.Cmd
		s.apiBaseInput, cmd = s.apiBaseInput.Update(msg)
		cmds = append(cmds, cmd)
	case focusAWSAccess:
		var cmd tea.Cmd
		s.awsAccessInput, cmd = s.awsAccessInput.Update(msg)
		cmds = append(cmds, cmd)
	case focusAWSSecret:
		var cmd tea.Cmd
		s.awsSecretInput, cmd = s.awsSecretInput.Update(msg)
		cmds = append(cmds, cmd)
	case focusRegion:
		var cmd tea.Cmd
		s.regionInput, cmd = s.regionInput.Update(msg)
		cmds = append(cmds, cmd)
	}
	return s, tea.Batch(cmds...)
}

func (s *step4Provider) startTest() tea.Cmd {
	s.testing = true
	s.testErr = ""
	s.tested = false
	return tea.Batch(s.testProviderCmd(), s.spinner.Tick)
}

func (s *step4Provider) resetInputs() {
	if s.selIdx >= len(s.catalog) {
		return
	}
	entry := s.catalog[s.selIdx]
	s.apiBaseInput.SetValue(entry.DefaultAPIBase)
	s.apiKeyInput.SetValue("")
	s.awsAccessInput.SetValue("")
	s.awsSecretInput.SetValue("")
	s.regionInput.SetValue("us-east-1")
}

func (s *step4Provider) blurAll() {
	s.apiKeyInput.Blur()
	s.apiBaseInput.Blur()
	s.awsAccessInput.Blur()
	s.awsSecretInput.Blur()
	s.regionInput.Blur()
}

func (s *step4Provider) focusActive() {
	switch s.focused {
	case focusAPIKey:
		s.apiKeyInput.Focus()
	case focusAPIBase:
		s.apiBaseInput.Focus()
	case focusAWSAccess:
		s.awsAccessInput.Focus()
	case focusAWSSecret:
		s.awsSecretInput.Focus()
	case focusRegion:
		s.regionInput.Focus()
	}
}

func (s *step4Provider) nextFocusable() int {
	entry := s.catalog[s.selIdx]
	fields := s.visibleFields(entry)
	for i, f := range fields {
		if f == s.focused {
			return fields[(i+1)%len(fields)]
		}
	}
	if len(fields) > 0 {
		return fields[0]
	}
	return focusCatalog
}

func (s *step4Provider) prevFocusable() int {
	entry := s.catalog[s.selIdx]
	fields := s.visibleFields(entry)
	for i, f := range fields {
		if f == s.focused {
			return fields[(i+len(fields)-1)%len(fields)]
		}
	}
	if len(fields) > 0 {
		return fields[len(fields)-1]
	}
	return focusCatalog
}

func (s *step4Provider) visibleFields(entry catalogEntry) []int {
	fields := []int{focusCatalog}
	switch entry.AuthType {
	case "aws_credentials":
		fields = append(fields, focusRegion, focusAWSAccess, focusAWSSecret)
	case "api_key":
		fields = append(fields, focusAPIKey)
		if entry.ID == "ollama" || entry.ID == "custom" || entry.DefaultAPIBase == "" {
			fields = append(fields, focusAPIBase)
		}
	case "none":
		fields = append(fields, focusAPIBase)
	}
	return fields
}

func (s *step4Provider) View() string {
	title := titleStyle.Render("Connect an LLM provider")
	sub := mutedStyle.Render("Pick a provider, enter credentials, and press 't' to test.")

	// Added providers summary
	addedSummary := ""
	if len(s.added) > 0 {
		names := make([]string, len(s.added))
		for i, a := range s.added {
			names[i] = a.DisplayName
		}
		addedSummary = "\n" + wSuccess("Added: "+strings.Join(names, ", "))
	}

	// Provider grid (3 per row)
	const cols = 3
	entry := s.catalog[s.selIdx]
	var rows []string
	row := ""
	for i, p := range s.catalog {
		isSelected := i == s.selIdx
		isAdded := false
		for _, a := range s.added {
			if a.ID == p.ID {
				isAdded = true
				break
			}
		}
		var cell string
		if isAdded {
			cell = successSt.Render("✓ " + fmt.Sprintf("%-12s", p.Name))
		} else if isSelected {
			cell = lipgloss.NewStyle().Foreground(cPrimary).Bold(true).Render("► " + fmt.Sprintf("%-12s", p.Name))
		} else {
			cell = mutedStyle.Render("  " + fmt.Sprintf("%-12s", p.Name))
		}
		row += cell + "  "
		if (i+1)%cols == 0 || i == len(s.catalog)-1 {
			rows = append(rows, row)
			row = ""
		}
	}
	grid := strings.Join(rows, "\n")

	// Credential fields
	var fields string
	switch entry.AuthType {
	case "aws_credentials":
		fields = s.renderField("Region", s.regionInput, focusRegion) + "\n" +
			s.renderField("AWS Access Key ID", s.awsAccessInput, focusAWSAccess) + "\n" +
			s.renderField("AWS Secret Access Key", s.awsSecretInput, focusAWSSecret)
	case "api_key":
		fields = s.renderField("API Key", s.apiKeyInput, focusAPIKey)
		if entry.ID == "ollama" || entry.DefaultAPIBase == "" {
			fields += "\n" + s.renderField("API Base URL", s.apiBaseInput, focusAPIBase)
		}
	case "none":
		fields = s.renderField("API Base URL", s.apiBaseInput, focusAPIBase)
	}

	// Test button / status
	var testLine string
	if s.testing {
		testLine = s.spinner.View() + " Testing " + entry.Name + "…"
	} else if s.testErr != "" {
		testLine = wError(s.testErr) + "\n" + mutedStyle.Render("Press 't' to retry")
	} else if s.tested {
		testLine = wSuccess(entry.Name+" connected")
		if s.testSample != "" {
			testLine += "\n" + mutedStyle.Render(`"`+s.testSample+`"`)
		}
		// Model picker
		if len(s.models) > 0 {
			testLine += "\n\n" + mutedStyle.Render("Primary model") + "\n"
			testLine += lipgloss.NewStyle().Foreground(cPrimary).Render("  "+s.models[s.modelIdx]) +
				mutedStyle.Render("  [ ] to pick")
		}
		testLine += "\n\n" + mutedStyle.Render("Press Ctrl+N or Continue to proceed")
	} else {
		testLine = mutedStyle.Render("Press 't' to test the connection")
	}

	catHint := mutedStyle.Render("← → to browse  Tab for fields")

	return title + "\n" + sub + addedSummary + "\n\n" +
		grid + "\n\n" +
		fields + "\n" +
		lipgloss.NewStyle().Border(lipgloss.NormalBorder(), true, false, false, false).
			BorderForeground(cBorder).Render(testLine) + "\n\n" +
		catHint
}

func (s *step4Provider) renderField(label string, inp textinput.Model, focus int) string {
	lbl := mutedStyle.Render(label)
	if s.focused == focus {
		lbl = lipgloss.NewStyle().Foreground(cPrimary).Bold(true).Render(label)
	}
	return lbl + "\n" + inp.View()
}

func (s *step4Provider) NextDisabled() bool { return !s.tested && len(s.added) == 0 }
func (s *step4Provider) NextLabel() string  { return "" }

// providerType maps catalog id + auth_type to the internal provider_type string.
func providerType(id, authType string) string {
	switch id {
	case "anthropic":
		return "anthropic_native"
	case "gemini":
		return "gemini_native"
	case "bedrock":
		return "bedrock"
	case "dashscope":
		return "dashscope"
	}
	if authType == "aws_credentials" {
		return "bedrock"
	}
	return "openai_compat"
}
