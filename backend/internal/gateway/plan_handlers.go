// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	apievents "github.com/qorvenai/qorven/internal/api/events"
	"github.com/qorvenai/qorven/internal/approvals"
	"github.com/qorvenai/qorven/internal/permissions"
	"github.com/qorvenai/qorven/internal/plans"
)

// decodeBody reads + JSON-unmarshals the request body with strict settings.
// Returns a 400 error string on failure, empty otherwise.
func decodeJSONBody(r *http.Request, dst any, maxBytes int64) string {
	if r.Body == nil {
		return "empty body"
	}
	if maxBytes > 0 {
		r.Body = http.MaxBytesReader(nil, r.Body, maxBytes)
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return "empty body"
		}
		return err.Error()
	}
	return ""
}

// ─────────────────────────── /v1/plans ──────────────────────────────

func (gw *Gateway) handleListPlans(w http.ResponseWriter, r *http.Request) {
	if gw.plans == nil {
		writeJSON(w, 503, map[string]string{"error": "plan store not configured"})
		return
	}
	list, err := gw.plans.ListByTenant(r.Context(), defaultTenant, 100)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if list == nil {
		list = []*plans.Plan{}
	}
	writeJSON(w, 200, map[string]any{"plans": list})
}

func (gw *Gateway) handleCreatePlan(w http.ResponseWriter, r *http.Request) {
	if gw.plans == nil {
		writeJSON(w, 503, map[string]string{"error": "plan store not configured"})
		return
	}
	var in struct {
		Title     string          `json:"title"`
		Summary   string          `json:"summary"`
		ProjectID string          `json:"project_id"`
		SessionID string          `json:"session_id"`
		Spec      json.RawMessage `json:"spec"`
	}
	if msg := decodeJSONBody(r, &in, 1<<20); msg != "" {
		writeJSON(w, 400, map[string]string{"error": msg})
		return
	}
	if in.Title == "" {
		writeJSON(w, 400, map[string]string{"error": "title required"})
		return
	}
	actor := actorFromContext(r.Context())
	p, err := gw.plans.CreatePlan(r.Context(), plans.CreatePlanInput{
		TenantID:  defaultTenant,
		Title:     in.Title,
		Summary:   in.Summary,
		ProjectID: in.ProjectID,
		SessionID: in.SessionID,
		Spec:      json.RawMessage(in.Spec),
		CreatedBy: actor,
	})
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 201, p)
}

func (gw *Gateway) handleGetPlan(w http.ResponseWriter, r *http.Request) {
	if gw.plans == nil {
		writeJSON(w, 503, map[string]string{"error": "plan store not configured"})
		return
	}
	id := chi.URLParam(r, "id")
	p, err := gw.plans.GetPlan(r.Context(), id)
	if errors.Is(err, plans.ErrNotFound) {
		writeJSON(w, 404, map[string]string{"error": "plan not found"})
		return
	}
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if err := gw.authorizeForPlan(r.Context(), p); err != nil {
		writePlanAuthzError(w, err)
		return
	}
	writeJSON(w, 200, p)
}

func (gw *Gateway) handleListPlanNodes(w http.ResponseWriter, r *http.Request) {
	if gw.plans == nil {
		writeJSON(w, 503, map[string]string{"error": "plan store not configured"})
		return
	}
	id := chi.URLParam(r, "id")
	p, err := gw.plans.GetPlan(r.Context(), id)
	if errors.Is(err, plans.ErrNotFound) {
		writeJSON(w, 404, map[string]string{"error": "plan not found"})
		return
	}
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if err := gw.authorizeForPlan(r.Context(), p); err != nil {
		writePlanAuthzError(w, err)
		return
	}
	nodes, err := gw.plans.ListNodesByPlan(r.Context(), id)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	edges, err := gw.plans.ListEdgesByPlan(r.Context(), id)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"nodes": nodes, "edges": edges})
}

// handleApprovePlan resolves the plan's open human_feedback approval and
// transitions the plan into approved status. Also records a system
// comment if one was supplied.
func (gw *Gateway) handleApprovePlan(w http.ResponseWriter, r *http.Request) {
	gw.resolvePlanApproval(w, r, approvals.StateApproved, apievents.TypePlanApproved)
}

func (gw *Gateway) handleRejectPlan(w http.ResponseWriter, r *http.Request) {
	gw.resolvePlanApproval(w, r, approvals.StateRejected, apievents.TypePlanRejected)
}

