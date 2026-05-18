// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tui

import (
	"fmt"
	"strings"
)

// openCreateForm opens the appropriate create form for the current table route.
func (m *Model) openCreateForm() {
	switch m.route {
	case routeProviders:
		m.enterFormProvider()
	case routeChannels:
		m.enterFormChannel()
	case routeCron:
		m.enterFormCron()
	case routeMCP:
		m.enterFormMCP()
	case routeRooms:
		m.enterFormRoom()
	case routeTasks:
		m.enterFormTask()
	case routeAgents:
		m.enterFormAgent()
	case routeSettings:
		m.enterFormIntegration()
	case routeVoice:
		m.enterFormVoice()
	case routeWorkflows:
		m.enterFormWorkflow()
	case routeVault:
		m.enterFormVault()
	case routeProviderKeys:
		m.formKeyID = m.providerKeysID
		m.enterFormKey()
		m.formReturn = routeProviderKeys // override — return to keys screen, not providers
	}
}

func (m *Model) enterFormVault() {
	m.formReturn = routeVault
	m.form = newFormModel("Add Vault Entry", []formField{
		{label: "Name", placeholder: "e.g. my-api-key"},
		{label: "Kind", placeholder: "secret / note / credential",
			choices: []string{"secret", "note", "credential"}},
		{label: "Value", placeholder: "Secret value…", secret: true},
		{label: "Description", placeholder: "Optional description"},
	}, m.width)
	m.route = routeFormVault
}

func (m *Model) enterFormIntegration() {
	m.formReturn = routeSettings
	m.form = newFormModel("Set Integration API Key", []formField{
		{label: "Integration", placeholder: "llmstats / artificialanalysis",
			choices: []string{"llmstats", "artificialanalysis"}},
		{label: "API Key", placeholder: "Paste API key…", secret: true},
	}, m.width)
	m.route = routeFormIntegration
}

func (m *Model) enterFormProvider() {
	m.formReturn = routeProviders
	// Load provider types from the live catalog; fall back to a minimal list if unavailable.
	types := m.api.listProviderCatalog()
	if len(types) == 0 {
		types = []string{"openai", "anthropic", "gemini", "groq", "deepseek", "mistral", "xai", "openrouter", "together", "fireworks", "cohere", "perplexity", "bedrock", "ollama", "custom"}
	}
	m.form = newFormModel("Add Provider", []formField{
		{label: "Type", placeholder: "select provider type", choices: types},
		{label: "Name", placeholder: "My Provider"},
		{label: "API Base", placeholder: "https://api.openai.com/v1"},
		{label: "API Key", placeholder: "(optional — add later with n on Providers)", secret: true},
	}, m.width)
	m.route = routeFormProvider
}

func (m *Model) enterFormKey() {
	m.formReturn = routeProviders
	providers := m.api.listProviders()
	var ids []string
	for _, p := range providers {
		ids = append(ids, p.FullID+" ("+p.Name+")")
	}
	m.form = newFormModel("Add API Key", []formField{
		{label: "Provider ID", placeholder: "select provider", choices: ids},
		{label: "API Key", placeholder: "sk-...", secret: true},
	}, m.width)
	m.route = routeFormKey
}

func (m *Model) enterFormAgent() {
	m.formReturn = routeAgents
	m.form = newFormModel("Create Agent", []formField{
		{label: "Name", placeholder: "My Agent"},
		{label: "Role", placeholder: "assistant / researcher / coder / analyst"},
		{label: "Model", placeholder: "claude-sonnet-4 / gpt-4o / deepseek-chat"},
		{label: "System Prompt", placeholder: "You are a helpful assistant..."},
		{label: "Tool Profile", placeholder: "(optional) default / code / research"},
	}, m.width)
	m.route = routeFormAgent
}

func (m *Model) enterFormAgentEdit(currentName, currentModel string) {
	m.formReturn = routeAgents
	m.form = newFormModel("Edit Agent: "+currentName, []formField{
		{label: "Name", placeholder: currentName},
		{label: "Role", placeholder: "assistant / researcher / coder / analyst"},
		{label: "Model", placeholder: currentModel},
		{label: "System Prompt", placeholder: "(optional) new system prompt"},
	}, m.width)
	m.route = routeFormAgentEdit
}

func (m *Model) enterFormChannel() {
	m.formReturn = routeChannels
	agents := m.api.listAgents()
	var agentChoices []string
	for _, a := range agents {
		agentChoices = append(agentChoices, a.ID+" ("+a.Name+")")
	}
	channelTypes := []string{"slack", "email", "telegram", "webhook", "discord", "whatsapp", "sms", "twitter", "instagram", "line", "signal", "wechat"}
	m.form = newFormModel("Create Channel", []formField{
		{label: "Type", placeholder: "select channel type", choices: channelTypes},
		{label: "Name", placeholder: "My Channel"},
		{label: "Agent ID", placeholder: "select agent", choices: agentChoices},
		{label: "Token / Webhook URL", placeholder: "xoxb-... / https://hook.example.com"},
		{label: "Phone / Email (optional)", placeholder: "+1555... or user@example.com"},
	}, m.width)
	m.route = routeFormChannel
}

