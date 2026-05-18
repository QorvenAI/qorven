// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tui

import (
	"encoding/json"
	"fmt"
)

// ── Daemon / multi-agent workers ─────────────────────────────────────────────

type DaemonAgentInfo struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Provider     string   `json:"provider"`
	Model        string   `json:"model"`
	Status       string   `json:"status"`
	Capabilities []string `json:"capabilities"`
}

type DaemonTaskInfo struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Owner    string `json:"owner"`
	Priority string `json:"priority"`
	Status   string `json:"status"`
	Error    string `json:"error"`
}

type DaemonPlanInfo struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	ProposedBy  string `json:"proposed_by"`
	Status      string `json:"status"`
	Description string `json:"description"`
}

func (a *apiClient) listDaemonAgents() []DaemonAgentInfo {
	data, err := a.http.Get("/v1/daemon/agents")
	if err != nil {
		return nil
	}
	var resp struct {
		Agents []DaemonAgentInfo `json:"agents"`
	}
	json.Unmarshal(data, &resp)
	return resp.Agents
}

func (a *apiClient) listDaemonTasks() []DaemonTaskInfo {
	data, err := a.http.Get("/v1/daemon/tasks")
	if err != nil {
		return nil
	}
	var resp struct {
		Tasks []DaemonTaskInfo `json:"tasks"`
	}
	json.Unmarshal(data, &resp)
	return resp.Tasks
}

func (a *apiClient) listDaemonPlans() []DaemonPlanInfo {
	data, err := a.http.Get("/v1/daemon/plans")
	if err != nil {
		return nil
	}
	var resp struct {
		Plans []DaemonPlanInfo `json:"plans"`
	}
	json.Unmarshal(data, &resp)
	return resp.Plans
}

func (a *apiClient) approveDaemonPlan(id string) error {
	_, err := a.http.Post("/v1/daemon/plans/"+id+"/approve", map[string]string{})
	return err
}

func (a *apiClient) rejectDaemonPlan(id string) error {
	_, err := a.http.Post("/v1/daemon/plans/"+id+"/reject", map[string]string{})
	return err
}

// ── Supervisor ────────────────────────────────────────────────────────────────

type EscalationInfo struct {
	ID        string // truncated for display
	FullID    string
	AgentID   string
	Message   string
	Severity  string
	Status    string
	CreatedAt string
}

func (a *apiClient) listEscalations() []EscalationInfo {
	data, err := a.http.Get("/v1/supervisor/escalations")
	if err != nil {
		return nil
	}
	var resp struct {
		Escalations []map[string]any `json:"escalations"`
	}
	json.Unmarshal(data, &resp)
	out := make([]EscalationInfo, 0, len(resp.Escalations))
	for _, e := range resp.Escalations {
		fullID := strField(e, "id")
		id := fullID
		if len(id) > 8 {
			id = id[:8]
		}
		agent := strField(e, "from") // supervisor.Message uses "from" not "agent_id"
		if len(agent) > 8 {
			agent = agent[:8]
		}
		msg := strField(e, "content") // supervisor.Message uses "content" not "message"
		if len(msg) > 40 {
			msg = msg[:40] + "…"
		}
		created := strField(e, "timestamp") // supervisor.Message uses "timestamp" not "created_at"
		if len(created) > 10 {
			created = created[:10]
		}
		out = append(out, EscalationInfo{
			ID:        id,
			FullID:    fullID,
			AgentID:   agent,
			Message:   msg,
			Severity:  strField(e, "risk"),   // "risk" not "severity"
			Status:    strField(e, "intent"), // use intent as status label
			CreatedAt: created,
		})
	}
	return out
}

func (a *apiClient) approveEscalation(id, reason string) error {
	_, err := a.http.Post("/v1/supervisor/escalations/"+id+"/approve", map[string]string{"reason": reason})
	return err
}

func (a *apiClient) rejectEscalation(id, reason string) error {
	_, err := a.http.Post("/v1/supervisor/escalations/"+id+"/reject", map[string]string{"reason": reason})
	return err
}

// ── Vault ─────────────────────────────────────────────────────────────────────

type VaultEntry struct {
	ID          string // truncated for display
	FullID      string
	Name        string
	Kind        string // "secret", "note", "credential"
	Description string
	UpdatedAt   string
}

