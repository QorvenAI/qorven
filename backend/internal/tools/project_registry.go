// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// CodeProject represents a user's coding project with its own session, memory, and tasks.
type CodeProject struct {
	ID          string `json:"id"`
	Name        string `json:"name"`         // slug: "go-todo-api"
	DisplayName string `json:"display_name"` // human: "Go Todo API"
	Path        string `json:"path"`
	SessionID string     `json:"session_id"`
	Tasks     []CodeTask `json:"tasks,omitempty"`
	Notes     string     `json:"notes,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`

	// Build pipeline state
	BuildPhase    string   `json:"build_phase,omitempty"`     // plan/pending_approval/spawning/building/testing/pushing/preview/done/failed
	BuildRoomID   string   `json:"build_room_id,omitempty"`
	BuildPlan     string   `json:"build_plan,omitempty"`
	PlanID        string   `json:"plan_id,omitempty"`         // graph runtime plan row id
	SpawnedAgents []string `json:"spawned_agents,omitempty"`
	Description   string   `json:"description,omitempty"`
	Stack         string   `json:"stack,omitempty"`
	PreviewURL    string   `json:"preview_url,omitempty"`

	// GitHub integration — set via POST /v1/projects/:id/github/connect
	GitHubOwner   string `json:"github_owner,omitempty"`
	GitHubRepo    string `json:"github_repo,omitempty"`
	GitHubSecret  string `json:"github_secret,omitempty"` // per-project webhook HMAC secret
	DefaultBranch string `json:"default_branch,omitempty"`

	InceptionBriefID string `json:"inception_brief_id,omitempty"`
	ProjectType      string `json:"project_type,omitempty"` // "qorven_app" triggers auto-install after build
}

// CodeTask is a todo item within a project.
type CodeTask struct {
	ID        string `json:"id"`
	Text      string `json:"text"`
	Done      bool   `json:"done"`
	Priority  string `json:"priority,omitempty"` // high, medium, low
	CreatedAt string `json:"created_at"`
}

// ProjectRegistry manages multiple coding projects.
type ProjectRegistry struct {
	mu       sync.RWMutex
	projects map[string]*CodeProject
	filePath string
}

func NewProjectRegistry(configDir string) *ProjectRegistry {
	fp := filepath.Join(configDir, "projects.json")
	r := &ProjectRegistry{projects: make(map[string]*CodeProject), filePath: fp}
	r.load()
	return r
}

func (r *ProjectRegistry) load() {
	data, err := os.ReadFile(r.filePath)
	if err != nil {
		return
	}
	projects := []*CodeProject{}
	if json.Unmarshal(data, &projects) == nil {
		for _, p := range projects {
			r.projects[p.ID] = p
		}
	}
}

func (r *ProjectRegistry) save() {
	r.mu.RLock()
	projects := make([]*CodeProject, 0, len(r.projects))
	for _, p := range r.projects {
		projects = append(projects, p)
	}
	r.mu.RUnlock()
	sort.Slice(projects, func(i, j int) bool { return projects[i].UpdatedAt.After(projects[j].UpdatedAt) })
	data, _ := json.MarshalIndent(projects, "", "  ")
	os.MkdirAll(filepath.Dir(r.filePath), 0755)
	// Atomic write: temp-in-same-dir + rename so a crash mid-write
	// can't truncate the registry and erase the user's project list.
	writeFileAtomic(r.filePath, data, 0644)
}

// writeFileAtomic writes data via a temp file + rename. Silent-
// failure shape matches the previous os.WriteFile call — registry
// saves shouldn't block the caller, but a torn file on crash is
// worse than a failed save.
func writeFileAtomic(path string, data []byte, perm os.FileMode) {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		// Fallback to direct write — better than no save at all.
		os.WriteFile(path, data, perm)
		return
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close(); os.Remove(tmpPath); return
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close(); os.Remove(tmpPath); return
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath); return
	}
	os.Chmod(tmpPath, perm)
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
	}
}

func (r *ProjectRegistry) Create(name, displayName, path string) *CodeProject {
	r.mu.Lock()
	defer r.mu.Unlock()
	if displayName == "" {
		displayName = name
	}
	id := fmt.Sprintf("proj-%d", time.Now().UnixMilli())
	p := &CodeProject{
		ID: id, Name: name, DisplayName: displayName, Path: path,
		SessionID: fmt.Sprintf("code-%s", id),
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	r.projects[id] = p
	go r.save()
	return p
}

// CreateFromInception creates a project and records the Inception brief ID.
// save() is called synchronously so persistence is guaranteed before the caller reads disk.
func (r *ProjectRegistry) CreateFromInception(name, displayName, path, briefID string) *CodeProject {
	p := r.Create(name, displayName, path)
	r.mu.Lock()
	p.InceptionBriefID = briefID
	r.mu.Unlock()
	r.save()
	return p
}

// GetByBriefID returns the CodeProject linked to briefID, or nil.
func (r *ProjectRegistry) GetByBriefID(briefID string) *CodeProject {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, p := range r.projects {
		if p.InceptionBriefID == briefID {
			return p
		}
	}
	return nil
}

func (r *ProjectRegistry) Get(id string) *CodeProject {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.projects[id]
}

func (r *ProjectRegistry) List() []*CodeProject {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]*CodeProject, 0, len(r.projects))
	for _, p := range r.projects {
		list = append(list, p)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].UpdatedAt.After(list[j].UpdatedAt) })
	return list
}