func (m *Model) enterFormCron() {
	m.formReturn = routeCron
	agents := m.api.listAgents()
	var agentChoices []string
	for _, a := range agents {
		agentChoices = append(agentChoices, a.ID+" ("+a.Name+")")
	}
	m.form = newFormModel("Create Cron Job", []formField{
		{label: "Agent ID", placeholder: "select agent", choices: agentChoices},
		{label: "Schedule (cron)", placeholder: "0 9 * * 1-5  (weekdays at 9am)"},
		{label: "Task Prompt", placeholder: "Daily standup summary"},
	}, m.width)
	m.route = routeFormCron
}

func (m *Model) enterFormMCP() {
	m.formReturn = routeMCP
	m.form = newFormModel("Add MCP Server", []formField{
		{label: "Name", placeholder: "my-mcp-server"},
		{label: "URL", placeholder: "http://localhost:8080"},
	}, m.width)
	m.route = routeFormMCP
}

func (m *Model) enterFormRoom() {
	m.formReturn = routeRooms
	m.form = newFormModel("Create Room / Hub", []formField{
		{label: "Name", placeholder: "Engineering Hub"},
		{label: "Description", placeholder: "Team collaboration space"},
	}, m.width)
	m.route = routeFormRoom
}

func (m *Model) enterFormTask() {
	m.formReturn = routeTasks
	agents := m.api.listAgents()
	agentChoices := []string{"(none)"}
	for _, a := range agents {
		agentChoices = append(agentChoices, a.ID+" ("+a.Name+")")
	}
	m.form = newFormModel("Create Task", []formField{
		{label: "Title", placeholder: "Implement login flow"},
		{label: "Agent ID", placeholder: "select agent (optional)", choices: agentChoices},
		{label: "Priority", placeholder: "low / medium / high", choices: []string{"low", "medium", "high"}},
	}, m.width)
	m.route = routeFormTask
}

func (m *Model) enterFormKeyBudget() {
	if m.route == routeProviderKeys {
		m.formReturn = routeProviderKeys
	} else {
		m.formReturn = routeProviders
	}
	m.form = newFormModel(fmt.Sprintf("Set Key Budget (Provider: %s)", m.formKeyID), []formField{
		{label: "Monthly USD Budget", placeholder: "100.00  (0 = unlimited)"},
		{label: "Monthly Token Budget", placeholder: "5000000  (0 = unlimited)"},
	}, m.width)
	m.route = routeFormKeyBudget
}

func (m *Model) enterFormPoolStrategy() {
	m.formReturn = routeProviders
	m.form = newFormModel(fmt.Sprintf("Pool Strategy (Provider: %s)", m.formKeyID), []formField{
		{label: "Strategy", placeholder: "priority / round_robin / least_used", choices: []string{"priority", "round_robin", "least_used"}},
		{label: "Failover Mode", placeholder: "on_exhaust / on_error / always", choices: []string{"on_exhaust", "on_error", "always"}},
	}, m.width)
	m.route = routeFormPoolStrategy
}

func (m *Model) enterFormVoice() {
	m.formReturn = routeVoice
	m.form = newFormModel("Add Voice Provider", []formField{
		{label: "Name", placeholder: "My TTS"},
		{label: "Kind", placeholder: "tts / stt / realtime", choices: []string{"tts", "stt", "realtime"}},
		{label: "Driver", placeholder: "openai / elevenlabs / deepgram / cartesia / …", choices: []string{
			"openai", "openai_compat", "elevenlabs", "deepgram", "cartesia", "groq",
			"assemblyai", "huggingface", "ollama_voice", "kokoro", "edge_tts",
			"faster_whisper", "moonshine", "piper", "openai_realtime", "gemini_live",
		}},
		{label: "API Base", placeholder: "https://api.openai.com/v1  (leave blank for default)"},
		{label: "API Key", placeholder: "sk-…", secret: true},
	}, m.width)
	m.route = routeFormVoice
}

func (m *Model) enterFormChannelEdit(currentName, channelType string) {
	m.formReturn = routeChannels
	m.form = newFormModel("Edit Channel: "+currentName, []formField{
		{label: "Name", placeholder: currentName},
		{label: "Token / Webhook URL", placeholder: "xoxb-... / https://hook.example.com (leave blank to keep)"},
	}, m.width)
	m.route = routeFormChannelEdit
}

func (m *Model) enterFormVoiceEdit(currentName string) {
	m.formReturn = routeVoice
	m.form = newFormModel("Edit Voice Provider: "+currentName, []formField{
		{label: "Name", placeholder: currentName},
		{label: "API Base", placeholder: "leave blank to keep current"},
		{label: "API Key", placeholder: "leave blank to keep current", secret: true},
	}, m.width)
	m.route = routeFormVoiceEdit
}

