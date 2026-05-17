// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package gateway

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

// WorkGoal is a node in the mission/sub-goal tree.
// Table: work_goals (separate from the existing agent KPI "goals" table).
type WorkGoal struct {
	ID          string    `json:"id"`
	TenantID    string    `json:"tenant_id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	ParentID    *string   `json:"parent_id"`
	OrderIndex  int       `json:"order_index"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// WorkGoalTreeNode extends WorkGoal with aggregated children + ticket counts.
type WorkGoalTreeNode struct {
	WorkGoal
	Children    []WorkGoalTreeNode `json:"children"`
	TicketCount int                `json:"ticket_count"`
	DoneCount   int                `json:"done_count"`
}

func (gw *Gateway) handleListWorkGoals(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 200, []WorkGoal{})
		return
	}
	rows, err := gw.db.Pool.Query(r.Context(),
		`SELECT id, tenant_id, title, description, parent_id, order_index, status, created_at, updated_at
		 FROM work_goals WHERE tenant_id = $1 ORDER BY order_index, created_at`, defaultTenant)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	list := []WorkGoal{}
	for rows.Next() {
		var g WorkGoal
		if err := rows.Scan(&g.ID, &g.TenantID, &g.Title, &g.Description, &g.ParentID,
			&g.OrderIndex, &g.Status, &g.CreatedAt, &g.UpdatedAt); err != nil {
			continue
		}
		list = append(list, g)
	}
	if list == nil {
		list = []WorkGoal{}
	}
	writeJSON(w, 200, list)
}

func (gw *Gateway) handleGetWorkGoalTree(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 200, []WorkGoalTreeNode{})
		return
	}
	rows, err := gw.db.Pool.Query(r.Context(),
		`SELECT wg.id, wg.tenant_id, wg.title, wg.description, wg.parent_id,
		        wg.order_index, wg.status, wg.created_at, wg.updated_at,
		        COUNT(t.id)                                        AS ticket_count,
		        COUNT(t.id) FILTER (WHERE t.status = 'done')      AS done_count
		 FROM work_goals wg
		 LEFT JOIN tickets t ON t.goal_id = wg.id
		 WHERE wg.tenant_id = $1
		 GROUP BY wg.id
		 ORDER BY wg.order_index, wg.created_at`, defaultTenant)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	nodes := []WorkGoalTreeNode{}
	byID := map[string]int{} // id → index in nodes
	for rows.Next() {
		var n WorkGoalTreeNode
		n.Children = []WorkGoalTreeNode{}
		if err := rows.Scan(&n.ID, &n.TenantID, &n.Title, &n.Description, &n.ParentID,
			&n.OrderIndex, &n.Status, &n.CreatedAt, &n.UpdatedAt,
			&n.TicketCount, &n.DoneCount); err != nil {
			continue
		}
		byID[n.ID] = len(nodes)
		nodes = append(nodes, n)
	}

	// Build tree in-place: for nodes with a parent, append to parent.Children.
	// We operate on a copy to avoid pointer aliasing issues with slice growth.
	roots := []WorkGoalTreeNode{}
	for i := range nodes {
		if nodes[i].ParentID == nil {
			roots = append(roots, nodes[i])
		}
	}
	// For simplicity (max 2 levels in Phase 1), attach children to roots.
	for i := range nodes {
		if nodes[i].ParentID == nil {
			continue
		}
		pid := *nodes[i].ParentID
		for j := range roots {
			if roots[j].ID == pid {
				roots[j].Children = append(roots[j].Children, nodes[i])
				break
			}
		}
	}
	if roots == nil {
		roots = []WorkGoalTreeNode{}
	}
	writeJSON(w, 200, roots)
}

func (gw *Gateway) handleCreateWorkGoal(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	var body struct {
		Title       string  `json:"title"`
		Description string  `json:"description"`
		ParentID    *string `json:"parent_id"`
		OrderIndex  int     `json:"order_index"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Title == "" {
		writeJSON(w, 400, map[string]string{"error": "title required"})
		return
	}
	var g WorkGoal
	err := gw.db.Pool.QueryRow(r.Context(),
		`INSERT INTO work_goals (tenant_id, title, description, parent_id, order_index)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, tenant_id, title, description, parent_id, order_index, status, created_at, updated_at`,
		defaultTenant, body.Title, body.Description, body.ParentID, body.OrderIndex).
		Scan(&g.ID, &g.TenantID, &g.Title, &g.Description, &g.ParentID,
			&g.OrderIndex, &g.Status, &g.CreatedAt, &g.UpdatedAt)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, g)
}

func (gw *Gateway) handleUpdateWorkGoal(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	id := chi.URLParam(r, "id")
	var body struct {
		Title       *string `json:"title"`
		Description *string `json:"description"`
		Status      *string `json:"status"`
		OrderIndex  *int    `json:"order_index"`
	}
	json.NewDecoder(r.Body).Decode(&body) //nolint:errcheck
	var g WorkGoal
	err := gw.db.Pool.QueryRow(r.Context(),
		`UPDATE work_goals SET
		   title       = COALESCE($2, title),
		   description = COALESCE($3, description),
		   status      = COALESCE($4, status),
		   order_index = COALESCE($5, order_index),
		   updated_at  = NOW()
		 WHERE id = $1 AND tenant_id = $6
		 RETURNING id, tenant_id, title, description, parent_id, order_index, status, created_at, updated_at`,
		id, body.Title, body.Description, body.Status, body.OrderIndex, defaultTenant).
		Scan(&g.ID, &g.TenantID, &g.Title, &g.Description, &g.ParentID,
			&g.OrderIndex, &g.Status, &g.CreatedAt, &g.UpdatedAt)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, g)
}

func (gw *Gateway) handleDeleteWorkGoal(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	id := chi.URLParam(r, "id")
	_, err := gw.db.Pool.Exec(r.Context(),
		`DELETE FROM work_goals WHERE id = $1 AND tenant_id = $2`, id, defaultTenant)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]string{"status": "deleted", "id": id})
}
