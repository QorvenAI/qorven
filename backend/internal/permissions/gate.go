// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package permissions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	apievents "github.com/qorvenai/qorven/internal/api/events"
	"github.com/qorvenai/qorven/internal/byom"
	"github.com/qorvenai/qorven/internal/store"
)

// q routes gate queries through the tenant-scoped tx on ctx when one
// is present (multi-tenant HTTP handlers), else the raw pool. Every
// query in this file MUST go through q.
func (g *Gate) q(ctx context.Context) store.Queryable {
	return store.FromContext(ctx, g.pool)
}

// Gate is the service the tool runner calls to gate a tool execution.
// It persists the request, emits permission.requested, then blocks on
// a per-request channel until Reply flips the state (or the timeout
// fires).
type Gate struct {
	pool    *pgxpool.Pool
	emitter *apievents.Emitter

	// Default timeout applied when RequestInput.Timeout is zero.
	DefaultTimeout time.Duration

	// pending is an in-process signal bus: goroutines in Request() block on
	// these channels until Reply() fires the signal. This is NOT a K-V
	// cache and cannot be replaced by cache.Cache — it needs Redis pub/sub
	// semantics (SUBSCRIBE/PUBLISH per request_id) for multi-replica.
	// Deferred to Phase 4: until then, the DB row is the source of truth
	// and clients may poll GET /v1/permissions/{id} if the long-poll races
	// a replica restart. FU-018 covers DraftStore and SA cache only.
	mu      sync.Mutex
	pending map[string]chan *Request // request id → reply signal

	// Transient in-memory grants that expire without persisting to DB.
	// Key: "<tenantID>:<userID>:<agentID>:<tool>" → expiry (zero = session-only, checked via presence)
	// session-scoped grants live until process restart; timed grants track an expiry wall-clock time.
	transientMu sync.RWMutex
	transient   map[string]time.Time // zero time = session-scoped (never expires via timer)
}

// SetPolicy stores an "always allow" policy for a (tenant, user, tool) triple.
// Subsequent calls to Request() for this combination skip the blocking prompt.
func (g *Gate) SetPolicy(ctx context.Context, tenantID, userID, tool string) error {
	_, err := g.pool.Exec(ctx, `
		INSERT INTO permission_policies (tenant_id, user_id, tool)
		VALUES ($1::uuid, $2::uuid, $3)
		ON CONFLICT (tenant_id, user_id, tool) DO NOTHING
	`, tenantID, userID, tool)
	return err
}

// HasPolicy reports whether a workspace-wide auto_approved policy exists.
// Kept for backward compatibility with gated tools that predate per-agent profiles.
func (g *Gate) HasPolicy(ctx context.Context, tenantID, userID, tool string) bool {
	return g.HasPolicyForAgent(ctx, tenantID, userID, "", tool, ScopeAutoApproved)
}

// RevokePolicy removes the policy for (tenant, user, agent, tool).
// Pass agentID="" to remove a workspace-wide policy.
func (g *Gate) RevokePolicy(ctx context.Context, tenantID, userID, agentID, tool string) error {
	agentArg := any(nil)
	if agentID != "" {
		agentArg = agentID
	}
	_, err := g.pool.Exec(ctx, `
		DELETE FROM permission_policies
		WHERE tenant_id = $1::uuid
		  AND user_id   = $2::uuid
		  AND tool      = $3
		  AND (($4::text IS NULL AND agent_id IS NULL) OR agent_id = $4::uuid)
	`, tenantID, userID, tool, agentArg)
	return err
}

// SetPolicyScoped upserts a policy for (tenant, user, agent, tool) with
// the given scope. Pass agentID="" for workspace-wide policies.
func (g *Gate) SetPolicyScoped(ctx context.Context, tenantID, userID, agentID, tool string, scope PermScope) error {
	_, err := g.pool.Exec(ctx, `
		INSERT INTO permission_policies (tenant_id, user_id, agent_id, tool, scope)
		VALUES ($1::uuid, $2::uuid, NULLIF($3, '')::uuid, $4, $5)
		ON CONFLICT ON CONSTRAINT permission_policies_unique
		DO UPDATE SET scope = EXCLUDED.scope
	`, tenantID, userID, agentID, tool, string(scope))
	return err
}

