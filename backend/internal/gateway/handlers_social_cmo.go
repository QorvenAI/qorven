// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/qorvenai/qorven/internal/governance"
	socialqor "github.com/qorvenai/qorven/internal/qor/social"
	"github.com/qorvenai/qorven/internal/tasks"
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

// --- Campaign CRUD ---

func (gw *Gateway) handleListCampaigns(w http.ResponseWriter, r *http.Request) {
	store := gw.socialStore()
	if store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "social not configured"})
		return
	}
	dept, _ := store.ResolveMarketingDepartment(r.Context(), defaultTenant)
	list, err := store.ListCampaigns(r.Context(), defaultTenant, dept)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"campaigns": list})
}

func (gw *Gateway) handleCreateCampaign(w http.ResponseWriter, r *http.Request) {
	store := gw.socialStore()
	if store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "social not configured"})
		return
	}
	if u := userFromContext(r.Context()); u == nil || u.Role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "admin role required"})
		return
	}
	var body struct {
		Title           string   `json:"title"`
		Brief           string   `json:"brief"`
		TargetPlatforms []string `json:"target_platforms"`
		CreatedBy       string   `json:"created_by_agent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Title == "" {
		writeJSON(w, 400, map[string]string{"error": "title required"})
		return
	}
	dept, _ := store.ResolveMarketingDepartment(r.Context(), defaultTenant)
	id, err := store.CreateCampaign(r.Context(), socialqor.Campaign{
		TenantID:         defaultTenant,
		DepartmentID:     dept,
		CreatedByAgentID: body.CreatedBy,
		Title:            body.Title,
		Brief:            body.Brief,
		TargetPlatforms:  body.TargetPlatforms,
		Status:           "active",
	})
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (gw *Gateway) handleGetCampaign(w http.ResponseWriter, r *http.Request) {
	store := gw.socialStore()
	if store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "social not configured"})
		return
	}
	c, err := store.GetCampaign(r.Context(), defaultTenant, chi.URLParam(r, "id"))
	if err != nil || c == nil {
		writeJSON(w, 404, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (gw *Gateway) handleSetCampaignStatus(w http.ResponseWriter, r *http.Request) {
	store := gw.socialStore()
	if store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "social not configured"})
		return
	}
	if u := userFromContext(r.Context()); u == nil || u.Role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "admin role required"})
		return
	}
	var body struct {
		Status string `json:"status"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil || body.Status == "" {
		writeJSON(w, 400, map[string]string{"error": "status required"})
		return
	}
	if err := store.SetCampaignStatus(r.Context(), defaultTenant, chi.URLParam(r, "id"), body.Status); err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- CMO Campaign Delegation ---

// handleDelegateCampaign creates an assigned subtask for a sub-agent in the campaign's
// (marketing) department. The target agent must belong to the campaign's department —
// this closes the authz hole where the CMO could delegate to an agent in a different dept.
func (gw *Gateway) handleDelegateCampaign(w http.ResponseWriter, r *http.Request) {
	store := gw.socialStore()
	if store == nil || gw.taskStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "not configured"})
		return
	}
	if u := userFromContext(r.Context()); u == nil || u.Role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "admin role required"})
		return
	}
	campaignID := chi.URLParam(r, "id")
	var body struct {
		AgentID     string `json:"agent_id"`
		Title       string `json:"title"`
		Description string `json:"description"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil || body.AgentID == "" {
		writeJSON(w, 400, map[string]string{"error": "agent_id required"})
		return
	}
	campaign, err := store.GetCampaign(r.Context(), defaultTenant, campaignID)
	if err != nil || campaign == nil {
		writeJSON(w, 404, map[string]string{"error": "campaign not found"})
		return
	}
	// CONSTRAINT: the target agent must be in the campaign's (marketing) department.
	var agentDept *string
	if gw.db != nil {
		_ = gw.db.Pool.QueryRow(r.Context(), `SELECT department_id FROM agents WHERE id=$1`, body.AgentID).Scan(&agentDept)
	}
	if campaign.DepartmentID != "" && (agentDept == nil || *agentDept != campaign.DepartmentID) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "target agent is not in the campaign's department"})
		return
	}
	title := body.Title
	if title == "" {
		title = "Campaign: " + campaign.Title
	}
	desc := body.Description
	if desc == "" {
		desc = campaign.Brief
	}
	assignedTo := body.AgentID
	taskID, terr := gw.taskStore.Create(r.Context(), defaultTenant, tasks.Task{
		TenantID:    defaultTenant,
		Title:       title,
		Description: desc + "\n\n(Campaign: " + campaign.Title + ", id=" + campaignID + ". Create social posts under this campaign.)",
		AssignedTo:  &assignedTo,
		Status:      tasks.StatusAssigned,
		Priority:    3,
	})
	if terr != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(terr)})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"task_id": taskID, "status": "delegated"})
}
