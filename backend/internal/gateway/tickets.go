// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/qorvenai/qorven/internal/realtime"
)

// Ticket is a work item assigned to an agent.
type Ticket struct {
	ID              string    `json:"id"`
	TenantID        string    `json:"tenant_id"`
	Slug            string    `json:"slug"`
	Title           string    `json:"title"`
	Description     string    `json:"description"`
	Status          string    `json:"status"`
	Priority        string    `json:"priority"`
	AssignedAgentID *string   `json:"assigned_agent_id"`
	GoalID          *string   `json:"goal_id"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// TicketComment is a comment on a ticket, from a user or agent.
type TicketComment struct {
	ID         string    `json:"id"`
	TicketID   string    `json:"ticket_id"`
	AuthorType string    `json:"author_type"`
	AuthorID   string    `json:"author_id"`
	Body       string    `json:"body"`
	CreatedAt  time.Time `json:"created_at"`
}

// TicketFile records a file touched by an agent while working a ticket.
type TicketFile struct {
	ID        string    `json:"id"`
	TicketID  string    `json:"ticket_id"`
	Path      string    `json:"path"`
	Operation string    `json:"operation"`
	TouchedAt time.Time `json:"touched_at"`
}

// nextTicketSlug atomically increments the per-tenant counter and returns T-N.
func (gw *Gateway) nextTicketSlug(ctx context.Context, tenantID string) (string, error) {
	var n int64
	err := gw.db.Pool.QueryRow(ctx,
		`INSERT INTO ticket_counters (tenant_id, next_val) VALUES ($1, 2)
		 ON CONFLICT (tenant_id) DO UPDATE SET next_val = ticket_counters.next_val + 1
		 RETURNING next_val - 1`,
		tenantID).Scan(&n)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("T-%d", n), nil
}

func (gw *Gateway) handleListTickets(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 200, []Ticket{})
		return
	}
	q := r.URL.Query()
	status := q.Get("status")
	priority := q.Get("priority")
	agentID := q.Get("agent_id")
	goalID := q.Get("goal_id")
	search := q.Get("q")

	rows, err := gw.db.Pool.Query(r.Context(),
		`SELECT id, tenant_id, slug, title, description, status, priority,
		        assigned_agent_id, goal_id, created_at, updated_at
		 FROM tickets
		 WHERE tenant_id = $1
		   AND ($2 = '' OR status   = $2)
		   AND ($3 = '' OR priority = $3)
		   AND ($4 = '' OR assigned_agent_id::text = $4)
		   AND ($5 = '' OR goal_id::text = $5)
		   AND ($6 = '' OR title ILIKE '%' || $6 || '%' OR description ILIKE '%' || $6 || '%')
		 ORDER BY created_at DESC LIMIT 50`,
		defaultTenant, status, priority, agentID, goalID, search)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	list := []Ticket{}
	for rows.Next() {
		var t Ticket
		if err := rows.Scan(&t.ID, &t.TenantID, &t.Slug, &t.Title, &t.Description,
			&t.Status, &t.Priority, &t.AssignedAgentID, &t.GoalID,
			&t.CreatedAt, &t.UpdatedAt); err != nil {
			continue
		}
		list = append(list, t)
	}
	if list == nil {
		list = []Ticket{}
	}
	writeJSON(w, 200, list)
}

func (gw *Gateway) handleCreateTicket(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	var body struct {
		Title       string  `json:"title"`
		Description string  `json:"description"`
		Priority    string  `json:"priority"`
		GoalID      *string `json:"goal_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Title == "" {
		writeJSON(w, 400, map[string]string{"error": "title required"})
		return
	}
	if body.Priority == "" {
		body.Priority = "normal"
	}
	slug, err := gw.nextTicketSlug(r.Context(), defaultTenant)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "slug generation failed: " + err.Error()})
		return
	}
	var t Ticket
	err = gw.db.Pool.QueryRow(r.Context(),
		`INSERT INTO tickets (tenant_id, slug, title, description, priority, goal_id)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, tenant_id, slug, title, description, status, priority,
		           assigned_agent_id, goal_id, created_at, updated_at`,
		defaultTenant, slug, body.Title, body.Description, body.Priority, body.GoalID).
		Scan(&t.ID, &t.TenantID, &t.Slug, &t.Title, &t.Description,
			&t.Status, &t.Priority, &t.AssignedAgentID, &t.GoalID,
			&t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, t)
}

