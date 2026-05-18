// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/qorvenai/qorven/internal/channels"
)

func (gw *Gateway) handleUpdateTask(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		w.WriteHeader(204)
		return
	}
	id := chi.URLParam(r, "id")
	var body map[string]any
	json.NewDecoder(r.Body).Decode(&body)
	sets := ""
	args := []any{id}
	i := 2
	for _, key := range []string{"title", "description", "status", "priority", "due_date", "assigned_to"} {
		if v, ok := body[key]; ok {
			if sets != "" {
				sets += ", "
			}
			sets += fmt.Sprintf("%s = $%d", key, i)
			args = append(args, v)
			i++
		}
	}
	if sets == "" {
		w.WriteHeader(204)
		return
	}
	gw.db.Pool.Exec(r.Context(), fmt.Sprintf("UPDATE tasks SET %s, updated_at = now() WHERE id = $1", sets), args...)
	w.WriteHeader(204)
}

func (gw *Gateway) handleListPairingRequests(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		json.NewEncoder(w).Encode([]any{})
		return
	}
	pc := channels.NewPolicyChecker(gw.db.Pool)
	list, err := pc.ListPending(r.Context(), defaultTenant)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	json.NewEncoder(w).Encode(list)
}

func (gw *Gateway) handleApprovePairing(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	var body struct {
		Code string `json:"code"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	pc := channels.NewPolicyChecker(gw.db.Pool)
	if err := pc.ApprovePairing(r.Context(), defaultTenant, body.Code); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, 400)
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "approved"})
}

func (gw *Gateway) handleListPairedDevices(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		json.NewEncoder(w).Encode([]any{})
		return
	}
	rows, err := gw.db.Pool.Query(r.Context(),
		`SELECT id, channel, sender_id, COALESCE(chat_id,''), COALESCE(sender_name,''), paired_at FROM paired_devices WHERE tenant_id = $1 ORDER BY paired_at DESC`, defaultTenant)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id, chType, senderID, chatID, senderName string
		var pairedAt time.Time
		rows.Scan(&id, &chType, &senderID, &chatID, &senderName, &pairedAt)
		list = append(list, map[string]any{"id": id, "channel_type": chType, "sender_id": senderID, "chat_id": chatID, "sender_name": senderName, "paired_at": pairedAt})
	}
	json.NewEncoder(w).Encode(list)
}