// HasPolicyForAgent reports whether a policy with the given scope exists
// for this (tenant, user, agent, tool). Checks the agent-specific row
// first; falls back to workspace-wide (agent_id IS NULL) rows.
func (g *Gate) HasPolicyForAgent(ctx context.Context, tenantID, userID, agentID, tool string, scope PermScope) bool {
	if tenantID == "" || userID == "" {
		return false
	}
	var exists bool
	err := g.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM permission_policies
			WHERE tenant_id = $1::uuid
			  AND user_id   = $2::uuid
			  AND tool      = $3
			  AND scope     = $4
			  AND (
			    ($5 != '' AND agent_id = $5::uuid)
			    OR agent_id IS NULL
			  )
		)
	`, tenantID, userID, tool, string(scope), agentID).Scan(&exists)
	return err == nil && exists
}

// IsPolicyBlocked reports whether a blocked policy exists for this combination.
func (g *Gate) IsPolicyBlocked(ctx context.Context, tenantID, userID, agentID, tool string) bool {
	return g.HasPolicyForAgent(ctx, tenantID, userID, agentID, tool, ScopeBlocked)
}

// ListPolicies returns all permission_policies rows for an agent.
// Pass agentID="" to list workspace-wide (agent_id IS NULL) rows.
func (g *Gate) ListPolicies(ctx context.Context, tenantID, agentID string) ([]PolicyEntry, error) {
	var (
		rows pgx.Rows
		err  error
	)
	if agentID == "" {
		rows, err = g.pool.Query(ctx, `
			SELECT id::text, tenant_id::text, COALESCE(user_id::text,''),
			       COALESCE(agent_id::text,''), tool, scope, created_at
			FROM permission_policies
			WHERE tenant_id = $1::uuid AND agent_id IS NULL
			ORDER BY tool
		`, tenantID)
	} else {
		rows, err = g.pool.Query(ctx, `
			SELECT id::text, tenant_id::text, COALESCE(user_id::text,''),
			       COALESCE(agent_id::text,''), tool, scope, created_at
			FROM permission_policies
			WHERE tenant_id = $1::uuid AND agent_id = $2::uuid
			ORDER BY tool
		`, tenantID, agentID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PolicyEntry
	for rows.Next() {
		var p PolicyEntry
		var scopeStr string
		if err := rows.Scan(&p.ID, &p.TenantID, &p.UserID, &p.AgentID, &p.Tool, &scopeStr, &p.CreatedAt); err != nil {
			return nil, err
		}
		p.Scope = PermScope(scopeStr)
		out = append(out, p)
	}
	return out, rows.Err()
}

// LoadDefaults seeds the default permission profile for an agent if none
// exist yet. Safe to call every session start — INSERT uses ON CONFLICT DO NOTHING.
func (g *Gate) LoadDefaults(ctx context.Context, tenantID, userID, agentID, agentKey string) error {
	if tenantID == "" || userID == "" || agentID == "" {
		return nil
	}
	defaults := DefaultsForRole(agentKey)
	for _, d := range defaults {
		_, err := g.pool.Exec(ctx, `
			INSERT INTO permission_policies (tenant_id, user_id, agent_id, tool, scope)
			VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5)
			ON CONFLICT ON CONSTRAINT permission_policies_unique DO NOTHING
		`, tenantID, userID, agentID, d.Tool, string(d.Scope))
		if err != nil {
			return err
		}
	}
	return nil
}

// NewGate constructs a Gate. pool is required; emitter may be nil (events
// won't fire — persistent state still works and clients polling via
// GET /v1/permissions/{id} can see updates).
func NewGate(pool *pgxpool.Pool, emitter *apievents.Emitter) *Gate {
	return &Gate{
		pool:           pool,
		emitter:        emitter,
		DefaultTimeout: byom.Load().PermissionTimeout,
		pending:        make(map[string]chan *Request),
		transient:      make(map[string]time.Time),
	}
}

// transientKey returns the map key for a transient allow grant.
func transientKey(tenantID, userID, agentID, tool string) string {
	return tenantID + ":" + userID + ":" + agentID + ":" + tool
}

// HasTransientAllow reports whether a live transient grant exists.
func (g *Gate) HasTransientAllow(tenantID, userID, agentID, tool string) bool {
	g.transientMu.RLock()
	expiry, ok := g.transient[transientKey(tenantID, userID, agentID, tool)]
	g.transientMu.RUnlock()
	if !ok {
		return false
	}
	// Zero time = session-scoped (never expires); non-zero = timed.
	return expiry.IsZero() || time.Now().Before(expiry)
}

// AddTransientAllow registers an in-memory grant. duration=0 means session-scoped.
// Called by the HTTP handler when the user chooses allow_session or allow_1h.
func (g *Gate) AddTransientAllow(tenantID, userID, agentID, tool string, duration time.Duration) {
	var expiry time.Time
	if duration > 0 {
		expiry = time.Now().Add(duration)
	}
	g.transientMu.Lock()
	g.transient[transientKey(tenantID, userID, agentID, tool)] = expiry
	g.transientMu.Unlock()
}

// Request gates a tool execution. Returns a Verdict or an error. Behavior:
//
//   - persists a pending row in permission_requests
//   - emits permission.requested on the event bus
//   - blocks until Reply marks the row allowed/denied, or the timeout
//     (in) fires
//   - on timeout the row is flipped to 'expired' and the caller gets
//     an *ExpiredError (callers MUST treat expired as deny)
//
// The gate panics if pool is nil — misconfiguration is a deploy error.
func (g *Gate) Request(ctx context.Context, in RequestInput) (*Verdict, error) {
	if g.pool == nil {
		return nil, errors.New("permissions: gate not configured (nil pool)")
	}
	if in.Tool == "" {
		return nil, errors.New("permissions: tool required")
	}

	// Short-circuit: check per-agent policy first, then workspace-wide.
	if in.UserID != "" && in.TenantID != "" {
		agentID := in.AgentID
		// Blocked takes precedence over all other policies.
		if g.IsPolicyBlocked(ctx, in.TenantID, in.UserID, agentID, in.Tool) {
			return &Verdict{Decision: DecisionDeny}, nil
		}
		// Auto-approved: skip the prompt entirely.
		if g.HasPolicyForAgent(ctx, in.TenantID, in.UserID, agentID, in.Tool, ScopeAutoApproved) {
			return &Verdict{Decision: DecisionAllow}, nil
		}
		// Transient (session or timed) allow granted by a prior user reply.
		if g.HasTransientAllow(in.TenantID, in.UserID, agentID, in.Tool) {
			return &Verdict{Decision: DecisionAllow}, nil
		}
	}

	timeout := in.Timeout
	if timeout == 0 {
		timeout = g.DefaultTimeout
	}
	argsJSON := []byte("{}")
	if in.Args != nil {
		b, err := json.Marshal(in.Args)
		if err != nil {
			return nil, fmt.Errorf("permissions: marshal args: %w", err)
		}
		argsJSON = b
	}

	// Persist.
	var r Request
	var expiresAt *time.Time
	var deadline time.Time
	if timeout > 0 {
		deadline = time.Now().Add(timeout)
		expiresAt = &deadline
	}
	// Fall back to the column default ('default') when TenantID is
	// empty — preserves the single-tenant shape. Multi-tenant callers
	// MUST set in.TenantID so the RLS policy's tenant_id::uuid cast
	// succeeds; that plumbing lands in the wrapper (WrapLazy) which
	// is what our orchestrator path goes through.
	tenantArg := any(nil)
	if in.TenantID != "" {
		tenantArg = in.TenantID
	}
	err := g.q(ctx).QueryRow(ctx, `
        INSERT INTO permission_requests
            (session_id, plan_id, node_id, agent_key, tool, args, reason,
             state, requested_by, expires_at, tenant_id)
        VALUES
            (NULLIF($1,'')::uuid, NULLIF($2,'')::uuid, NULLIF($3,'')::uuid,
             NULLIF($4,''), $5, $6, $7, 'pending', $8, $9,
             COALESCE($10, 'default'))
        RETURNING id, COALESCE(session_id::text,''), COALESCE(plan_id::text,''),
                  COALESCE(node_id::text,''), COALESCE(agent_key,''),
                  tool, args, COALESCE(reason,''), state, COALESCE(requested_by,''),
                  COALESCE(replied_by,''), COALESCE(note,''), created_at, replied_at, expires_at
    `,
		in.SessionID, in.PlanID, in.NodeID, in.AgentKey, in.Tool,
		argsJSON, in.Reason, in.RequestedBy, expiresAt, tenantArg,
	).Scan(
		&r.ID, &r.SessionID, &r.PlanID, &r.NodeID, &r.AgentKey,
		&r.Tool, &r.Args, &r.Reason, &r.State, &r.RequestedBy,
		&r.RepliedBy, &r.Note, &r.CreatedAt, &r.RepliedAt, &r.ExpiresAt,
	)
	if err != nil {
		return nil, fmt.Errorf("permissions: persist: %w", err)
	}

	// Install a reply channel BEFORE the event fires to prevent a TOCTTOU
	// where the reply arrives before we start listening.
	ch := make(chan *Request, 1)
	g.mu.Lock()
	g.pending[r.ID] = ch
	g.mu.Unlock()
	defer func() {
		g.mu.Lock()
		delete(g.pending, r.ID)
		g.mu.Unlock()
	}()

	// Emit permission.requested.
	if g.emitter != nil {
		_ = g.emitter.Emit(ctx, apievents.SinkAll, apievents.TypePermissionRequested,
			apievents.PermissionRequestedProps{
				RequestID:          r.ID,
				SessionID:          r.SessionID,
				AgentKey:           r.AgentKey,
				Tool:               r.Tool,
				Args:               decodeArgsMap(r.Args),
				Reason:             r.Reason,
				AutoApproveAfterMS: int(timeout / time.Millisecond),
			})
	}

	var timeoutCh <-chan time.Time
	if timeout > 0 {
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		timeoutCh = timer.C
	}

	select {
	case <-ctx.Done():
		// Context cancel: flip to expired (caller abandoned) and return ctx.Err.
		_ = g.expire(context.Background(), r.ID)
		return nil, ctx.Err()

	case <-timeoutCh:
		if err := g.expire(ctx, r.ID); err != nil {
			return nil, err
		}
		if g.emitter != nil {
			_ = g.emitter.Emit(ctx, apievents.SinkAll, apievents.TypePermissionReplied,
				apievents.PermissionRepliedProps{
					RequestID: r.ID,
					Decision:  "deny",
					Note:      fmt.Sprintf("auto-denied after %s", timeout),
				})
		}
		return &Verdict{Decision: DecisionDeny, Request: &r, Expired: true},
			&ExpiredError{RequestID: r.ID, After: timeout}

	case reply := <-ch:
		var verdict Verdict
		verdict.Request = reply
		switch reply.State {
		case StateAllowed:
			verdict.Decision = DecisionAllow
		case StateDenied:
			verdict.Decision = DecisionDeny
		case StateExpired:
			verdict.Decision = DecisionDeny
			verdict.Expired = true
		default:
			return nil, fmt.Errorf("permissions: unexpected reply state %q", reply.State)
		}
		return &verdict, nil
	}
}

// Reply resolves a pending request. Called by the HTTP handler behind
// POST /v1/permissions/{id}/reply. Idempotent — repeating the same
// decision is a no-op; conflicting later decisions return an error so
// an audit signal surfaces rather than silently accept the overwrite.
func (g *Gate) Reply(ctx context.Context, requestID string, in ReplyInput) (*Request, error) {
	if requestID == "" {
		return nil, errors.New("permissions: request_id required")
	}
	switch in.Decision {
	case DecisionAllow, DecisionAlwaysAllow, DecisionAllowSession, DecisionAllow1h, DecisionDeny:
		// ok
	default:
		return nil, fmt.Errorf("permissions: invalid decision %q", in.Decision)
	}

	// allow_always / allow_session / allow_1h: treat the current request as
	// allowed; side-effects (policy persist or transient grant) are applied
	// by the HTTP handler after Reply succeeds.
	effective := in.Decision
	if effective == DecisionAlwaysAllow || effective == DecisionAllowSession || effective == DecisionAllow1h {
		effective = DecisionAllow
	}

	target := StateAllowed
	if effective == DecisionDeny {
		target = StateDenied
	}

	tx, err := g.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var current State
	var currentRepliedBy string
	if err := tx.QueryRow(ctx,
		`SELECT state, COALESCE(replied_by,'') FROM permission_requests WHERE id = $1 FOR UPDATE`,
		requestID,
	).Scan(&current, &currentRepliedBy); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	// Idempotency: same decision + same replier → success.
	if current == target && currentRepliedBy == in.RepliedBy {
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		return g.loadByID(ctx, requestID)
	}
	if current != StatePending {
		return nil, fmt.Errorf("permissions: cannot reply — request is %s", current)
	}

	var r Request
	err = tx.QueryRow(ctx, `
        UPDATE permission_requests
           SET state = $2,
               replied_by = $3,
               note = $4,
               replied_at = NOW()
         WHERE id = $1
        RETURNING id, COALESCE(session_id::text,''), COALESCE(plan_id::text,''),
                  COALESCE(node_id::text,''), COALESCE(agent_key,''),
                  tool, args, COALESCE(reason,''), state, COALESCE(requested_by,''),
                  COALESCE(replied_by,''), COALESCE(note,''), created_at, replied_at, expires_at
    `, requestID, target, in.RepliedBy, in.Note).Scan(
		&r.ID, &r.SessionID, &r.PlanID, &r.NodeID, &r.AgentKey,
		&r.Tool, &r.Args, &r.Reason, &r.State, &r.RequestedBy,
		&r.RepliedBy, &r.Note, &r.CreatedAt, &r.RepliedAt, &r.ExpiresAt,
	)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	// Signal any goroutine blocking on this request.
	g.mu.Lock()
	ch, ok := g.pending[requestID]
	g.mu.Unlock()
	if ok {
		select {
		case ch <- &r:
		default:
			// Channel already drained — means the blocker timed out
			// right before us. Not an error; the persisted row is
			// authoritative.
		}
	}

	// Emit permission.replied so every attached client sees the verdict
	// (not just the blocked goroutine). Actor is the replier for audit.
	if g.emitter != nil {
		_ = g.emitter.Emit(ctx, apievents.SinkAll, apievents.TypePermissionReplied,
			apievents.PermissionRepliedProps{
				RequestID: r.ID,
				Decision:  string(in.Decision),
				Note:      in.Note,
				Actor:     in.RepliedBy,
			})
	}
	return &r, nil
}

// expire flips a pending row to expired.
func (g *Gate) expire(ctx context.Context, requestID string) error {
	_, err := g.q(ctx).Exec(ctx, `
        UPDATE permission_requests
           SET state = 'expired', replied_at = NOW()
         WHERE id = $1 AND state = 'pending'
    `, requestID)
	return err
}

// loadByID reads the full Request row.
func (g *Gate) loadByID(ctx context.Context, id string) (*Request, error) {
	var r Request
	err := g.q(ctx).QueryRow(ctx, `
        SELECT id, COALESCE(session_id::text,''), COALESCE(plan_id::text,''),
               COALESCE(node_id::text,''), COALESCE(agent_key,''),
               tool, args, COALESCE(reason,''), state, COALESCE(requested_by,''),
               COALESCE(replied_by,''), COALESCE(note,''), created_at, replied_at, expires_at
        FROM permission_requests WHERE id = $1
    `, id).Scan(
		&r.ID, &r.SessionID, &r.PlanID, &r.NodeID, &r.AgentKey,
		&r.Tool, &r.Args, &r.Reason, &r.State, &r.RequestedBy,
		&r.RepliedBy, &r.Note, &r.CreatedAt, &r.RepliedAt, &r.ExpiresAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &r, nil
}

// Get returns a request by id.
func (g *Gate) Get(ctx context.Context, id string) (*Request, error) {
	return g.loadByID(ctx, id)
}

// ListPendingForSession returns pending requests for a session. Used by
// the TUI's /permissions command.
func (g *Gate) ListPendingForSession(ctx context.Context, sessionID string) ([]*Request, error) {
	rows, err := g.q(ctx).Query(ctx, `
        SELECT id, COALESCE(session_id::text,''), COALESCE(plan_id::text,''),
               COALESCE(node_id::text,''), COALESCE(agent_key,''),
               tool, args, COALESCE(reason,''), state, COALESCE(requested_by,''),
               COALESCE(replied_by,''), COALESCE(note,''), created_at, replied_at, expires_at
        FROM permission_requests
        WHERE session_id = $1::uuid AND state = 'pending'
        ORDER BY created_at ASC
    `, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Request{}
	for rows.Next() {
		var r Request
		if err := rows.Scan(
			&r.ID, &r.SessionID, &r.PlanID, &r.NodeID, &r.AgentKey,
			&r.Tool, &r.Args, &r.Reason, &r.State, &r.RequestedBy,
			&r.RepliedBy, &r.Note, &r.CreatedAt, &r.RepliedAt, &r.ExpiresAt,
		); err != nil {
			return nil, err
		}
		out = append(out, &r)
	}
	return out, rows.Err()
}

// ErrNotFound is returned by Get/Reply when no matching request exists.
var ErrNotFound = errors.New("permissions: not found")

// decodeArgsMap parses the args JSON into a map[string]any for
// event payloads. Returns an empty map on failure — this must never
// break the emission path.
func decodeArgsMap(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	return m
}
