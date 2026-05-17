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

// ── Step 5 — Channels ─────────────────────────────────────────────────────────

type channelFieldDef struct {
	key    string
	label  string
	secret bool
	hint   string
}

type channelDef struct {
	id     string
	name   string
	fields []channelFieldDef
}

var channelDefs = []channelDef{
	{id: "telegram", name: "Telegram", fields: []channelFieldDef{
		{key: "bot_token", label: "Bot Token", secret: true, hint: "Get from @BotFather on Telegram"},
	}},
	{id: "discord", name: "Discord", fields: []channelFieldDef{
		{key: "bot_token", label: "Bot Token", secret: true, hint: "discord.com/developers → Bot → Reset Token"},
	}},
	{id: "slack", name: "Slack", fields: []channelFieldDef{
		{key: "bot_token", label: "Bot Token (xoxb-)", secret: true, hint: "OAuth & Permissions → Bot Token"},
		{key: "app_token", label: "App Token (xapp-)", secret: true, hint: "Basic Info → App-Level Tokens → connections:write"},
	}},
	{id: "whatsapp", name: "WhatsApp", fields: []channelFieldDef{
		{key: "bridge_url", label: "Bridge URL", hint: "http://localhost:3001 — run Baileys sidecar first"},
	}},
	{id: "email", name: "Email", fields: []channelFieldDef{
		{key: "email", label: "Email Address", hint: "e.g. agent@gmail.com"},
		{key: "password", label: "App Password", secret: true, hint: "myaccount.google.com/apppasswords"},
		{key: "imap_host", label: "IMAP Host", hint: "e.g. imap.gmail.com"},
		{key: "smtp_host", label: "SMTP Host", hint: "e.g. smtp.gmail.com"},
	}},
	{id: "sms", name: "SMS (Twilio)", fields: []channelFieldDef{
		{key: "from_number", label: "From Number", hint: "+14155552671"},
		{key: "api_key", label: "Account SID", secret: true},
		{key: "api_secret", label: "Auth Token", secret: true},
	}},
	{id: "teams", name: "Microsoft Teams", fields: []channelFieldDef{
		{key: "app_id", label: "App ID", hint: "Azure → App registrations → Application ID"},
		{key: "app_secret", label: "App Secret", secret: true},
		{key: "tenant_id", label: "Tenant ID"},
	}},
	{id: "github", name: "GitHub", fields: []channelFieldDef{
		{key: "app_id", label: "App ID", hint: "GitHub → Developer settings → GitHub Apps"},
		{key: "installation_id", label: "Installation ID"},
		{key: "private_key", label: "Private Key (PEM)", secret: true, hint: "Paste full -----BEGIN RSA PRIVATE KEY----- block"},
	}},
	{id: "webchat", name: "Webchat", fields: []channelFieldDef{
		{key: "allowed_domains", label: "Allowed Domains", hint: "comma-separated, or leave blank for any"},
	}},
	{id: "webhook", name: "Webhook", fields: []channelFieldDef{
		{key: "secret", label: "Webhook Secret (optional)", secret: true, hint: "leave blank to skip HMAC verification"},
	}},
	{id: "signal", name: "Signal", fields: []channelFieldDef{
		{key: "phone_number", label: "Phone Number", hint: "+15551234567 (E.164)"},
		{key: "socket_path", label: "signal-cli Socket Path", hint: "/run/user/1000/signal-cli/socket"},
	}},
	{id: "imessage", name: "iMessage", fields: []channelFieldDef{
		{key: "server_url", label: "BlueBubbles Server URL", hint: "https://yourname.ngrok.io"},
		{key: "password", label: "Server Password", secret: true},
	}},
	{id: "facebook", name: "Facebook Messenger", fields: []channelFieldDef{
		{key: "page_access_token", label: "Page Access Token", secret: true, hint: "Meta for Developers → Messenger → Settings"},
		{key: "verify_token", label: "Webhook Verify Token", hint: "any string you choose"},
	}},
	{id: "line", name: "LINE", fields: []channelFieldDef{
		{key: "channel_access_token", label: "Channel Access Token", secret: true, hint: "LINE Developers → Messaging API → Issue"},
		{key: "channel_secret", label: "Channel Secret", secret: true},
	}},
	{id: "zalo", name: "Zalo", fields: []channelFieldDef{
		{key: "app_id", label: "App ID"},
		{key: "app_secret", label: "App Secret", secret: true},
		{key: "refresh_token", label: "Refresh Token", secret: true, hint: "from OA authorization flow"},
		{key: "oa_id", label: "OA ID", hint: "OA Management → Info"},
	}},
	{id: "feishu", name: "Feishu / Lark", fields: []channelFieldDef{
		{key: "app_id", label: "App ID"},
		{key: "app_secret", label: "App Secret", secret: true},
	}},
	{id: "dingtalk", name: "DingTalk", fields: []channelFieldDef{
		{key: "client_id", label: "Client ID"},
		{key: "client_secret", label: "Client Secret", secret: true},
	}},
	{id: "wecom", name: "WeCom", fields: []channelFieldDef{
		{key: "corp_id", label: "Corp ID"},
		{key: "wecom_agent_id", label: "Agent ID (WeCom)"},
		{key: "app_secret", label: "App Secret", secret: true},
	}},
	{id: "matrix", name: "Matrix", fields: []channelFieldDef{
		{key: "homeserver_url", label: "Homeserver URL", hint: "https://matrix.org"},
		{key: "user_id", label: "User ID", hint: "@bot:matrix.org"},
		{key: "access_token", label: "Access Token", secret: true},
	}},
	{id: "mattermost", name: "Mattermost", fields: []channelFieldDef{
		{key: "server_url", label: "Server URL", hint: "https://your-mattermost.com"},
		{key: "bot_token", label: "Bot Token", secret: true},
	}},
}