func (r *ProjectRegistry) Delete(id string) {
	r.mu.Lock()
	delete(r.projects, id)
	r.mu.Unlock()
	go r.save()
}

func (r *ProjectRegistry) AddTask(projectID, text, priority string) *CodeTask {
	r.mu.Lock()
	defer r.mu.Unlock()
	p := r.projects[projectID]
	if p == nil {
		return nil
	}
	t := CodeTask{
		ID: fmt.Sprintf("task-%d", time.Now().UnixMilli()),
		Text: text, Priority: priority, CreatedAt: time.Now().Format(time.RFC3339),
	}
	p.Tasks = append(p.Tasks, t)
	p.UpdatedAt = time.Now()
	go r.save()
	return &t
}

func (r *ProjectRegistry) ToggleTask(projectID, taskID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p := r.projects[projectID]
	if p == nil {
		return
	}
	for i := range p.Tasks {
		if p.Tasks[i].ID == taskID {
			p.Tasks[i].Done = !p.Tasks[i].Done
			break
		}
	}
	p.UpdatedAt = time.Now()
	go r.save()
}

func (r *ProjectRegistry) UpdateNotes(projectID, notes string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p := r.projects[projectID]
	if p == nil {
		return
	}
	p.Notes = notes
	p.UpdatedAt = time.Now()
	go r.save()
}

// UpdateBuild sets build pipeline fields atomically.
func (r *ProjectRegistry) UpdateBuild(projectID string, fn func(p *CodeProject)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p := r.projects[projectID]
	if p == nil {
		return
	}
	fn(p)
	p.UpdatedAt = time.Now()
	go r.save()
}

// ProjectManagerTool exposes project management to the agent.
type ProjectManagerTool struct {
	registry *ProjectRegistry
}

func NewProjectManagerTool(reg *ProjectRegistry) *ProjectManagerTool {
	return &ProjectManagerTool{registry: reg}
}

func (t *ProjectManagerTool) Name() string { return "project_manager" }
func (t *ProjectManagerTool) Description() string {
	return `Manage coding projects — create, list, switch, add tasks, update notes.
Actions: create (name, path), list, get (id), add_task (id, text, priority), toggle_task (id, task_id), notes (id, text), delete (id).`
}
func (t *ProjectManagerTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"action":   map[string]any{"type": "string", "description": "create|list|get|add_task|toggle_task|notes|delete"},
		"id":       map[string]any{"type": "string", "description": "Project ID"},
		"name":     map[string]any{"type": "string", "description": "Project name (for create)"},
		"path":     map[string]any{"type": "string", "description": "Project path (for create)"},
		"text":     map[string]any{"type": "string", "description": "Task text or notes content"},
		"priority": map[string]any{"type": "string", "description": "Task priority: high|medium|low"},
		"task_id":  map[string]any{"type": "string", "description": "Task ID (for toggle_task)"},
	}, "required": []string{"action"}}
}

func (t *ProjectManagerTool) Execute(ctx context.Context, args map[string]any) *Result {
	action, _ := args["action"].(string)
	id, _ := args["id"].(string)

	switch action {
	case "create":
		name, _ := args["name"].(string)
		path, _ := args["path"].(string)
		if name == "" || path == "" {
			return ErrorResult("name and path required")
		}
		p := t.registry.Create(name, "", path)
		data, _ := json.Marshal(p)
		return TextResult(string(data))

	case "list":
		projects := t.registry.List()
		data, _ := json.MarshalIndent(projects, "", "  ")
		return TextResult(string(data))

	case "get":
		p := t.registry.Get(id)
		if p == nil {
			return ErrorResult("project not found")
		}
		data, _ := json.MarshalIndent(p, "", "  ")
		return TextResult(string(data))

	case "add_task":
		text, _ := args["text"].(string)
		priority, _ := args["priority"].(string)
		if priority == "" {
			priority = "medium"
		}
		task := t.registry.AddTask(id, text, priority)
		if task == nil {
			return ErrorResult("project not found")
		}
		return TextResult(fmt.Sprintf("Task added: %s", task.Text))

	case "toggle_task":
		taskID, _ := args["task_id"].(string)
		t.registry.ToggleTask(id, taskID)
		return TextResult("Task toggled")

	case "notes":
		text, _ := args["text"].(string)
		t.registry.UpdateNotes(id, text)
		return TextResult("Notes updated")

	case "delete":
		t.registry.Delete(id)
		return TextResult("Project deleted")

	default:
		return ErrorResult("unknown action: " + action)
	}
}
