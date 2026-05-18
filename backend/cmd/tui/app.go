// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tui

import (
	"context"
	"os"
	"time"

	"charm.land/bubbles/v2/filepicker"
	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/stopwatch"
	"charm.land/bubbles/v2/table"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// ---------------- Routes ----------------

type route int

const (
	routeChat route = iota
	routeAgents
	routeSessions
	routeProviders
	routeTools
	routeProjects
	routeModelPicker
	routeAgentPicker
	routeHelp
	routeCode
	routeVoice
	routeMedia
	routeModels
	routeWorkers
	routeTasks
	routeCron
	routeMCP
	routeRooms
	routeChannels
	routeSkills
	routePlans
	routeMemory
	routeDrive
	routeDiscovered
	routeGitHub

	// Form routes — each maps to a specific create operation
	routeFormProvider
	routeFormKey
	routeFormAgent
	routeFormChannel
	routeFormCron
	routeFormMCP
	routeFormRoom
	routeFormTask
	routeFormKeyBudget
	routeFormPoolStrategy
	routeFormAgentEdit
	routeFormIntegration

	// Skill marketplace picker
	routeSkillMarket

	// System routes
	routeSupervisor
	routeRouter

	// File picker for drive upload
	routeDriveUpload

	// Settings overview
	routeSettings

	// Workflows
	routeWorkflows
	routeFormWorkflow

	// Voice
	routeFormVoice
	routeFormChannelEdit
	routeFormVoiceEdit

	// New screens: home dashboard, usage, room chat, vault, provider keys, notifications
	routeHome
	routeUsage
	routeRoomChat
	routeVault
	routeProviderKeys
	routeNotifications
	routeFormVault
)

// ---------------- Messages ----------------

type ChatMessage struct {
	Role       string
	Content    string
	Tools      []string
	ToolEvents []toolEvent
	Widgets    []widgetRef
}

type buildPhaseMsg struct{ phase string }

type homeLoadedMsg struct{ data homeDashboard }
type pingResultMsg struct {
	ok      bool
	latency string
}
type roomMessagesLoadedMsg struct {
	roomID   string
	messages []RoomMessage
}
type agentHistoryLoadedMsg struct {
	agentID     string
	messages    []TUIConversationMessage
	discussions []TUIDiscussion
}
type vaultLoadedMsg struct{ entries []VaultEntry }
type usageLoadedMsg struct{ data UsageSummary }
type notificationsLoadedMsg struct{ data []NotificationInfo }
type providerKeysLoadedMsg struct {
	providerID string
	pool       PoolInfo
}

type realTimeMsg struct {
	kind    string
	payload string
}

type pingTickMsg struct{}
type rtConnectedMsg struct{}

var buildPhaseFraction = map[string]float64{
	"idle":             0.0,
	"planning":         0.10,
	"pending_approval": 0.20,
	"spawning":         0.30,
	"building":         0.50,
	"reviewing":        0.65,
	"pushing":          0.80,
	"awaiting_ci":      0.90,
	"preview":          0.95,
	"done":             1.00,
}

// ---------------- Model ----------------

type Model struct {
	agentName string
	agentID   string
	modelName string
	sessionID string
	api       *apiClient

	width       int
	height      int
	ready       bool
	sidebarOpen bool
	route       route

	viewport      viewport.Model
	textarea      textarea.Model
	messages      []ChatMessage
	historyMsgs  []TUIConversationMessage // loaded once on agent selection from /agents/{id}/messages
	discussions  []TUIDiscussion          // loaded once on agent selection from /agents/{id}/discussions
	historyReady bool
	streaming     string
	isStreaming   bool
	thinkingLevel string
	streamCancel  context.CancelFunc

	spinner     spinner.Model
	streamTimer stopwatch.Model
	help        help.Model

	slashPopup     list.Model
	slashPopupOpen bool

	agentsList   list.Model
	toolsList    list.Model
	projectsList list.Model

	sessionsTable   table.Model
	providersTable  table.Model
	voiceTable      table.Model
	mediaTable      table.Model
	modelsTable     table.Model
	tasksTable      table.Model
	cronTable       table.Model
	mcpTable        table.Model
	roomsTable      table.Model
	channelsTable   table.Model
	skillsTable     table.Model
	plansTable      table.Model
	memoryTable     table.Model
	driveTable      table.Model
	discoveredTable table.Model
	supervisorTable table.Model
	routerTable     table.Model
	settingsTable   table.Model
	workflowsTable  table.Model

	modelPicker       list.Model
	agentPicker       list.Model
	helpList          list.Model
	skillMarketPicker list.Model

	agentsData      []AgentInfo
	sessionsData    []SessionInfo
	providersData   []ProviderInfo
	modelsData      []ModelHubEntry
	toolsData       []ToolInfo
	projectsData    []ProjectInfo
	voiceData       []VoiceProviderInfo
	mediaData       []MediaProviderInfo
	workersAgents   []DaemonAgentInfo
	workersTasks    []DaemonTaskInfo
	workersPlans    []DaemonPlanInfo
	tasksData       []TaskInfo
	cronData        []CronInfo
	mcpData         []MCPServerInfo
	roomsData       []RoomInfo
	channelsData    []ChannelInfo
	skillsData      []SkillInfo
	plansData       []PlanInfo
	memoryData      []MemoryResult
	driveData       []DriveFileInfo
	workflowsData   []WorkflowInfo
	memoryQuery     string
	discoveredData  []DiscoveredModel
	escalationsData []EscalationInfo
	routerData      []RouterCategoryInfo
	rankingsData    []ModelRankingInfo
	formKeyID       string
	formEditID      string

	form              formModel
	formReturn        route
	driveUploadPicker filepicker.Model

	codeFiles     []string
	codeChanges   []string
	codeContent   string
	codePath      string
	codeProject   string
	codeCursor    int
	codePicker    filepicker.Model
	buildProgress progress.Model
	buildPhase    string

	githubOwner      string
	githubRepo       string
	githubPRs        []ghPR
	githubIssues     []ghIssue
	githubTasks      []ghTask
	githubSubView    int
	githubPRTable    table.Model
	githubIssueTable table.Model
	githubTaskTable  table.Model

	gatewayOnline  bool
	gatewayLatency string
	pingSeq        int
	lastPingOK     bool

	homeData         homeDashboard
	homeSessionTable table.Model

	roomChatID       string
	roomChatName     string
	roomMessages     []RoomMessage
	roomChatViewport viewport.Model
	roomChatInput    textarea.Model

	vaultData  []VaultEntry
	vaultTable table.Model

	providerKeysID    string
	providerKeysData  PoolInfo
	providerKeysTable table.Model

	notificationsData  []NotificationInfo
	notificationsTable table.Model

	workersSubView    int
	workersAgentTable table.Model
	workersPlanTable  table.Model

	usageData  UsageSummary
	usageTable table.Model

	helpSection int

	mentionPopupOpen bool
	mentionPopup     list.Model
	mentionFiles     []string

	pingTick    int
	rtSubActive bool
	consumedKey bool
	taskCount   int
}

// ---------------- Construction ----------------

func NewModel(agentName, agentID, modelName, sessionID, server, token string) Model {
	ta := textarea.New()
	ta.Placeholder = "Message Qorven… (/ for commands, ? for help)"
	ta.Focus()
	ta.CharLimit = 10000
	ta.ShowLineNumbers = false
	ta.SetHeight(1)
	s := ta.Styles()
	s.Focused.Prompt = lipgloss.NewStyle()
	s.Blurred.Prompt = lipgloss.NewStyle()
	ta.SetStyles(s)
	ta.Prompt = ""

	sp := spinner.New(
		spinner.WithSpinner(spinner.MiniDot),
		spinner.WithStyle(lipgloss.NewStyle().Foreground(purple)),
	)

	sw := stopwatch.New(stopwatch.WithInterval(time.Second))

	hp := help.New()

	pr := progress.New(progress.WithDefaultBlend(), progress.WithWidth(40))

	roomTa := textarea.New()
	roomTa.Placeholder = "Message room…"
	roomTa.CharLimit = 2000
	roomTa.ShowLineNumbers = false
	roomTa.SetHeight(1)
	rs := roomTa.Styles()
	rs.Focused.Prompt = lipgloss.NewStyle()
	rs.Blurred.Prompt = lipgloss.NewStyle()
	roomTa.SetStyles(rs)
	roomTa.Prompt = ""

	startRoute := routeHome
	if sessionID != "" {
		startRoute = routeChat
	}

	m := Model{
		agentName:     agentName,
		agentID:       agentID,
		modelName:     modelName,
		sessionID:     sessionID,
		sidebarOpen:   true,
		api:           newAPI(server, token),
		route:         startRoute,
		messages:      loadSession(sessionID),
		textarea:      ta,
		roomChatInput: roomTa,
		spinner:       sp,
		streamTimer:   sw,
		help:          hp,
		buildProgress: pr,
		gatewayOnline: true,
	}

	go pruneOldSessions(30*24*time.Hour, 100)

	return m
}

// ---------------- Lifecycle ----------------

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{
		textarea.Blink,
		m.spinner.Tick,
		m.loadHomeAsync(),
		m.pingGatewayAsync(),
		m.scheduleNextPing(),
		m.subscribeRealTime(),
	}
	if m.agentID != "" {
		cmds = append(cmds, m.loadAgentHistoryAsync(m.agentID))
	}
	return tea.Batch(cmds...)
}

// ---------------- Entry point ----------------

func Run(agentName, agentID, modelName, sessionID string) error {
	detectCapabilities()

	server := os.Getenv("QORVEN_SERVER")
	if server == "" {
		server = "http://localhost:4200"
	}
	token := os.Getenv("QORVEN_TOKEN")
	if token == "" {
		token = os.Getenv("QORVEN_GATEWAY_TOKEN")
	}
	m := NewModel(agentName, agentID, modelName, sessionID, server, token)
	p := tea.NewProgram(m)
	_, err := p.Run()
	return err
}