type channelState struct {
	inputs      []textinput.Model
	activeField int
	connected   bool
	err         string
	busy        bool
}

type channelConnectResultMsg struct {
	idx int
	err error
}

type step5Channels struct {
	client  *Client
	state   *State
	selIdx  int
	ch      []channelState
	spinner spinner.Model
	primeID string
}

func newStep5Channels(client *Client, state *State) *step5Channels {
	ch := make([]channelState, len(channelDefs))
	for i, def := range channelDefs {
		inputs := make([]textinput.Model, len(def.fields))
		for j, f := range def.fields {
			inp := textinput.New()
			inp.Placeholder = "paste " + f.label + " here"
			if f.secret {
				inp.EchoMode = textinput.EchoPassword
				inp.EchoCharacter = '*'
			}
			inp.SetWidth(50)
			inputs[j] = inp
		}
		ch[i] = channelState{inputs: inputs}
	}

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(cPrimary)

	return &step5Channels{
		client:  client,
		state:   state,
		ch:      ch,
		spinner: sp,
	}
}

func (s *step5Channels) Init() tea.Cmd {
	s.ch[s.selIdx].inputs[0].Focus()
	return tea.Batch(textinput.Blink, s.spinner.Tick)
}

func (s *step5Channels) connectCmd(idx int) tea.Cmd {
	def := channelDefs[idx]
	config := make(map[string]string)
	for j, f := range def.fields {
		v := s.ch[idx].inputs[j].Value()
		if v != "" {
			config[f.key] = v
		}
	}
	primeID := s.resolvePrimeID()
	return func() tea.Msg {
		if primeID == "" {
			return channelConnectResultMsg{idx: idx, err: fmt.Errorf("prime agent not found — complete step 4 first")}
		}
		cfgAny := make(map[string]any, len(config))
		for k, v := range config {
			cfgAny[k] = v
		}
		err := s.client.CreateChannel(CreateChannelReq{
			AgentID:     primeID,
			ChannelType: def.id,
			Name:        def.name,
			Config:      cfgAny,
		})
		return channelConnectResultMsg{idx: idx, err: err}
	}
}

func (s *step5Channels) resolvePrimeID() string {
	if s.primeID != "" {
		return s.primeID
	}
	if s.state.PrimeID != "" {
		s.primeID = s.state.PrimeID
		return s.primeID
	}
	agents, err := s.client.ListAgents()
	if err != nil {
		return ""
	}
	for _, a := range agents {
		if a.AgentKey == "chief" || a.AgentKey == "prime" {
			s.primeID = a.ID
			s.state.PrimeID = a.ID
			return s.primeID
		}
	}
	return ""
}

func (s *step5Channels) activeInput() *textinput.Model {
	cs := &s.ch[s.selIdx]
	if len(cs.inputs) == 0 {
		return nil
	}
	return &cs.inputs[cs.activeField]
}

