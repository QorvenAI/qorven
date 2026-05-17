// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tui

import "encoding/json"

// ── Tasks ─────────────────────────────────────────────────────────────────────

type TaskInfo struct {
	ID         string // truncated for display
	FullID     string
	Title      string
	Status     string
	Priority   string
	AssignedTo string
	UpdatedAt  string
}

func (a *apiClient) listTasks(agentID string) []TaskInfo {
	path := "/v1/tasks"
	if agentID != "" {
		path += "?agent_id=" + agentID
	}
	data, err := a.http.Get(path)
	if err != nil {
		return nil
	}
	var resp struct {
		Tasks []map[string]any `json:"tasks"`
	}
	json.Unmarshal(data, &resp)
	out := make([]TaskInfo, 0, len(resp.Tasks))
	for _, t := range resp.Tasks {
		fullID := strField(t, "id")
		id := fullID
		if len(id) > 8 {
			id = id[:8]
		}
		assigned := strField(t, "assigned_to")
		if len(assigned) > 8 {
			assigned = assigned[:8]
		}
		updated := strField(t, "updated_at")
		if len(updated) > 10 {
			updated = updated[:10]
		}
		priorityLabel := strField(t, "priority")
		switch priorityLabel {
		case "1":
			priorityLabel = "lowest"
		case "2":
			priorityLabel = "low"
		case "3":
			priorityLabel = "medium"
		case "4":
			priorityLabel = "high"
		case "5":
			priorityLabel = "critical"
		}
		out = append(out, TaskInfo{
			ID:         id,
			FullID:     fullID,
			Title:      strField(t, "title"),
			Status:     strField(t, "status"),
			Priority:   priorityLabel,
			AssignedTo: assigned,
			UpdatedAt:  updated,
		})
	}
	return out
}

func (a *apiClient) cancelTask(id string) error {
	_, err := a.http.Post("/v1/tasks/"+id+"/cancel", nil)
	return err
}

// ── Cron jobs ─────────────────────────────────────────────────────────────────

type CronInfo struct {
	ID         string // truncated for display
	FullID     string
	AgentID    string
	Expression string
	Task       string
	Enabled    string
	LastRun    string
}

func (a *apiClient) listCronJobs() []CronInfo {
	data, err := a.http.Get("/v1/cron-jobs")
	if err != nil {
		return nil
	}
	var resp struct {
		Jobs []map[string]any `json:"jobs"`
	}
	json.Unmarshal(data, &resp)
	list := resp.Jobs
	out := make([]CronInfo, 0, len(list))
	for _, j := range list {
		fullID := strField(j, "id")
		id := fullID
		if len(id) > 8 {
			id = id[:8]
		}
		agent := strField(j, "agent_id")
		if len(agent) > 8 {
			agent = agent[:8]
		}
		task := strField(j, "name")
		if len(task) > 30 {
			task = task[:30] + "…"
		}
		lastRun := strField(j, "last_run_at")
		if len(lastRun) > 16 {
			lastRun = lastRun[:16]
		}
		enabled := "no"
		if e, ok := j["enabled"].(bool); ok && e {
			enabled = "yes"
		}
		out = append(out, CronInfo{
			ID:         id,
			FullID:     fullID,
			AgentID:    agent,
			Expression: strField(j, "cron_expression"),
			Task:       task,
			Enabled:    enabled,
			LastRun:    lastRun,
		})
	}
	return out
}

func (a *apiClient) toggleCronJob(id string) error {
	_, err := a.http.Post("/v1/cron-jobs/"+id+"/toggle", nil)
	return err
}

func (a *apiClient) deleteCronJob(id string) error {
	_, err := a.http.Delete("/v1/cron-jobs/" + id)
	return err
}

// ── Plans ─────────────────────────────────────────────────────────────────────

type PlanInfo struct {
	ID        string // truncated for display
	FullID    string
	Title     string
	Status    string
	CreatedAt string
}

func (a *apiClient) listPlans() []PlanInfo {
	data, err := a.http.Get("/v1/plans")
	if err != nil {
		return nil
	}
	var resp struct {
		Plans []map[string]any `json:"plans"`
	}
	json.Unmarshal(data, &resp)
	list := resp.Plans
	out := make([]PlanInfo, 0, len(list))
	for _, p := range list {
		fullID := strField(p, "id")
		id := fullID
		if len(id) > 8 {
			id = id[:8]
		}
		created := strField(p, "created_at")
		if len(created) > 10 {
			created = created[:10]
		}
		out = append(out, PlanInfo{
			ID:        id,
			FullID:    fullID,
			Title:     strField(p, "title"),
			Status:    strField(p, "status"),
			CreatedAt: created,
		})
	}
	return out
}