func (gw *Gateway) handleRevisePlan(w http.ResponseWriter, r *http.Request) {
	gw.resolvePlanApproval(w, r, approvals.StateRevisionRequested, apievents.TypePlanRevisionRequested)
}

func (gw *Gateway) resolvePlanApproval(w http.ResponseWriter, r *http.Request, target approvals.State, eventType apievents.Type) {
	if gw.plans == nil || gw.approvals == nil {
		writeJSON(w, 503, map[string]string{"error": "plan or approval store not configured"})
		return
	}
	id := chi.URLParam(r, "id")
	p, err := gw.plans.GetPlan(r.Context(), id)
	if errors.Is(err, plans.ErrNotFound) {
		writeJSON(w, 404, map[string]string{"error": "plan not found"})
		return
	}
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if err := gw.authorizeForPlan(r.Context(), p); err != nil {
		writePlanAuthzError(w, err)
		return
	}

	var in struct {
		Comment string `json:"comment"`
	}
	if r.ContentLength > 0 {
		if msg := decodeJSONBody(r, &in, 1<<20); msg != "" && msg != "empty body" {
			writeJSON(w, 400, map[string]string{"error": msg})
			return
		}
	}
	// Target-state sanity: reject/revision comments are strongly recommended.
	if (target == approvals.StateRejected || target == approvals.StateRevisionRequested) && strings.TrimSpace(in.Comment) == "" {
		writeJSON(w, 400, map[string]string{"error": "comment required for reject/revision"})
		return
	}

	// Find the plan's latest pending approval — the runtime creates at
	// most one per human_feedback node at a time.
	list, err := gw.approvals.ListByPlan(r.Context(), id)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	var pending *approvals.Approval
	for _, a := range list {
		if a.State == approvals.StatePending {
			pending = a
			break
		}
	}
	if pending == nil {
		// Grace-window idempotency: if the plan already reached the target
		// status (e.g. a duplicate approve within seconds), treat as success.
		alreadyDone := false
		switch target {
		case approvals.StateApproved:
			alreadyDone = p.Status == plans.StatusApproved || p.Status == plans.StatusRunning || p.Status == plans.StatusDone
		case approvals.StateRejected:
			alreadyDone = p.Status == plans.StatusRejected
		case approvals.StateRevisionRequested:
			alreadyDone = p.Status == plans.StatusRevisionRequested
		}
		if alreadyDone {
			writeJSON(w, 200, map[string]any{"plan": p, "approval": nil})
			return
		}
		writeJSON(w, 409, map[string]string{"error": "no pending approval for this plan", "code": "no_pending"})
		return
	}

	actor := actorFromContext(r.Context())
	resolved, err := gw.approvals.Resolve(r.Context(), approvals.ResolveInput{
		ApprovalID: pending.ID,
		Next:       target,
		ResolvedBy: actor,
		Comment:    in.Comment,
	})
	if err != nil {
		if approvals.IsIllegalTransition(err) {
			writeJSON(w, 409, map[string]string{"error": err.Error(), "code": "illegal_transition"})
			return
		}
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	// Emit the matching plan.* event so clients update the UI.
	if gw.events != nil {
		var props any
		switch target {
		case approvals.StateApproved:
			props = apievents.PlanApprovedProps{ProjectID: p.ProjectID, PlanID: p.ID, Actor: actor}
		case approvals.StateRejected:
			props = apievents.PlanRejectedProps{ProjectID: p.ProjectID, PlanID: p.ID, Actor: actor, Comment: in.Comment}
		case approvals.StateRevisionRequested:
			props = apievents.PlanRevisionRequestedProps{ProjectID: p.ProjectID, PlanID: p.ID, Actor: actor, Comment: in.Comment}
		}
		if err := gw.events.Emit(r.Context(), apievents.SinkAll, eventType, props); err != nil {
			slog.Warn("plan event emit failed", "type", eventType, "err", err)
		}
	}

	// Queue a wakeup_request for auditability — even if the graph runs
	// inline below, the row is the durable record of who approved and when.
	// The legacy schema (migration 034) requires tenant_id/source/actor_type;
	// migration 037 keeps them with defaults but we pass explicit values.
	if gw.db != nil {
		_, _ = gw.db.Pool.Exec(r.Context(), `
            INSERT INTO wakeup_requests (agent_id, tenant_id, source, actor_type, cause, payload, plan_id)
            VALUES ($1, $2, 'approval_handler', 'user', 'approval_resolved', $3::jsonb, $4)
        `, "orchestrator", p.TenantID, `{"approval_id":"`+resolved.ID+`"}`, p.ID)
	}

	// Resume the plan if the orchestrator is configured. We detach from
	// the HTTP request context so the caller gets a fast 200 back while
	// the graph drives to its next quiescent state on a background
	// context. Errors are logged; the next call (poll or resubmit) sees
	// the plan's updated row.
	if gw.orchestrator != nil && target == approvals.StateApproved {
		planID := p.ID
		go func() {
			bgCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			if err := gw.orchestrator.ExecutePlan(bgCtx, planID); err != nil {
				slog.Warn("orchestrator.ExecutePlan after approve failed", "plan_id", planID, "err", err)
			}
		}()
	}

	writeJSON(w, 200, map[string]any{
		"plan":     p,
		"approval": resolved,
	})
}

// handleAppendApprovalComment appends a comment to an approval thread.
func (gw *Gateway) handleAppendApprovalComment(w http.ResponseWriter, r *http.Request) {
	if gw.approvals == nil {
		writeJSON(w, 503, map[string]string{"error": "approval store not configured"})
		return
	}
	apprID := chi.URLParam(r, "id")
	var in struct {
		Body     string `json:"body"`
		AuthorIs string `json:"author_is"`
	}
	if msg := decodeJSONBody(r, &in, 1<<20); msg != "" {
		writeJSON(w, 400, map[string]string{"error": msg})
		return
	}
	if strings.TrimSpace(in.Body) == "" {
		writeJSON(w, 400, map[string]string{"error": "body required"})
		return
	}
	if in.AuthorIs == "" {
		in.AuthorIs = "user"
	}
	actor := actorFromContext(r.Context())
	c, err := gw.approvals.AppendComment(r.Context(), apprID, actor, in.AuthorIs, in.Body)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 201, c)
}

