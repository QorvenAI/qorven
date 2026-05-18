// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/qorvenai/qorven/internal/serviceaccounts"
)

func (gw *Gateway) handleListServiceAccounts(w http.ResponseWriter, r *http.Request) {
	if gw.serviceAccounts == nil {
		writeJSON(w, 503, map[string]string{"error": "service accounts not configured"})
		return
	}
	list, err := gw.serviceAccounts.List(r.Context())
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if list == nil {
		list = []*serviceaccounts.ServiceAccount{}
	}
	writeJSON(w, 200, list)
}

func (gw *Gateway) handleCreateServiceAccount(w http.ResponseWriter, r *http.Request) {
	if gw.serviceAccounts == nil {
		writeJSON(w, 503, map[string]string{"error": "service accounts not configured"})
		return
	}
	var body struct {
		ID          string `json:"id"`
		Role        string `json:"role"`
		Description string `json:"description"`
		TenantID    string `json:"tenant_id"`
		Global      bool   `json:"global"`
		Force       bool   `json:"force"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid request body"})
		return
	}
	actor := actorFromContext(r.Context())

	var sa *serviceaccounts.ServiceAccount
	var err error
	if body.Global || body.TenantID == "" {
		sa, err = gw.serviceAccounts.AddGlobal(r.Context(), body.ID, serviceaccounts.Role(body.Role), body.Description, actor)
	} else {
		in := serviceaccounts.AddInput{
			ID:          body.ID,
			Role:        serviceaccounts.Role(body.Role),
			Description: body.Description,
			CreatedBy:   actor,
			TenantID:    body.TenantID,
		}
		if body.Force {
			sa, err = gw.serviceAccounts.Upsert(r.Context(), in)
		} else {
			sa, err = gw.serviceAccounts.Add(r.Context(), in)
		}
	}
	if err != nil {
		status := 400
		if errors.Is(err, serviceaccounts.ErrAlreadyExists) {
			status = 409
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 201, sa)
}

func (gw *Gateway) handleRevokeServiceAccount(w http.ResponseWriter, r *http.Request) {
	if gw.serviceAccounts == nil {
		writeJSON(w, 503, map[string]string{"error": "service accounts not configured"})
		return
	}
	id := chi.URLParam(r, "id")
	actor := actorFromContext(r.Context())
	if err := gw.serviceAccounts.Revoke(r.Context(), id, actor); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]string{"status": "revoked"})
}