func (m *Model) enterFormWorkflow() {
	m.formReturn = routeWorkflows
	m.form = newFormModel("Create Workflow", []formField{
		{label: "Name", placeholder: "Daily standup report"},
		{label: "Description", placeholder: "(optional) what this workflow does"},
	}, m.width)
	m.route = routeFormWorkflow
}

// handleFormSubmit is called when the form signals done.
func (m *Model) handleFormSubmit() {
	v := m.form.values()
	var err error
	var successMsg string

	switch m.route {
	case routeFormProvider:
		err = m.api.createProvider(v["Type"], v["Name"], v["API Base"], v["API Key"])
		successMsg = "Created provider: " + v["Name"]

	case routeFormKey:
		providerID := v["Provider ID"]
		if idx := strings.Index(providerID, " ("); idx > 0 {
			providerID = providerID[:idx]
		}
		err = m.api.addProviderKey(providerID, v["API Key"])
		successMsg = "Added API key to provider: " + providerID

	case routeFormAgent:
		err = m.api.createAgent(v["Name"], v["Role"], v["Model"], v["System Prompt"], v["Tool Profile"])
		successMsg = "Created agent: " + v["Name"]

	case routeFormChannel:
		agentID := v["Agent ID"]
		if idx := strings.Index(agentID, " ("); idx > 0 {
			agentID = agentID[:idx]
		}
		err = m.api.createChannelFull(v["Type"], v["Name"], agentID, v["Token / Webhook URL"], v["Phone / Email (optional)"])
		successMsg = "Created channel: " + v["Name"]

	case routeFormCron:
		agentID := v["Agent ID"]
		if idx := strings.Index(agentID, " ("); idx > 0 {
			agentID = agentID[:idx]
		}
		err = m.api.createCronJob(agentID, v["Schedule (cron)"], v["Task Prompt"])
		successMsg = "Created cron job"

	case routeFormMCP:
		err = m.api.createMCPServer(v["Name"], v["URL"])
		successMsg = "Added MCP server: " + v["Name"]

	case routeFormRoom:
		err = m.api.createRoom(v["Name"], v["Description"])
		successMsg = "Created room: " + v["Name"]

	case routeFormTask:
		agentID := v["Agent ID"]
		if agentID == "(none)" {
			agentID = ""
		} else if idx := strings.Index(agentID, " ("); idx > 0 {
			agentID = agentID[:idx]
		}
		err = m.api.createTask(v["Title"], agentID, v["Priority"])
		successMsg = "Created task: " + v["Title"]

	case routeFormAgentEdit:
		err = m.api.updateAgent(m.formEditID, v["Name"], v["Role"], v["Model"], v["System Prompt"])
		successMsg = "Updated agent: " + v["Name"]

	case routeFormKeyBudget:
		var budgetUSD float64
		var budgetTokens int64
		fmt.Sscanf(v["Monthly USD Budget"], "%f", &budgetUSD)
		fmt.Sscanf(v["Monthly Token Budget"], "%d", &budgetTokens)
		err = m.api.setKeyBudget(m.formKeyID, budgetUSD, budgetTokens)
		successMsg = "Budget updated for provider: " + m.formKeyID

	case routeFormPoolStrategy:
		err = m.api.savePoolConfig(m.formKeyID, v["Strategy"], v["Failover Mode"])
		successMsg = "Pool strategy updated for provider: " + m.formKeyID

	case routeFormIntegration:
		err = m.api.saveIntegration(v["Integration"], v["API Key"])
		successMsg = "API key saved for integration: " + v["Integration"]

	case routeFormVoice:
		err = m.api.createVoiceProvider(v["Name"], v["Kind"], v["Driver"], v["API Base"], v["API Key"])
		successMsg = "Added voice provider: " + v["Name"]

	case routeFormWorkflow:
		wfID, createErr := m.api.createWorkflow(v["Name"], v["Description"])
		if createErr != nil {
			err = createErr
		} else {
			shortID := wfID
			if len(shortID) > 8 {
				shortID = shortID[:8]
			}
			successMsg = "Created workflow: " + v["Name"] + " (" + shortID + ")"
		}

	case routeFormVault:
		err = m.api.createVaultEntry(v["Name"], v["Kind"], v["Value"], v["Description"])
		successMsg = "Vault entry created: " + v["Name"]

	case routeFormChannelEdit:
		err = m.api.updateChannel(m.formEditID, v["Name"], v["Token / Webhook URL"])
		successMsg = "Updated channel: " + v["Name"]

	case routeFormVoiceEdit:
		err = m.api.updateVoiceProvider(m.formEditID, v["Name"], v["API Base"], v["API Key"])
		successMsg = "Updated voice provider: " + v["Name"]
	}

	if err != nil {
		m.form.err = err.Error()
		m.form.done = false
		return
	}

	m.messages = append(m.messages, ChatMessage{Role: "system", Content: "✓ " + successMsg})
	m.route = m.formReturn
	m.formEditID = ""
	m.formKeyID = ""
	m.refreshActiveList()
}