// writePlanAuthzError translates an authorizeForPlan error to an HTTP
// response. All plan handlers use this to keep the 403 shape uniform.
// FU-017: replaces the old string-returning adapter; auth now goes
// through authorize.go exclusively.
func writePlanAuthzError(w http.ResponseWriter, err error) {
	if authzErr, ok := isAuthzError(err); ok {
		writeJSON(w, 403, map[string]string{"error": authzErr.Reason, "code": authzErr.Code})
		return
	}
	writeJSON(w, 500, map[string]string{"error": err.Error()})
}

// ─────────────────────────── /v1/permissions ──────────────────────────────

// handlePermissionReply is POST /v1/permissions/{id}/reply.
func (gw *Gateway) handlePermissionReply(w http.ResponseWriter, r *http.Request) {
	if gw.permissionGate == nil {
		writeJSON(w, 503, map[string]string{"error": "permission gate not configured"})
		return
	}
	reqID := chi.URLParam(r, "id")
	var in permissions.ReplyInput
	if msg := decodeJSONBody(r, &in, 64*1024); msg != "" {
		writeJSON(w, 400, map[string]string{"error": msg})
		return
	}
	in.RepliedBy = actorFromContext(r.Context())
	switch in.Decision {
	case permissions.DecisionAllow, permissions.DecisionAlwaysAllow,
		permissions.DecisionAllowSession, permissions.DecisionAllow1h, permissions.DecisionDeny:
		// ok
	default:
		writeJSON(w, 400, map[string]string{"error": "decision must be 'allow', 'allow_always', 'allow_session', 'allow_1h', or 'deny'"})
		return
	}
	// Authz: the replier must own the session the request belongs to,
	// OR be an admin/service account.
	existing, err := gw.permissionGate.Get(r.Context(), reqID)
	if errors.Is(err, permissions.ErrNotFound) {
		writeJSON(w, 404, map[string]string{"error": "permission request not found"})
		return
	}
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if existing.SessionID != "" {
		if authzErr := gw.protocolOwnerCheck(r.Context(), existing.SessionID); authzErr != nil {
			writeJSON(w, 403, map[string]string{"error": authzErr.Error(), "code": "forbidden"})
			return
		}
	}

	r2, err := gw.permissionGate.Reply(r.Context(), reqID, in)
	if err != nil {
		if errors.Is(err, permissions.ErrNotFound) {
			writeJSON(w, 404, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}

	u := userFromContext(r.Context())
	switch in.Decision {
	case permissions.DecisionAlwaysAllow:
		// Persist a permanent workspace-wide policy so future requests skip the gate.
		if u != nil && u.TenantID != "" {
			_ = gw.permissionGate.SetPolicyScoped(r.Context(), u.TenantID, u.ID, "", r2.Tool, permissions.ScopeAutoApproved)
		}
	case permissions.DecisionAllowSession:
		// Transient session-scoped allow (lives until process restart).
		if u != nil && u.TenantID != "" {
			gw.permissionGate.AddTransientAllow(u.TenantID, u.ID, "", r2.Tool, 0)
		}
	case permissions.DecisionAllow1h:
		// Transient timed allow — expires in 1 hour.
		if u != nil && u.TenantID != "" {
			gw.permissionGate.AddTransientAllow(u.TenantID, u.ID, "", r2.Tool, time.Hour)
		}
	}

	writeJSON(w, 200, r2)
}

// handleListPendingPermissions returns pending requests for a session.
func (gw *Gateway) handleListPendingPermissions(w http.ResponseWriter, r *http.Request) {
	if gw.permissionGate == nil {
		writeJSON(w, 503, map[string]string{"error": "permission gate not configured"})
		return
	}
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		writeJSON(w, 400, map[string]string{"error": "session_id required"})
		return
	}
	if authzErr := gw.protocolOwnerCheck(r.Context(), sessionID); authzErr != nil {
		writeJSON(w, 403, map[string]string{"error": authzErr.Error(), "code": "forbidden"})
		return
	}
	list, err := gw.permissionGate.ListPendingForSession(r.Context(), sessionID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"requests": list})
}

