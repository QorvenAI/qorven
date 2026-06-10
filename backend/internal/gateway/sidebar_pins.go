// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

// SidebarPin is a per-user pinned hub (room) or chat that surfaces in the
// pinned group at the top of the sidebar.
// Table: sidebar_pins.
type SidebarPin struct {
	ID         string    `json:"id"`
	ItemType   string    `json:"item_type"` // 'hub' | 'chat'
	ItemID     string    `json:"item_id"`
	OrderIndex int       `json:"order_index"`
	CreatedAt  time.Time `json:"created_at"`
}

func (gw *Gateway) handleListSidebarPins(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 200, []SidebarPin{})
		return
	}
	u := userFromContext(r.Context())
	if u == nil {
		writeJSON(w, 401, map[string]string{"error": "not authenticated"})
		return
	}
	rows, err := gw.db.Pool.Query(r.Context(),
		`SELECT id, item_type, item_id, order_index, created_at
		 FROM sidebar_pins WHERE user_id = $1 ORDER BY order_index, created_at`, u.ID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	defer rows.Close()
	list := []SidebarPin{}
	for rows.Next() {
		var p SidebarPin
		if err := rows.Scan(&p.ID, &p.ItemType, &p.ItemID, &p.OrderIndex, &p.CreatedAt); err != nil {
			continue
		}
		list = append(list, p)
	}
	if list == nil {
		list = []SidebarPin{}
	}
	writeJSON(w, 200, list)
}

func (gw *Gateway) handleCreateSidebarPin(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not available"})
		return
	}
	u := userFromContext(r.Context())
	if u == nil {
		writeJSON(w, 401, map[string]string{"error": "not authenticated"})
		return
	}
	var body struct {
		ItemType string `json:"item_type"`
		ItemID   string `json:"item_id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if (body.ItemType != "hub" && body.ItemType != "chat") || body.ItemID == "" {
		writeJSON(w, 400, map[string]string{"error": "item_type must be hub or chat, item_id required"})
		return
	}
	var p SidebarPin
	err := gw.db.Pool.QueryRow(r.Context(),
		`INSERT INTO sidebar_pins (tenant_id, user_id, item_type, item_id)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (user_id, item_type, item_id) DO UPDATE SET item_id = EXCLUDED.item_id
		 RETURNING id, item_type, item_id, order_index, created_at`,
		defaultTenant, u.ID, body.ItemType, body.ItemID).
		Scan(&p.ID, &p.ItemType, &p.ItemID, &p.OrderIndex, &p.CreatedAt)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, 200, p)
}

func (gw *Gateway) handleDeleteSidebarPin(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not available"})
		return
	}
	u := userFromContext(r.Context())
	if u == nil {
		writeJSON(w, 401, map[string]string{"error": "not authenticated"})
		return
	}
	itemType := chi.URLParam(r, "type")
	itemID := chi.URLParam(r, "id")
	if _, err := gw.db.Pool.Exec(r.Context(),
		`DELETE FROM sidebar_pins WHERE user_id = $1 AND item_type = $2 AND item_id = $3`,
		u.ID, itemType, itemID); err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}
