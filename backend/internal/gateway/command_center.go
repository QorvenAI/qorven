// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"net/http"
	"sort"
	"sync"
	"time"
)

// AgentJobStatus represents the state of a background agent job.
type AgentJobStatus string

const (
	JobStatusQueued     AgentJobStatus = "queued"
	JobStatusRunning    AgentJobStatus = "running"
	JobStatusCompleted  AgentJobStatus = "completed"
	JobStatusFailed     AgentJobStatus = "failed"
	JobStatusCancelled  AgentJobStatus = "cancelled"
)

// AgentJob tracks a background agent execution (build, task, etc.)
type AgentJob struct {
	ID          string         `json:"id"`
	ProjectID   string         `json:"project_id,omitempty"`
	ProjectName string         `json:"project_name,omitempty"`
	AgentID     string         `json:"agent_id"`
	AgentName   string         `json:"agent_name,omitempty"`
	Title       string         `json:"title"`
	Status      AgentJobStatus `json:"status"`
	Progress    int            `json:"progress"`
	StartedAt   *time.Time     `json:"started_at,omitempty"`
	CompletedAt *time.Time     `json:"completed_at,omitempty"`
	DurationMs  int64          `json:"duration_ms,omitempty"`
	CostCents   float64        `json:"cost_cents,omitempty"`
	TokensIn    int            `json:"tokens_in,omitempty"`
	TokensOut   int            `json:"tokens_out,omitempty"`
	Iterations  int            `json:"iterations,omitempty"`
	Error       string         `json:"error,omitempty"`
	Output      string         `json:"output,omitempty"`
}

// CommandCenter manages background agent jobs.
type CommandCenter struct {
	mu   sync.RWMutex
	jobs map[string]*AgentJob
}

func NewCommandCenter() *CommandCenter {
	return &CommandCenter{
		jobs: make(map[string]*AgentJob),
	}
}

// AddJob registers a new job.
func (cc *CommandCenter) AddJob(job *AgentJob) {
	cc.mu.Lock()
	cc.jobs[job.ID] = job
	cc.mu.Unlock()
}

// UpdateJob updates a job's status.
func (cc *CommandCenter) UpdateJob(id string, fn func(*AgentJob)) {
	cc.mu.Lock()
	if j, ok := cc.jobs[id]; ok {
		fn(j)
	}
	cc.mu.Unlock()
}

// GetJob returns a job by ID.
func (cc *CommandCenter) GetJob(id string) *AgentJob {
	cc.mu.RLock()
	defer cc.mu.RUnlock()
	return cc.jobs[id]
}

// ListJobs returns all jobs sorted by start time (newest first).
func (cc *CommandCenter) ListJobs() []*AgentJob {
	cc.mu.RLock()
	defer cc.mu.RUnlock()
	result := make([]*AgentJob, 0, len(cc.jobs))
	for _, j := range cc.jobs {
		result = append(result, j)
	}
	sort.Slice(result, func(i, j int) bool {
		ti := result[i].StartedAt
		tj := result[j].StartedAt
		if ti == nil || tj == nil {
			return ti != nil
		}
		return ti.After(*tj)
	})
	return result
}

// ListByStatus returns jobs filtered by status.
func (cc *CommandCenter) ListByStatus(status AgentJobStatus) []*AgentJob {
	cc.mu.RLock()
	defer cc.mu.RUnlock()
	var result []*AgentJob
	for _, j := range cc.jobs {
		if j.Status == status {
			result = append(result, j)
		}
	}
	return result
}

// Stats returns counts per status.
func (cc *CommandCenter) Stats() map[string]int {
	cc.mu.RLock()
	defer cc.mu.RUnlock()
	stats := map[string]int{
		"queued":    0,
		"running":   0,
		"completed": 0,
		"failed":    0,
	}
	for _, j := range cc.jobs {
		stats[string(j.Status)]++
	}
	return stats
}

// Cleanup removes completed/failed jobs older than maxAge.
func (cc *CommandCenter) Cleanup(maxAge time.Duration) int {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	cutoff := time.Now().Add(-maxAge)
	removed := 0
	for id, j := range cc.jobs {
		if j.Status == JobStatusCompleted || j.Status == JobStatusFailed || j.Status == JobStatusCancelled {
			if j.CompletedAt != nil && j.CompletedAt.Before(cutoff) {
				delete(cc.jobs, id)
				removed++
			}
		}
	}
	return removed
}

// --- HTTP Handlers ---

func (gw *Gateway) handleCommandCenter(w http.ResponseWriter, r *http.Request) {
	if gw.commandCenter == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"jobs":  []any{},
			"stats": map[string]int{"queued": 0, "running": 0, "completed": 0, "failed": 0},
		})
		return
	}

	jobs := gw.commandCenter.ListJobs()
	stats := gw.commandCenter.Stats()

	// Group for kanban view
	kanban := map[string][]*AgentJob{
		"queued":    {},
		"running":   {},
		"completed": {},
		"failed":    {},
	}
	for _, j := range jobs {
		kanban[string(j.Status)] = append(kanban[string(j.Status)], j)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"jobs":   jobs,
		"stats":  stats,
		"kanban": kanban,
	})
}

func (gw *Gateway) handleCommandCenterStats(w http.ResponseWriter, r *http.Request) {
	if gw.commandCenter == nil {
		writeJSON(w, http.StatusOK, map[string]int{"queued": 0, "running": 0, "completed": 0, "failed": 0})
		return
	}
	writeJSON(w, http.StatusOK, gw.commandCenter.Stats())
}