func (gw *Gateway) handleUpdateTicket(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	id := chi.URLParam(r, "id")
	var body struct {
		Title       *string `json:"title"`
		Description *string `json:"description"`
		Status      *string `json:"status"`
		Priority    *string `json:"priority"`
		GoalID      *string `json:"goal_id"`
	}
	json.NewDecoder(r.Body).Decode(&body) //nolint:errcheck
	var t Ticket
	err := gw.db.Pool.QueryRow(r.Context(),
		`UPDATE tickets SET
		   title       = COALESCE($2, title),
		   description = COALESCE($3, description),
		   status      = COALESCE($4, status),
		   priority    = COALESCE($5, priority),
		   goal_id     = CASE WHEN $6::text IS NOT NULL THEN $6::uuid ELSE goal_id END,
		   updated_at  = NOW()
		 WHERE id = $1 AND tenant_id = $7
		 RETURNING id, tenant_id, slug, title, description, status, priority,
		           assigned_agent_id, goal_id, created_at, updated_at`,
		id, body.Title, body.Description, body.Status, body.Priority, body.GoalID, defaultTenant).
		Scan(&t.ID, &t.TenantID, &t.Slug, &t.Title, &t.Description,
			&t.Status, &t.Priority, &t.AssignedAgentID, &t.GoalID,
			&t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	gw.rtHub.Broadcast(realtime.Event{Type: realtime.EventTicketUpdated, Data: t})
	writeJSON(w, 200, t)
}

func (gw *Gateway) handleDeleteTicket(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	id := chi.URLParam(r, "id")
	_, err := gw.db.Pool.Exec(r.Context(),
		`DELETE FROM tickets WHERE id = $1 AND tenant_id = $2`, id, defaultTenant)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]string{"status": "deleted", "id": id})
}

func (gw *Gateway) handleAssignTicket(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	id := chi.URLParam(r, "id")
	var body struct {
		AgentID string `json:"agent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.AgentID == "" {
		writeJSON(w, 400, map[string]string{"error": "agent_id required"})
		return
	}
	var t Ticket
	err := gw.db.Pool.QueryRow(r.Context(),
		`UPDATE tickets SET assigned_agent_id = $2, status = 'in_progress', updated_at = NOW()
		 WHERE id = $1 AND tenant_id = $3
		 RETURNING id, tenant_id, slug, title, description, status, priority,
		           assigned_agent_id, goal_id, created_at, updated_at`,
		id, body.AgentID, defaultTenant).
		Scan(&t.ID, &t.TenantID, &t.Slug, &t.Title, &t.Description,
			&t.Status, &t.Priority, &t.AssignedAgentID, &t.GoalID,
			&t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	_, err = gw.db.Pool.Exec(r.Context(),
		`INSERT INTO heartbeat_queue (tenant_id, agent_id, trigger, context_type, context_id)
		 VALUES ($1, $2, 'ticket_assigned', 'ticket', $3)`,
		defaultTenant, body.AgentID, id)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "queue failed: " + err.Error()})
		return
	}
	gw.rtHub.Broadcast(realtime.Event{Type: realtime.EventTicketUpdated, Data: t})
	writeJSON(w, 200, map[string]any{"ticket": t, "heartbeat": "queued"})
}

func (gw *Gateway) handleListTicketComments(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 200, []TicketComment{})
		return
	}
	id := chi.URLParam(r, "id")
	rows, err := gw.db.Pool.Query(r.Context(),
		`SELECT id, ticket_id, author_type, author_id, body, created_at
		 FROM ticket_comments WHERE ticket_id = $1 ORDER BY created_at`, id)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	comments := []TicketComment{}
	for rows.Next() {
		var c TicketComment
		if err := rows.Scan(&c.ID, &c.TicketID, &c.AuthorType, &c.AuthorID, &c.Body, &c.CreatedAt); err != nil {
			continue
		}
		comments = append(comments, c)
	}
	if comments == nil {
		comments = []TicketComment{}
	}
	writeJSON(w, 200, comments)
}

func (gw *Gateway) handleAddTicketComment(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	id := chi.URLParam(r, "id")
	var body struct {
		Body string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Body == "" {
		writeJSON(w, 400, map[string]string{"error": "body required"})
		return
	}
	var c TicketComment
	err := gw.db.Pool.QueryRow(r.Context(),
		`INSERT INTO ticket_comments (ticket_id, author_type, author_id, body)
		 VALUES ($1, 'user', 'user', $2)
		 RETURNING id, ticket_id, author_type, author_id, body, created_at`,
		id, body.Body).
		Scan(&c.ID, &c.TicketID, &c.AuthorType, &c.AuthorID, &c.Body, &c.CreatedAt)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	gw.rtHub.Broadcast(realtime.Event{Type: realtime.EventTicketComment, Data: c})
	writeJSON(w, 200, c)
}

func (gw *Gateway) handleListTicketFiles(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 200, []TicketFile{})
		return
	}
	id := chi.URLParam(r, "id")
	rows, err := gw.db.Pool.Query(r.Context(),
		`SELECT id, ticket_id, path, operation, touched_at
		 FROM ticket_files WHERE ticket_id = $1 ORDER BY touched_at DESC`, id)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	files := []TicketFile{}
	for rows.Next() {
		var f TicketFile
		if err := rows.Scan(&f.ID, &f.TicketID, &f.Path, &f.Operation, &f.TouchedAt); err != nil {
			continue
		}
		files = append(files, f)
	}
	if files == nil {
		files = []TicketFile{}
	}
	writeJSON(w, 200, files)
}