// ─────────────────────────── /v1/admin/dead-letters ───────────────────────

// handleArchivePlan is POST /v1/plans/{id}/archive.
// Only terminal plans (done, failed, cancelled, rejected) may be archived.
func (gw *Gateway) handleArchivePlan(w http.ResponseWriter, r *http.Request) {
	if gw.plans == nil {
		writeJSON(w, 503, map[string]string{"error": "plan store not configured"})
		return
	}
	id := chi.URLParam(r, "id")
	p, err := gw.plans.GetPlan(r.Context(), id)
	if errors.Is(err, plans.ErrNotFound) {
		writeJSON(w, 404, map[string]string{"error": "plan not found"})
		return
	}
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if err := gw.authorizeForPlan(r.Context(), p); err != nil {
		writePlanAuthzError(w, err)
		return
	}
	archived, err := gw.plans.ArchivePlan(r.Context(), id)
	if plans.IsIllegalTransition(err) {
		writeJSON(w, 409, map[string]string{
			"error": err.Error(),
			"code":  "illegal_transition",
		})
		return
	}
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, archived)
}

// handleListArchivedPlans is GET /v1/plans/archived.
func (gw *Gateway) handleListArchivedPlans(w http.ResponseWriter, r *http.Request) {
	if gw.plans == nil {
		writeJSON(w, 503, map[string]string{"error": "plan store not configured"})
		return
	}
	list, err := gw.plans.ListArchivedByTenant(r.Context(), defaultTenant, 100)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if list == nil {
		list = []*plans.Plan{}
	}
	writeJSON(w, 200, list)
}

// handleListDeadLetters returns wakeup_request rows that have been
// dead-lettered after exhausting their retry budget (FU-021).
func (gw *Gateway) handleListDeadLetters(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	rows, err := gw.db.Pool.Query(r.Context(), `
        SELECT id, cause, plan_id::text, COALESCE(node_id::text,''),
               attempts, dead_letter_reason, created_at, consumed_at
          FROM wakeup_requests
         WHERE dead_letter_reason IS NOT NULL
         ORDER BY consumed_at DESC
         LIMIT 200
    `)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	type row struct {
		ID               string     `json:"id"`
		Cause            string     `json:"cause"`
		PlanID           string     `json:"plan_id,omitempty"`
		NodeID           string     `json:"node_id,omitempty"`
		Attempts         int        `json:"attempts"`
		DeadLetterReason string     `json:"dead_letter_reason"`
		CreatedAt        time.Time  `json:"created_at"`
		ConsumedAt       *time.Time `json:"consumed_at,omitempty"`
	}
	out := []row{}
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.ID, &r.Cause, &r.PlanID, &r.NodeID,
			&r.Attempts, &r.DeadLetterReason, &r.CreatedAt, &r.ConsumedAt); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		out = append(out, r)
	}
	if out == nil {
		out = []row{}
	}
	writeJSON(w, 200, out)
}