func (s *step5Channels) Update(msg tea.Msg) (stepModel, tea.Cmd) {
	switch m := msg.(type) {

	case channelConnectResultMsg:
		s.ch[m.idx].busy = false
		if m.err != nil {
			s.ch[m.idx].err = m.err.Error()
		} else {
			s.ch[m.idx].connected = true
			s.ch[m.idx].err = ""
			def := channelDefs[m.idx]
			already := false
			for _, c := range s.state.ConnectedChannels {
				if c == def.id {
					already = true
					break
				}
			}
			if !already {
				s.state.ConnectedChannels = append(s.state.ConnectedChannels, def.id)
			}
		}
		return s, s.spinner.Tick

	case tea.KeyMsg:
		anySpin := false
		for _, c := range s.ch {
			if c.busy {
				anySpin = true
				break
			}
		}
		if anySpin {
			var cmd tea.Cmd
			s.spinner, cmd = s.spinner.Update(msg)
			return s, cmd
		}

		cs := &s.ch[s.selIdx]
		def := channelDefs[s.selIdx]

		switch m.String() {
		case "tab":
			// Tab within a channel: advance field; at last field, move to next channel
			cs.inputs[cs.activeField].Blur()
			cs.activeField++
			if cs.activeField >= len(cs.inputs) {
				cs.activeField = 0
				// Move to next channel
				s.selIdx = (s.selIdx + 1) % len(channelDefs)
				s.ch[s.selIdx].inputs[s.ch[s.selIdx].activeField].Focus()
			} else {
				cs.inputs[cs.activeField].Focus()
			}
			return s, textinput.Blink

		case "shift+tab":
			cs.inputs[cs.activeField].Blur()
			if cs.activeField > 0 {
				cs.activeField--
				cs.inputs[cs.activeField].Focus()
			} else {
				s.selIdx = (s.selIdx + len(channelDefs) - 1) % len(channelDefs)
				newCs := &s.ch[s.selIdx]
				newCs.activeField = len(newCs.inputs) - 1
				newCs.inputs[newCs.activeField].Focus()
			}
			return s, textinput.Blink

		case "down":
			cs.inputs[cs.activeField].Blur()
			cs.activeField = 0
			s.selIdx = (s.selIdx + 1) % len(channelDefs)
			s.ch[s.selIdx].activeField = 0
			s.ch[s.selIdx].inputs[0].Focus()
			return s, textinput.Blink

		case "up":
			cs.inputs[cs.activeField].Blur()
			s.selIdx = (s.selIdx + len(channelDefs) - 1) % len(channelDefs)
			s.ch[s.selIdx].activeField = 0
			s.ch[s.selIdx].inputs[0].Focus()
			return s, textinput.Blink

		case "enter":
			// Connect if at least one field has a value and not yet connected
			hasValue := false
			for _, inp := range cs.inputs {
				if inp.Value() != "" {
					hasValue = true
					break
				}
			}
			if hasValue && !cs.connected {
				cs.busy = true
				cs.err = ""
				return s, tea.Batch(s.connectCmd(s.selIdx), s.spinner.Tick)
			}

		case "ctrl+n":
			return s, Next()
		}

		_ = def
	}

	// Update active input
	anyBusy := false
	for i := range s.ch {
		if s.ch[i].busy {
			anyBusy = true
			break
		}
	}
	if anyBusy {
		var cmd tea.Cmd
		s.spinner, cmd = s.spinner.Update(msg)
		return s, cmd
	}

	cs := &s.ch[s.selIdx]
	if len(cs.inputs) > 0 {
		if cs.activeField >= len(cs.inputs) {
			cs.activeField = len(cs.inputs) - 1
		}
		var cmd tea.Cmd
		cs.inputs[cs.activeField], cmd = cs.inputs[cs.activeField].Update(msg)
		return s, cmd
	}
	return s, nil
}

func (s *step5Channels) View() string {
	title := titleStyle.Render("Connect messaging channels")
	sub := mutedStyle.Render("Optional — skip with Ctrl+N if you don't need chat channels. Tab = next field, ↑↓ = switch channel.")

	var rows []string
	for i, def := range channelDefs {
		cs := s.ch[i]
		isActive := i == s.selIdx

		nameSt := mutedStyle
		if isActive {
			nameSt = lipgloss.NewStyle().Foreground(cPrimary).Bold(true)
		}
		nameStr := nameSt.Render(fmt.Sprintf("%-22s", def.name))

		var status string
		if cs.busy {
			status = s.spinner.View() + " connecting…"
		} else if cs.connected {
			status = successSt.Render("✓ connected")
		} else if cs.err != "" {
			status = errorSt.Render("✗ " + truncate(cs.err, 40))
		}

		if isActive && !cs.connected {
			rows = append(rows, nameStr)
			for j, f := range def.fields {
				fieldLabel := mutedStyle.Render("  " + f.label)
				rows = append(rows, fieldLabel+"\n  "+cs.inputs[j].View())
				if f.hint != "" && j == cs.activeField {
					rows = append(rows, mutedStyle.Render("  ↳ "+f.hint))
				}
			}
			if status != "" {
				rows = append(rows, "  "+status)
			} else {
				rows = append(rows, mutedStyle.Render("  Enter = connect  Tab = next field"))
			}
		} else {
			rows = append(rows, nameStr+" "+status)
		}
		rows = append(rows, "")
	}

	connectedCount := 0
	var connNames []string
	for i, cs := range s.ch {
		if cs.connected {
			connectedCount++
			connNames = append(connNames, channelDefs[i].name)
		}
	}
	summary := ""
	if connectedCount > 0 {
		summary = "\n" + wSuccess("Connected: "+strings.Join(connNames, ", "))
	}

	return title + "\n" + sub + summary + "\n\n" +
		strings.Join(rows, "\n") +
		mutedStyle.Render("Tab = next field  •  ↑↓ = switch channel  •  Enter = connect  •  Ctrl+N = skip/proceed")
}

func (s *step5Channels) NextDisabled() bool { return false }
func (s *step5Channels) NextLabel() string  { return "" }

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