func (a *apiClient) listVaultEntries() []VaultEntry {
	data, err := a.http.Get("/v1/connections")
	if err != nil {
		return nil
	}
	var resp struct {
		Connections []map[string]any `json:"connections"`
	}
	if json.Unmarshal(data, &resp) != nil {
		return nil
	}
	out := make([]VaultEntry, 0, len(resp.Connections))
	for _, c := range resp.Connections {
		fullID := strField(c, "platform_id")
		displayID := fullID
		if len(displayID) > 8 {
			displayID = displayID[:8]
		}
		updated := strField(c, "updated_at")
		if len(updated) > 10 {
			updated = updated[:10]
		}
		out = append(out, VaultEntry{
			ID:          displayID,
			FullID:      fullID,
			Name:        strField(c, "label"),
			Kind:        strField(c, "auth_type"),
			Description: strField(c, "platform_id"),
			UpdatedAt:   updated,
		})
	}
	return out
}

func (a *apiClient) createVaultEntry(name, kind, value, description string) error {
	_, err := a.http.Post("/v1/connections/"+name, map[string]any{
		"api_key": value,
		"label":   description,
	})
	return err
}

func (a *apiClient) deleteVaultEntry(id string) error {
	_, err := a.http.Delete("/v1/connections/" + id)
	return err
}

// ── Notifications ─────────────────────────────────────────────────────────────

type NotificationInfo struct {
	ID        string // truncated for display
	FullID    string
	Kind      string
	Title     string
	Body      string
	Read      bool
	CreatedAt string
}

func (a *apiClient) listNotifications() []NotificationInfo {
	data, err := a.http.Get("/v1/notifications")
	if err != nil {
		return nil
	}
	var resp struct {
		Notifications []map[string]any `json:"notifications"`
	}
	if json.Unmarshal(data, &resp) != nil {
		return nil
	}
	out := make([]NotificationInfo, 0, len(resp.Notifications))
	for _, n := range resp.Notifications {
		fullID := strField(n, "id")
		id := fullID
		if len(id) > 8 {
			id = id[:8]
		}
		body := strField(n, "body")
		if len(body) > 50 {
			body = body[:50] + "…"
		}
		created := strField(n, "created_at")
		if len(created) > 10 {
			created = created[:10]
		}
		read, _ := n["read"].(bool)
		out = append(out, NotificationInfo{
			ID:        id,
			FullID:    fullID,
			Kind:      strField(n, "kind"),
			Title:     strField(n, "title"),
			Body:      body,
			Read:      read,
			CreatedAt: created,
		})
	}
	return out
}

func (a *apiClient) markNotificationRead(id string) error {
	_, err := a.http.Post("/v1/notifications/"+id+"/read", nil)
	return err
}

// ── Usage / Token spend ───────────────────────────────────────────────────────

type UsageSummary struct {
	TotalCostUSD float64
	ByAgent      []AgentUsage
}

type AgentUsage struct {
	AgentName    string
	CostUSD      float64
	SessionCount int
}

func (a *apiClient) getUsageSummary() UsageSummary {
	data, err := a.http.Get("/v1/usage/account")
	if err != nil {
		return UsageSummary{}
	}
	var resp struct {
		TotalCostThisMonth float64          `json:"total_cost_this_month"`
		Souls              []map[string]any `json:"souls"`
	}
	if json.Unmarshal(data, &resp) != nil {
		return UsageSummary{}
	}
	agents := make([]AgentUsage, 0, len(resp.Souls))
	for _, s := range resp.Souls {
		cost, _ := s["cost"].(float64)
		calls, _ := s["calls"].(float64)
		agents = append(agents, AgentUsage{
			AgentName:    strField(s, "name"),
			CostUSD:      cost,
			SessionCount: int(calls),
		})
	}
	return UsageSummary{
		TotalCostUSD: resp.TotalCostThisMonth,
		ByAgent:      agents,
	}
}

// ── Home dashboard ────────────────────────────────────────────────────────────

type homeDashboard struct {
	PendingPlans        int
	PendingEscalations  int
	ActiveTasks         int
	ActiveDaemonAgents  int
	UnreadNotifications int
	RecentSessions      []SessionInfo
	SystemStatus        string
	GatewayVersion      string
}

func (a *apiClient) getHomeDashboard() homeDashboard {
	dash := homeDashboard{SystemStatus: "online"}

	status := a.getStatus()
	dash.SystemStatus = status["status"]
	dash.GatewayVersion = status["version"]

	plans := a.listPlans()
	for _, p := range plans {
		if p.Status == "pending" {
			dash.PendingPlans++
		}
	}

	escalations := a.listEscalations()
	for _, e := range escalations {
		if e.Status == "pending" {
			dash.PendingEscalations++
		}
	}

	tasks := a.listTasks("")
	for _, t := range tasks {
		if t.Status == "in_progress" || t.Status == "running" {
			dash.ActiveTasks++
		}
	}

	agents := a.listDaemonAgents()
	for _, ag := range agents {
		if ag.Status == "running" || ag.Status == "active" {
			dash.ActiveDaemonAgents++
		}
	}

	notifs := a.listNotifications()
	for _, n := range notifs {
		if !n.Read {
			dash.UnreadNotifications++
		}
	}

	sessions := a.listSessions()
	if len(sessions) > 5 {
		sessions = sessions[:5]
	}
	dash.RecentSessions = sessions

	return dash
}

