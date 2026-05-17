// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package agent

// SystemPromptConfig holds all inputs for system prompt construction.
type SystemPromptConfig struct {
	AgentID       string
	Model         string
	Workspace     string
	Channel       string
	ChannelType   string
	ChatTitle     string
	PeerKind      string // "direct" or "group"
	OwnerIDs      []string
	Mode          PromptMode
	ToolNames     []string
	SkillsSummary string
	HasMemory     bool
	HasSpawn      bool
	HasTeam       bool
	TeamWorkspace string
	TeamMembers   []TeamMemberData
	TeamGuidance  string
	ContextFiles  []ContextFile
	ExtraPrompt   string
	AgentType     string // "open" or "predefined"

	HasSkillSearch    bool
	HasSkillManage    bool
	HasMCPToolSearch  bool
	HasKnowledgeGraph bool
	MCPToolDescs      map[string]string

	// Sandbox info
	SandboxEnabled        bool
	SandboxContainerDir   string
	SandboxWorkspaceAccess string

	ProviderType string
	SelfEvolve   bool
	LearnedHints string // Dynamic hints from learning loop (e.g. "User prefers concise answers")

	ShellDenyGroups      map[string]bool
	CredentialCLIContext string
	IsBootstrap          bool
}

// ContextFile represents a loaded context file (SOUL.md, AGENTS.md, etc.)
type ContextFile struct {
	Path    string
	Content string
}

// TeamMemberData represents a team member for task assignment.
type TeamMemberData struct {
	AgentKey    string
	DisplayName string
	Role        string
	Frontmatter string
}

// coreToolSummaries maps tool names to one-line descriptions.
var coreToolSummaries = map[string]string{
	// Filesystem
	"read_file":  "Read file contents",
	"write_file": "Create or overwrite files",
	"list_files": "List directory contents",
	"edit":       "Edit a file by replacing exact text matches",
	"exec":       "Run shell commands",
	"lsp":        "Semantic code navigation — find definitions, references, symbols across a codebase",

	// Coding tools — file discovery, search, and patch primitives.
	"glob":        "Find files by glob pattern — **/*.go, src/**/*.ts — sorted by modification time",
	"grep":        "Search file contents by regex — find function definitions, usages, errors across codebase",
	"diagnostics": "Check code for errors — runs go build, tsc, cargo check and returns diagnostics",
	"apply_patch": "Apply multi-file patches atomically — add, update, delete files in one operation",
	"undo":        "Undo the last edit to a file — restores previous version from this session",

	// Web — anti-bot scraping with 5-layer bypass (TLS fingerprint, HTTP/2, headers, headless browser, proxy)
	"web_search": "Search the web via 11 providers (Tavily, Brave, Exa, Jina, Kagi, DuckDuckGo, SearXNG, Serper, Perplexity, Google, Bing)",
	"web_fetch":  "Fetch and extract content from a URL — has built-in anti-bot bypass and headless browser fallback for protected sites",
	"crawl":      "Deep-crawl a website — follows links, extracts structured content from multiple pages",
	"scrape":     "Scrape structured data from a page — tables, lists, prices, specs",

	// Research & Knowledge (prefer these for complex questions)
	"research":              "Deep research — decomposes question into sub-queries, searches in parallel, synthesizes answer with citations",
	"memory_search":         "Search indexed memory files",
	"memory_get":            "Read specific sections of memory files",
	"knowledge_graph_search": "Find people, projects, and connections in the knowledge graph",

	// Flight & Travel
	"flight_search": "Search real-time flight prices and schedules — uses Google Flights natively, no API key needed",

	// Social & Communication
	"social_monitor": "Monitor social media — track mentions, trends, sentiment across 15 platforms",
	"send_dm":        "Send a direct message to a user on any connected channel",
	"send_telegram":  "Send a Telegram message to the owner",
	"email_send":     "Send an email via SMTP (supports attachments)",
	"email_read":     "Read emails from IMAP inbox",
	"message":        "Send a proactive message to another channel/chat",

	// Browser & Automation
	"browse_and_act": "Autonomous browser agent — navigates pages, fills forms, clicks buttons, extracts data from JS-rendered sites",
	"browser":        "Browse web pages interactively",

	// Multi-Agent
	"delegate":    "Delegate a task to a specialist agent — use for complex tasks that need domain expertise",
	"list_agents": "List available specialist agents and their roles",
	"spawn":         "Spawn a self-clone subagent for background tasks",
	"manage_agents": "Create, update, or delete agents — actions: create (name, model, role), update (id, model, system_prompt), delete (id)",

	// Autonomous Coding
	"project":        "Create an isolated coding workspace — write, build, test code in a sandboxed project",
	"project_manager": "Manage coding projects — create, list, switch, add tasks, update notes",
	"prime_coder":    "Structured coding workflow — plan, spec, rules, memory, tasks for a project",
	"self_knowledge": "Query this agent's own codebase — packages, files, DB schema, build status, git log",
	"self_patch":     "Propose a code change to the agent's own codebase — creates branch, builds, auto-reverts on failure",
	"self_test":      "Run go build + go test on the agent's own codebase — returns pass/fail with output",
	"self_improve":   "Analyze codebase for improvements — vet warnings, low test coverage, TODOs, then suggest fixes",

	// Scheduling & Sessions
	"datetime":         "Get current date/time with timezone support",
	"cron":             "Manage scheduled jobs and reminders",
	"heartbeat":        "Manage agent heartbeat for autonomous monitoring",
	"sessions_list":    "List sessions for this agent",
	"sessions_history": "View conversation history of a session",
	"session_status":   "Check status of a session",
	"sleep":            "QOROS background agent — sleep until next tick",
	"daily_log":        "QOROS background agent — write daily activity log",

	// Skills
	"skill_search": "Search available skills by keyword",
	"skill_manage": "Create, patch, or delete skills",
	"use_skill":    "Invoke a skill by name",

	// Media
	"read_image":    "Analyze images — use with path from <media:image> tags",
	"read_audio":    "Analyze audio — use with media_id from <media:audio> tags",
	"read_video":    "Analyze video — use with media_id from <media:video> tags",
	"read_document": "Analyze documents (PDF, DOCX, etc.)",
	"create_image":  "Generate images from text descriptions",
	"create_audio":  "Generate music or sound effects",
	"create_video":  "Generate videos from text descriptions",

	// Team & Collaboration
	"team_tasks":   "Team task board — track progress, manage dependencies",
	"team_message": "Send a message to a team member",
	"join_room":    "Join a collaboration room",
	"leave_room":   "Leave a collaboration room",
	"room_post":    "Post a message to a collaboration room",

	// Connectors & Actions
	"execute_action":  "Execute a native connector action (GitHub, Jira, Slack, etc.)",
	"clarify":         "Ask the user a clarifying question before proceeding",
	"mcp_tool_search": "Search for available MCP tools",

	// Utilities
	"qorven_download": "Download a file from a URL to the workspace",
	"qorven_fly":      "Deploy to Fly.io",
	"qorven_lint":     "Lint and check code quality",
	"qorven_report":   "Generate a structured report",
	"qorven_wiki":     "Search or write to the wiki knowledge base",
	"tts":             "Convert text to speech audio",
}
