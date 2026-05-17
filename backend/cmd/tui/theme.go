// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tui

import (
	"charm.land/lipgloss/v2"
)

// Theme colors
var (
	logoMagenta = lipgloss.Color("#FF5FFF")
	brandColor  = lipgloss.Color("#CC44CC")
	hatchColor  = lipgloss.Color("#2D2D44")
	purple      = lipgloss.Color("#8B5CF6")
	dimPurple   = lipgloss.Color("#6D28D9")
	green       = lipgloss.Color("#10B981")
	dimText     = lipgloss.Color("#52525B")
	surface     = lipgloss.Color("#16161F")
	borderClr   = lipgloss.Color("#2A2A3A")
	white       = lipgloss.Color("#E4E4E7")
	cyan        = lipgloss.Color("#06B6D4")
	amber       = lipgloss.Color("#F59E0B")
	red         = lipgloss.Color("#EF4444")
)

// Styles
var (
	sidebarBg    = lipgloss.NewStyle().Background(surface)
	userLabel    = lipgloss.NewStyle().Foreground(purple).Bold(true)
	agentLabel   = lipgloss.NewStyle().Foreground(green).Bold(true)
	systemLabel  = lipgloss.NewStyle().Foreground(amber).Bold(true)
	toolStyle    = lipgloss.NewStyle().Foreground(cyan)
	dimStyle     = lipgloss.NewStyle().Foreground(dimText)
	sectionTitle = lipgloss.NewStyle().Foreground(dimText)
	activeItem   = lipgloss.NewStyle().Foreground(white)
	logoStyle    = lipgloss.NewStyle().Foreground(logoMagenta).Bold(true)
	brandStyle   = lipgloss.NewStyle().Foreground(brandColor)
	hatchStyle   = lipgloss.NewStyle().Foreground(hatchColor)
	dividerStyle = lipgloss.NewStyle().Foreground(borderClr)
	statusKey    = lipgloss.NewStyle().Foreground(white).Bold(true)
	statusDesc   = lipgloss.NewStyle().Foreground(dimText)
	statusBar    = lipgloss.NewStyle().Foreground(dimText).Background(surface).Padding(0, 1)
	errorStyle   = lipgloss.NewStyle().Foreground(red)
	headerStyle  = lipgloss.NewStyle().Foreground(logoMagenta).Bold(true)
	cellDim      = lipgloss.NewStyle().Foreground(dimText)
	cellNormal   = lipgloss.NewStyle().Foreground(white)
	cellGreen    = lipgloss.NewStyle().Foreground(green)
	cellCyan     = lipgloss.NewStyle().Foreground(cyan)
)

// Slash commands
type slashCmd struct {
	name string
	desc string
}

var slashCommands = []slashCmd{
	{"/model", "Switch LLM model"},
	{"/agent", "Switch agent"},
	{"/new", "Clear local view (canonical chat kept server-side)"},
	{"/clear", "Clear messages"},
	{"/help", "Show commands"},
	{"/agents", "View all agents"},
	{"/sessions", "Other channels (email, groups)"},
	{"/tools", "View tools"},
	{"/providers", "View providers"},
	{"/voice", "View voice (TTS/STT) providers"},
	{"/media", "View media (image/video) providers"},
	{"/keys", "Manage API keys"},
	{"/code", "Code mode (VS Code layout)"},
	{"/status", "Gateway status"},
	{"/compact", "Toggle sidebar"},
	{"/research", "Search social platforms"},
	{"/read", "Read a URL"},
	{"/vault", "Secret vault — keys, notes, credentials"},
	{"/usage", "Token usage by agent"},
	{"/home", "Home dashboard"},
	{"/scan", "Prompt injection scan"},
	{"/memory", "Search agent memories"},
	{"/skills", "List available skills"},
	{"/approve", "Approve pending actions"},
	{"/deny", "Deny pending actions"},
	{"/whoami", "Show current agent/model/session"},
	{"/tokens", "Show token usage"},
	{"/search", "Search message history"},
	{"/reset", "Reset session"},
	{"/export", "Export session"},
	{"/import", "Import session"},
	{"/config", "View/edit config"},
	{"/team", "Show team tasks"},
	{"/cron", "List cron jobs"},
	{"/drive", "List files"},
	{"/mail", "Check inbox"},
	{"/notifications", "Notifications centre"},
	{"/budget", "Token usage by agent"},
	{"/projects", "List all projects"},
	{"/project", "List all projects"},
	{"/supervisor", "Supervisor — escalations and health"},
	{"/router", "Smart router — category assignments"},
	{"/settings", "Settings overview — system, agents, providers"},
	{"/agent new", "Create a new agent"},
	{"/provider add", "Add a provider"},
	{"/key add", "Add an API key"},
	{"/channel new", "Create a channel"},
	{"/cron new", "Create a cron job"},
	{"/mcp add", "Add an MCP server"},
	{"/room new", "Create a room/hub"},
	{"/task new", "Create a task"},
	{"/workflows", "Workflow automations"},
	{"/workflow new", "Create a workflow"},
	{"/voice add", "Add a voice provider (TTS/STT/realtime)"},
	{"/modelshub", "Models Hub — providers + discovery"},
	{"/discovered", "Review newly discovered models"},
	{"/workers", "Daemon workers and plans"},
	{"/plans", "Approval queue"},
	{"/github", "GitHub — PRs, issues, tasks  (use /github owner/repo to connect)"},
	{"/gh", "GitHub (alias)"},
	{"/channels", "Channels (email, Slack, etc.)"},
	{"/rooms", "Rooms / collaboration hubs"},
	{"/tasks", "Task list"},
	{"/mcp", "MCP servers"},
	{"/exit", "Quit"},
}

func mmin(a, b int) int {
	if a < b {
		return a
	}
	return b
}