func (a *apiClient) approvePlan(id string) error {
	_, err := a.http.Post("/v1/plans/"+id+"/approve", nil)
	return err
}

func (a *apiClient) rejectPlan(id string) error {
	_, err := a.http.Post("/v1/plans/"+id+"/reject", nil)
	return err
}

// ── Memory ────────────────────────────────────────────────────────────────────

type MemoryResult struct {
	Score   float64
	Type    string
	Content string
}

func (a *apiClient) searchMemory(q string) []MemoryResult {
	if q == "" {
		return a.listRecentMemory()
	}
	data, err := a.http.Post("/v1/memory/search", map[string]any{"query": q, "max_results": 20})
	if err != nil {
		return nil
	}
	return parseMemoryResults(data)
}

func (a *apiClient) listRecentMemory() []MemoryResult {
	data, err := a.http.Post("/v1/memory/search", map[string]any{"query": "*", "max_results": 20})
	if err != nil {
		data, err = a.http.Get("/v1/memory/search?q=*&max_results=20")
		if err != nil {
			return nil
		}
	}
	return parseMemoryResults(data)
}

// parseMemoryResults decodes the {memories:[{memory:{...},score:float},...]} shape.
func parseMemoryResults(data json.RawMessage) []MemoryResult {
	var resp struct {
		Memories []struct {
			Memory struct {
				Content string `json:"content"`
				Type    string `json:"memory_type"`
			} `json:"memory"`
			Score float64 `json:"score"`
		} `json:"memories"`
	}
	if json.Unmarshal(data, &resp) != nil {
		return nil
	}
	out := make([]MemoryResult, 0, len(resp.Memories))
	for _, r := range resp.Memories {
		content := r.Memory.Content
		if len(content) > 80 {
			content = content[:79] + "…"
		}
		out = append(out, MemoryResult{Score: r.Score, Type: r.Memory.Type, Content: content})
	}
	return out
}

// ── Drive ─────────────────────────────────────────────────────────────────────

type DriveFileInfo struct {
	ID        string // truncated for display
	FullID    string
	Name      string
	Kind      string
	Size      string
	UpdatedAt string
}

func (a *apiClient) listDriveFiles() []DriveFileInfo {
	data, err := a.http.Get("/v1/drive/files")
	if err != nil {
		return nil
	}
	var list []map[string]any
	json.Unmarshal(data, &list)
	out := make([]DriveFileInfo, 0, len(list))
	for _, f := range list {
		fullID := strField(f, "id")
		displayID := fullID
		if len(displayID) > 8 {
			displayID = displayID[:8]
		}
		updated := strField(f, "updated_at")
		if len(updated) > 10 {
			updated = updated[:10]
		}
		out = append(out, DriveFileInfo{
			ID:        displayID,
			FullID:    fullID,
			Name:      strField(f, "name"),
			Kind:      strField(f, "mime_type"),
			Size:      strField(f, "size_bytes"),
			UpdatedAt: updated,
		})
	}
	return out
}

func (a *apiClient) deleteDriveFile(id string) error {
	_, err := a.http.Delete("/v1/drive/files/" + id)
	return err
}

// ── Projects ──────────────────────────────────────────────────────────────────

type ProjectInfo struct {
	ID          string
	Name        string
	DisplayName string
	Path        string
	Phase       string
	Files       int
	GitHubOwner string
	GitHubRepo  string
}

func (a *apiClient) listProjects() []ProjectInfo {
	data, err := a.http.Get("/v1/projects")
	if err != nil {
		return nil
	}
	var list []struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		DisplayName string `json:"display_name"`
		Path        string `json:"path"`
		BuildPhase  string `json:"build_phase"`
		GitHubOwner string `json:"github_owner"`
		GitHubRepo  string `json:"github_repo"`
	}
	if json.Unmarshal(data, &list) != nil {
		return nil
	}
	var projects []ProjectInfo
	for _, p := range list {
		name := p.DisplayName
		if name == "" {
			name = p.Name
		}
		path := p.Path
		if len(path) > 35 {
			path = "..." + path[len(path)-32:]
		}
		phase := p.BuildPhase
		if phase == "" {
			phase = "ready"
		}
		id := p.ID
		if len(id) > 12 {
			id = id[:12]
		}
		projects = append(projects, ProjectInfo{
			ID: id, Name: p.Name, DisplayName: name,
			Path: path, Phase: phase,
			GitHubOwner: p.GitHubOwner, GitHubRepo: p.GitHubRepo,
		})
	}
	return projects
}