// ── GitHub ────────────────────────────────────────────────────────────────────

type ghPR struct {
	Number     int
	Title      string
	HeadBranch string
	CIStatus   string
	Author     string
	State      string
	HTMLURL    string
	UpdatedAt  string
}

type ghIssue struct {
	Number    int
	Title     string
	Labels    []string
	State     string
	Assignee  string
	HTMLURL   string
	UpdatedAt string
}

type ghTask struct {
	ID        string `json:"id"`
	Owner     string `json:"owner"`
	Repo      string `json:"repo"`
	Branch    string `json:"branch"`
	Phase     string `json:"phase"`
	PRNumber  int    `json:"pr_number"`
	PRURL     string `json:"pr_url"`
	AgentID   string `json:"agent_id"`
	UpdatedAt string `json:"updated_at"`
}

func (a *apiClient) listGitHubPRs(owner, repo string) []ghPR {
	data, err := a.http.Get(fmt.Sprintf("/v1/github/%s/%s/pulls?state=open&limit=30", owner, repo))
	if err != nil {
		return nil
	}
	var resp struct {
		Pulls []struct {
			Number    int    `json:"number"`
			Title     string `json:"title"`
			State     string `json:"state"`
			HTMLURL   string `json:"html_url"`
			Head      struct{ Ref string `json:"ref"` } `json:"head"`
			User      struct{ Login string `json:"login"` } `json:"user"`
			UpdatedAt string `json:"updated_at"`
		} `json:"pulls"`
	}
	json.Unmarshal(data, &resp)
	out := make([]ghPR, 0, len(resp.Pulls))
	for _, p := range resp.Pulls {
		out = append(out, ghPR{
			Number:     p.Number,
			Title:      p.Title,
			HeadBranch: p.Head.Ref,
			CIStatus:   "pending",
			Author:     p.User.Login,
			State:      p.State,
			HTMLURL:    p.HTMLURL,
			UpdatedAt:  shortAge(p.UpdatedAt),
		})
	}
	return out
}

func (a *apiClient) listGitHubIssues(owner, repo string) []ghIssue {
	data, err := a.http.Get(fmt.Sprintf("/v1/github/%s/%s/issues?state=open&limit=50", owner, repo))
	if err != nil {
		return nil
	}
	var resp struct {
		Issues []struct {
			Number    int    `json:"number"`
			Title     string `json:"title"`
			State     string `json:"state"`
			HTMLURL   string `json:"html_url"`
			Assignee  *struct{ Login string `json:"login"` } `json:"assignee"`
			Labels    []struct{ Name string `json:"name"` } `json:"labels"`
			UpdatedAt string `json:"updated_at"`
		} `json:"issues"`
	}
	json.Unmarshal(data, &resp)
	out := make([]ghIssue, 0, len(resp.Issues))
	for _, i := range resp.Issues {
		labels := make([]string, 0, len(i.Labels))
		for _, l := range i.Labels {
			labels = append(labels, l.Name)
		}
		assignee := ""
		if i.Assignee != nil {
			assignee = i.Assignee.Login
		}
		out = append(out, ghIssue{
			Number:    i.Number,
			Title:     i.Title,
			Labels:    labels,
			State:     i.State,
			Assignee:  assignee,
			HTMLURL:   i.HTMLURL,
			UpdatedAt: shortAge(i.UpdatedAt),
		})
	}
	return out
}

func (a *apiClient) listGitHubTasks() []ghTask {
	data, err := a.http.Get("/v1/github/tasks")
	if err != nil {
		return nil
	}
	var resp struct {
		Tasks []ghTask `json:"tasks"`
	}
	json.Unmarshal(data, &resp)
	return resp.Tasks
}

func (a *apiClient) mergeGitHubPR(owner, repo string, prNum int) error {
	_, err := a.http.Post(fmt.Sprintf("/v1/github/%s/%s/pulls/%d/merge", owner, repo, prNum),
		map[string]string{"merge_method": "squash"})
	return err
}

func (a *apiClient) closeGitHubIssue(owner, repo string, num int) error {
	_, err := a.http.Post(fmt.Sprintf("/v1/github/%s/%s/issues/%d/close", owner, repo, num), nil)
	return err
}
