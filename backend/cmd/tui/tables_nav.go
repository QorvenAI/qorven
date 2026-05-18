// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tui

// ─── Entry helpers ───────────────────────────────────────────────────────────

func (m *Model) enterAgentsList() {
	m.agentsData = m.api.listAgents()
	m.agentsList = m.newList("Agents", buildAgentItems(m.agentsData), "agent", "agents")
	m.route = routeAgents
}

func (m *Model) enterToolsList() {
	m.toolsData = m.api.listTools()
	m.toolsList = m.newList("Tools", buildToolItems(m.toolsData), "tool", "tools")
	m.route = routeTools
}

func (m *Model) enterProjectsList() {
	m.projectsData = m.api.listProjects()
	m.projectsList = m.newList("Projects", buildProjectItems(m.projectsData), "project", "projects")
	m.route = routeProjects
}

func (m *Model) enterSessionsTable() {
	m.sessionsData = m.api.listSessions()
	m.sessionsTable = m.newSessionsTable()
	m.route = routeSessions
}

func (m *Model) enterProvidersTable() {
	m.providersData = m.api.listProviders()
	m.providersTable = m.newProvidersTable()
	m.route = routeProviders
}

func (m *Model) enterVoiceTable() {
	m.voiceData = m.api.listVoiceProviders()
	m.voiceTable = m.newVoiceTable()
	m.route = routeVoice
}

func (m *Model) enterMediaTable() {
	m.mediaData = m.api.listMediaProviders()
	m.mediaTable = m.newMediaTable()
	m.route = routeMedia
}

func (m *Model) enterModelsTable() {
	m.modelsData = m.api.listModelHub()
	m.modelsTable = m.newModelsTable()
	m.route = routeModels
}

func (m *Model) enterWorkersTable() {
	m.workersAgents = m.api.listDaemonAgents()
	m.workersTasks = m.api.listDaemonTasks()
	m.workersPlans = m.api.listDaemonPlans()
	m.workersAgentTable = m.newWorkersAgentTable()
	m.workersPlanTable = m.newWorkersPlanTable()
	m.tasksData = m.api.listTasks("")
	m.tasksTable = m.newTasksTable()
	m.route = routeWorkers
}

func (m *Model) enterTasksTable(agentID string) {
	m.tasksData = m.api.listTasks(agentID)
	m.tasksTable = m.newTasksTable()
	m.route = routeTasks
}

func (m *Model) enterCronTable() {
	m.cronData = m.api.listCronJobs()
	m.cronTable = m.newCronTable()
	m.route = routeCron
}

func (m *Model) enterMCPTable() {
	m.mcpData = m.api.listMCPServers()
	m.mcpTable = m.newMCPTable()
	m.route = routeMCP
}

func (m *Model) enterRoomsTable() {
	m.roomsData = m.api.listRooms()
	m.roomsTable = m.newRoomsTable()
	m.route = routeRooms
}

func (m *Model) enterChannelsTable() {
	m.channelsData = m.api.listChannels()
	m.channelsTable = m.newChannelsTable()
	m.route = routeChannels
}

func (m *Model) enterSkillsTable() {
	m.skillsData = m.api.listSkills()
	m.skillsTable = m.newSkillsTable()
	m.route = routeSkills
}

func (m *Model) enterPlansTable() {
	m.plansData = m.api.listPlans()
	m.plansTable = m.newPlansTable()
	m.route = routePlans
}

func (m *Model) enterMemoryTable(query string) {
	m.memoryQuery = query
	m.memoryData = m.api.searchMemory(query)
	m.memoryTable = m.newMemoryTable()
	m.route = routeMemory
}

func (m *Model) enterDriveTable() {
	m.driveData = m.api.listDriveFiles()
	m.driveTable = m.newDriveTable()
	m.route = routeDrive
}

func (m *Model) enterWorkflowsTable() {
	m.workflowsData = m.api.listWorkflows()
	m.workflowsTable = m.newWorkflowsTable()
	m.route = routeWorkflows
}

func (m *Model) enterGitHubScreen() {
	if m.githubOwner == "" {
		for _, p := range m.api.listProjects() {
			if p.GitHubOwner != "" {
				m.githubOwner = p.GitHubOwner
				m.githubRepo = p.GitHubRepo
				break
			}
		}
	}
	m.githubSubView = 0
	m.refreshGitHubData()
	m.route = routeGitHub
}

func (m *Model) refreshGitHubData() {
	if m.githubOwner != "" && m.githubRepo != "" {
		m.githubPRs = m.api.listGitHubPRs(m.githubOwner, m.githubRepo)
		m.githubIssues = m.api.listGitHubIssues(m.githubOwner, m.githubRepo)
	}
	m.githubTasks = m.api.listGitHubTasks()
	m.githubPRTable = m.newGitHubPRTable()
	m.githubIssueTable = m.newGitHubIssueTable()
	m.githubTaskTable = m.newGitHubTaskTable()
}

func (m *Model) enterDiscoveredTable() {
	m.discoveredData = m.api.listDiscoveredModels()
	m.discoveredTable = m.newDiscoveredTable()
	m.route = routeDiscovered
}

func (m *Model) enterSupervisorTable() {
	m.escalationsData = m.api.listEscalations()
	m.supervisorTable = m.newSupervisorTable()
	m.route = routeSupervisor
}

func (m *Model) enterRouterTable() {
	m.routerData = m.api.listRouterCategories()
	m.rankingsData, _ = m.api.listModelRankings()
	m.routerTable = m.newRouterTable()
	m.route = routeRouter
}

