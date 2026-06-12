// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/qorvenai/qorven/internal/governance"
	socialqor "github.com/qorvenai/qorven/internal/qor/social"
)

// applySocialApprovalGate decides the status a freshly-created post should hold.
// A human admin composing directly bypasses (schedules). An agent-authored post
// honors the author's outbound_approval: "none" schedules; otherwise it is held
// pending_approval and an approval request is routed to the CMO. The scheduled
// dispatcher only publishes status='scheduled', so a pending post never goes out.
func (gw *Gateway) applySocialApprovalGate(ctx context.Context, post *socialqor.Post, humanAdmin bool) socialqor.PostStatus {
	if humanAdmin {
		return socialqor.PostScheduled // a human operator publishing their own post
	}
	mode := "supervisor"
	if gw.db != nil && post.AgentID != "" {
		_ = gw.db.Pool.QueryRow(ctx, `SELECT COALESCE(outbound_approval,'supervisor') FROM agents WHERE id=$1`, post.AgentID).Scan(&mode)
	}
	if !socialqor.NeedsApproval(mode) {
		return socialqor.PostScheduled
	}
	if gw.approvalStore != nil {
		_ = gw.approvalStore.CreateRequest(ctx, governance.ApprovalRequest{
			TenantID:     defaultTenant,
			ActionType:   "social_post",
			RequestorID:  post.AgentID,
			ApproverRole: "cmo",
			Context:      map[string]any{"post_id": post.ID, "department_id": post.DepartmentID},
			Status:       "pending",
		})
	}
	return socialqor.PostPendingApproval
}

func (gw *Gateway) handleListSocialPendingApprovals(w http.ResponseWriter, r *http.Request) {
	store := gw.socialStore()
	if store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "social not configured"})
		return
	}
	dept, _ := store.ResolveMarketingDepartment(r.Context(), defaultTenant)
	posts, err := store.ListByDepartment(r.Context(), defaultTenant, dept, socialqor.PostPendingApproval)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"posts": posts})
}

func (gw *Gateway) handleApproveSocialPost(w http.ResponseWriter, r *http.Request) {
	if u := userFromContext(r.Context()); u == nil || u.Role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "admin role required"})
		return
	}
	store := gw.socialStore()
	if store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "social not configured"})
		return
	}
	postID := chi.URLParam(r, "id")
	if err := store.UpdatePostStatus(r.Context(), postID, socialqor.PostScheduled); err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	store.SetApprovalStatus(r.Context(), postID, "approved")
	w.WriteHeader(http.StatusNoContent)
}

func (gw *Gateway) handleRejectSocialPost(w http.ResponseWriter, r *http.Request) {
	if u := userFromContext(r.Context()); u == nil || u.Role != "admin" {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	store := gw.socialStore()
	if store == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	postID := chi.URLParam(r, "id")
	store.UpdatePostStatus(r.Context(), postID, socialqor.PostFailed)
	store.SetApprovalStatus(r.Context(), postID, "rejected")
	w.WriteHeader(http.StatusNoContent)
}