func (m *Model) enterSettingsTable() {
	m.settingsTable = m.newSettingsTable()
	m.route = routeSettings
}

func (m *Model) enterVaultTable() {
	m.vaultData = m.api.listVaultEntries()
	m.vaultTable = m.newVaultTable()
	m.route = routeVault
}

func (m *Model) enterUsageTable() {
	m.usageData = m.api.getUsageSummary()
	m.usageTable = m.newUsageTable()
	m.route = routeUsage
}

func (m *Model) enterNotificationsTable() {
	m.notificationsData = m.api.listNotifications()
	m.notificationsTable = m.newNotificationsTable()
	m.route = routeNotifications
}

func (m *Model) enterProviderKeysTable(providerID string) {
	m.providerKeysID = providerID
	m.providerKeysData = m.api.getPoolConfig(providerID)
	m.providerKeysTable = m.newProviderKeysTable()
	m.route = routeProviderKeys
}

func (m *Model) enterRoomChatScreen(roomID, roomName string) {
	m.roomChatID = roomID
	m.roomChatName = roomName
	m.roomMessages = m.api.listRoomMessages(roomID)
	m.route = routeRoomChat
}

func (m *Model) enterHomeScreen() {
	m.homeData = m.api.getHomeDashboard()
	m.homeSessionTable = m.newHomeSessionTable()
	m.route = routeHome
}

// ─── Refresh ──────────────────────────────────────────────────────────────────

func (m *Model) refreshActiveList() {
	switch m.route {
	case routeAgents:
		m.agentsData = m.api.listAgents()
		m.agentsList.SetItems(buildAgentItems(m.agentsData))
	case routeTools:
		m.toolsData = m.api.listTools()
		m.toolsList.SetItems(buildToolItems(m.toolsData))
	case routeProjects:
		m.projectsData = m.api.listProjects()
		m.projectsList.SetItems(buildProjectItems(m.projectsData))
	case routeSessions:
		m.sessionsData = m.api.listSessions()
		m.sessionsTable = m.newSessionsTable()
	case routeProviders:
		m.providersData = m.api.listProviders()
		m.providersTable = m.newProvidersTable()
	case routeVoice:
		m.voiceData = m.api.listVoiceProviders()
		m.voiceTable = m.newVoiceTable()
	case routeMedia:
		m.mediaData = m.api.listMediaProviders()
		m.mediaTable = m.newMediaTable()
	case routeModels:
		m.modelsData = m.api.listModelHub()
		m.modelsTable = m.newModelsTable()
	case routeWorkers:
		m.workersAgents = m.api.listDaemonAgents()
		m.workersTasks = m.api.listDaemonTasks()
		m.workersPlans = m.api.listDaemonPlans()
		m.workersAgentTable = m.newWorkersAgentTable()
		m.workersPlanTable = m.newWorkersPlanTable()
		m.tasksData = m.api.listTasks("")
		m.tasksTable = m.newTasksTable()
	case routeTasks:
		m.tasksData = m.api.listTasks("")
		m.tasksTable = m.newTasksTable()
	case routeCron:
		m.cronData = m.api.listCronJobs()
		m.cronTable = m.newCronTable()
	case routeMCP:
		m.mcpData = m.api.listMCPServers()
		m.mcpTable = m.newMCPTable()
	case routeRooms:
		m.roomsData = m.api.listRooms()
		m.roomsTable = m.newRoomsTable()
	case routeChannels:
		m.channelsData = m.api.listChannels()
		m.channelsTable = m.newChannelsTable()
	case routeSkills:
		m.skillsData = m.api.listSkills()
		m.skillsTable = m.newSkillsTable()
	case routePlans:
		m.plansData = m.api.listPlans()
		m.plansTable = m.newPlansTable()
	case routeMemory:
		m.memoryData = m.api.searchMemory(m.memoryQuery)
		m.memoryTable = m.newMemoryTable()
	case routeDrive:
		m.driveData = m.api.listDriveFiles()
		m.driveTable = m.newDriveTable()
	case routeDiscovered:
		m.discoveredData = m.api.listDiscoveredModels()
		m.discoveredTable = m.newDiscoveredTable()
	case routeSupervisor:
		m.escalationsData = m.api.listEscalations()
		m.supervisorTable = m.newSupervisorTable()
	case routeRouter:
		m.routerData = m.api.listRouterCategories()
		m.rankingsData, _ = m.api.listModelRankings()
		m.routerTable = m.newRouterTable()
	case routeSettings:
		m.settingsTable = m.newSettingsTable()
	case routeWorkflows:
		m.workflowsData = m.api.listWorkflows()
		m.workflowsTable = m.newWorkflowsTable()
	case routeGitHub:
		m.refreshGitHubData()
	case routeVault:
		m.vaultData = m.api.listVaultEntries()
		m.vaultTable = m.newVaultTable()
	case routeUsage:
		m.usageData = m.api.getUsageSummary()
		m.usageTable = m.newUsageTable()
	case routeNotifications:
		m.notificationsData = m.api.listNotifications()
		m.notificationsTable = m.newNotificationsTable()
	case routeProviderKeys:
		m.providerKeysData = m.api.getPoolConfig(m.providerKeysID)
		m.providerKeysTable = m.newProviderKeysTable()
	case routeRoomChat:
		m.roomMessages = m.api.listRoomMessages(m.roomChatID)
	case routeHome:
		m.homeData = m.api.getHomeDashboard()
		m.homeSessionTable = m.newHomeSessionTable()
	}
}
