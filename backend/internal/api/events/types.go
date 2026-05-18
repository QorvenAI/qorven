// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

// Package events defines the canonical Qorven SSE event taxonomy and
// envelope shape. Every event emitted by the gateway flows as:
//
//	{"type": "<domain>.<verb>", "properties": { ... }}
//
// The discriminator lives on the envelope, not on the SSE "event:" field.
// Clients (web /code page + bubbletea TUI + opencode-sdk-go-alike) decode
// the envelope via ssestream.Stream[Envelope] and switch on Type.
//
// This file is the single source of truth. Adding a new event requires:
//  1. Adding a Type constant below.
//  2. Defining its properties struct.
//  3. Registering it in typeRegistry (below).
//  4. Updating web/lib/events.ts with the mirrored TypeScript union.
//  5. Updating §9.3 of the app-builder plan.
package events

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Type is the namespaced event discriminator. Values are immutable once
// published — never rename; deprecate and add a new one instead.
type Type string

// Every event Qorven emits. Organized by domain. Matches the taxonomy in
// QORVEN-APP-BUILDER-DEEP-PLAN.md §A2. Thirty types cover the Phase 1
// surface; more may be appended.
const (
	// Session lifecycle
	TypeSessionCreated   Type = "session.created"
	TypeSessionUpdated   Type = "session.updated"
	TypeSessionIdle      Type = "session.idle"
	TypeSessionError     Type = "session.error"
	// TypeSessionCancelled is emitted when a user-initiated abort cancels
	// a running turn. Distinct from session.error so consumers can tell
	// "the user clicked stop" apart from "the provider failed".
	TypeSessionCancelled Type = "session.cancelled"

	// Messages and streaming parts
	TypeMessageUpdated     Type = "message.updated"
	TypeMessagePartUpdated Type = "message.part.updated"
	TypeMessagePartRemoved Type = "message.part.removed"

	// Plan lifecycle
	TypePlanProposed          Type = "plan.proposed"
	TypePlanApproved          Type = "plan.approved"
	TypePlanRejected          Type = "plan.rejected"
	TypePlanRevisionRequested Type = "plan.revision_requested"

	// Sub-agent lifecycle
	TypeAgentSnapshot  Type = "agent.snapshot"
	TypeAgentSpawned   Type = "agent.spawned"
	TypeAgentStarted   Type = "agent.started"
	TypeAgentProgress  Type = "agent.progress"
	TypeAgentCompleted Type = "agent.completed"
	TypeAgentError     Type = "agent.error"

	// Room (team discussion)
	TypeRoomPosted   Type = "room.posted"
	TypeRoomDecision Type = "room.decision"

	// File and workspace
	TypeFileEdited         Type = "file.edited"
	TypeFileWatcherUpdated Type = "file.watcher.updated"

	// Build pipeline
	TypeBuildPhase    Type = "build.phase"
	TypeBuildProgress Type = "build.progress"

	// GitHub
	TypeGitHubRepoCreated Type = "github.repo_created"
	TypeGitHubPROpened    Type = "github.pr_opened"
	TypeGitHubPRReady     Type = "github.pr_ready"   // webhook-triggered: PR needs human review
	TypeGitHubCIStatus    Type = "github.ci_status"
	TypeGitHubCommitPending Type = "github.commit_pending" // agent wants to push files

	// Preview
	TypePreviewReady Type = "preview.ready"

	// LSP
	TypeLSPDiagnostics Type = "lsp.diagnostics"

	// Permission gate
	TypePermissionRequested Type = "permission.requested"
	TypePermissionReplied   Type = "permission.replied"

	// Todo / task tracker
	TypeTodoUpdated Type = "todo.updated"

	// Graph-runtime node lifecycle (Phase 3, FU-025).
	// Distinct from agent.progress so clients can observe graph
	// traversal without overloading the agent-telemetry bucket.
	// During the Phase 3 migration window both agent.progress and
	// graph.node_* fire per event; clients are expected to dedupe
	// via the envelope id.
	TypeGraphNodeStarted   Type = "graph.node_started"
	TypeGraphNodeCompleted Type = "graph.node_completed"
	TypeGraphNodePaused    Type = "graph.node_paused"
	TypeGraphNodeFailed    Type = "graph.node_failed"
)

// Envelope is the outer shape of every event on the wire. Properties is a
// typed structure per event; we carry it as json.RawMessage on decode so
// clients can dispatch on Type before allocating the concrete payload.
type Envelope struct {
	Type       Type            `json:"type"`
	Properties json.RawMessage `json:"properties,omitempty"`

	// ID is an optional monotonic event id for clients that want to resume
	// from a particular point in the stream. The server assigns it at emit
	// time; clients treat it as opaque.
	ID string `json:"id,omitempty"`

	// EmittedAtMS is unix-milliseconds of emit time; useful for latency
	// measurement. Clients should not rely on strict monotonicity across
	// concurrent senders.
	EmittedAtMS int64 `json:"ts,omitempty"`
}

// Encode marshals a typed property struct into the envelope payload.
func (e *Envelope) Encode(props any) error {
	if props == nil {
		e.Properties = nil
		return nil
	}
	b, err := json.Marshal(props)
	if err != nil {
		return fmt.Errorf("events: encode %s: %w", e.Type, err)
	}
	e.Properties = b
	return nil
}

// Decode unmarshals the envelope payload into dst.
func (e *Envelope) Decode(dst any) error {
	if len(e.Properties) == 0 {
		return errors.New("events: empty properties")
	}
	if err := json.Unmarshal(e.Properties, dst); err != nil {
		return fmt.Errorf("events: decode %s: %w", e.Type, err)
	}
	return nil
}

// NewEnvelope constructs an envelope with the given type and properties.
// Returns an error if properties fail to marshal.
func NewEnvelope(t Type, props any) (Envelope, error) {
	env := Envelope{Type: t}
	if err := env.Encode(props); err != nil {
		return Envelope{}, err
	}
	return env, nil
}

// ─────────────────────────── property structs ────────────────────────────

// SessionCreatedProps fires once per new session.
type SessionCreatedProps struct {
	SessionID string `json:"session_id"`
	AgentID   string `json:"agent_id"`
	Channel   string `json:"channel,omitempty"`
}

// SessionUpdatedProps fires when session metadata changes (model switch, etc).
type SessionUpdatedProps struct {
	SessionID string         `json:"session_id"`
	Changes   map[string]any `json:"changes"`
}

// SessionIdleProps marks the end of a streamed turn. Clients typically
// re-enable input on this event. Actor carries the actor whose submit
// initiated the turn, so audit trails can link idle to submit.
type SessionIdleProps struct {
	SessionID string `json:"session_id"`
	Actor     string `json:"actor,omitempty"`
}

// SessionErrorProps reports a session-level failure. The caller must
// distinguish soft errors (invalid input) from hard errors (provider
// outage) via Severity.
type SessionErrorProps struct {
	SessionID string `json:"session_id"`
	Message   string `json:"message"`
	Code      string `json:"code,omitempty"`
	Severity  string `json:"severity,omitempty"` // "warn" | "error" | "fatal"
}

// SessionCancelledProps reports a user- or admin-initiated abort. The
// running agent goroutine has received a cancel signal by the time this
// event is emitted. Actor carries the authenticated identity that
// issued the abort (tenant/user id); Reason carries the optional note.
type SessionCancelledProps struct {
	SessionID string `json:"session_id"`
	Actor     string `json:"actor,omitempty"`
	Reason    string `json:"reason,omitempty"`
	// Code is one of: "user_abort" | "admin_abort" | "timeout" | "shutdown".
	Code string `json:"code,omitempty"`
}

// MessageUpdatedProps is fired whenever a message's top-level state changes
// (e.g., cost rollup, model id assignment).
type MessageUpdatedProps struct {
	SessionID string         `json:"session_id"`
	MessageID string         `json:"message_id"`
	Role      string         `json:"role"`
	ModelID   string         `json:"model_id,omitempty"`
	Cost      map[string]any `json:"cost,omitempty"`
	Tokens    map[string]any `json:"tokens,omitempty"`
}

// MessagePartUpdatedProps carries an incremental part update. The part's
// Payload shape is determined by Kind — text/file/tool_call/tool_result/snapshot.
// Clients merge by (MessageID, Order).
type MessagePartUpdatedProps struct {
	MessageID string          `json:"message_id"`
	PartID    string          `json:"part_id"`
	Kind      string          `json:"kind"` // "text"|"file"|"tool_call"|"tool_result"|"snapshot"
	Order     int             `json:"order"`
	Payload   json.RawMessage `json:"payload"`
	Final     bool            `json:"final,omitempty"` // last chunk for this part
}

// MessagePartRemovedProps removes a previously emitted part (e.g., on revert).
type MessagePartRemovedProps struct {
	MessageID string `json:"message_id"`
	PartID    string `json:"part_id"`
}

// PlanProposedProps is emitted by the planner node. The client shows the
// approval gate. Plan carries the parsed planner output.
type PlanProposedProps struct {
	ProjectID string          `json:"project_id"`
	PlanID    string          `json:"plan_id"`
	Plan      json.RawMessage `json:"plan"`
	Raw       string          `json:"raw,omitempty"` // unparsed planner text when Plan is nil
	Summary   string          `json:"summary,omitempty"`
}

// PlanApprovedProps fires on user approval. Graph resumes.
type PlanApprovedProps struct {
	ProjectID string `json:"project_id"`
	PlanID    string `json:"plan_id"`
	Actor     string `json:"actor,omitempty"` // user id that approved
}

// PlanRejectedProps fires on hard rejection. Graph halts.
type PlanRejectedProps struct {
	ProjectID string `json:"project_id"`
	PlanID    string `json:"plan_id"`
	Actor     string `json:"actor,omitempty"`
	Comment   string `json:"comment,omitempty"`
}

// PlanRevisionRequestedProps fires when a user asks for edits. The plan
// state machine moves from `pending → revision_requested` and the planner
// node will re-run with Comment as additional context.
type PlanRevisionRequestedProps struct {
	ProjectID string `json:"project_id"`
	PlanID    string `json:"plan_id"`
	Actor     string `json:"actor,omitempty"`
	Comment   string `json:"comment"`
}

// AgentSpawnedProps fires when the orchestrator calls manage_agents to
// create a new specialist. Distinct from agent.started — spawn is
// infrastructure, start is first useful work.
type AgentSpawnedProps struct {
	ProjectID string `json:"project_id"`
	PlanID    string `json:"plan_id,omitempty"`
	AgentKey  string `json:"agent_key"`
	Role      string `json:"role"`
	Model     string `json:"model,omitempty"`
}

// AgentStartedProps marks the beginning of a sub-agent's run.
type AgentStartedProps struct {
	ProjectID string `json:"project_id"`
	PlanID    string `json:"plan_id,omitempty"`
	AgentKey  string `json:"agent_key"`
	Role      string `json:"role,omitempty"`
	Task      string `json:"task,omitempty"`
}

// AgentProgressProps carries per-agent progress (file written, tool call, etc).
// Free-form Detail keeps this open for any progress shape.
type AgentProgressProps struct {
	ProjectID string         `json:"project_id"`
	AgentKey  string         `json:"agent_key"`
	Kind      string         `json:"kind"` // "file_created"|"tool_start"|"tool_end"|"text"
	Detail    map[string]any `json:"detail,omitempty"`
}

// AgentCompletedProps marks clean termination of a sub-agent.
type AgentCompletedProps struct {
	ProjectID string `json:"project_id"`
	AgentKey  string `json:"agent_key"`
	Summary   string `json:"summary,omitempty"`
}

// AgentErrorProps marks a failed sub-agent. Fatal=true halts the plan.
type AgentErrorProps struct {
	ProjectID string `json:"project_id"`
	AgentKey  string `json:"agent_key"`
	Error     string `json:"error"`
	Fatal     bool   `json:"fatal,omitempty"`
}

// RoomPostedProps broadcasts a new message in a project/build room.
type RoomPostedProps struct {
	RoomID    string `json:"room_id"`
	RoomName  string `json:"room_name,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	Author    string `json:"author"`      // agent key or user id
	AuthorIs  string `json:"author_is"`   // "user" | "agent"
	Body      string `json:"body"`
	TS        int64  `json:"ts,omitempty"` // unix ms
}

// RoomDecisionProps fires when a room_decide tool resolves.
type RoomDecisionProps struct {
	RoomID    string `json:"room_id"`
	Decision  string `json:"decision"`
	Rationale string `json:"rationale,omitempty"`
	Actor     string `json:"actor,omitempty"`
}

// FileEditedProps emits on any write_file or apply_patch touching the
// workspace. Diff is unified-diff text. Snapshot references the diff store
// for revert.
type FileEditedProps struct {
	ProjectID  string `json:"project_id"`
	Path       string `json:"path"`
	Diff       string `json:"diff,omitempty"`
	SnapshotID string `json:"snapshot_id,omitempty"`
	BytesAfter int    `json:"bytes_after,omitempty"`
	Actor      string `json:"actor,omitempty"` // agent key
}

// FileWatcherUpdatedProps reports external filesystem changes (user edits,
// git operations) in the workspace.
type FileWatcherUpdatedProps struct {
	ProjectID string   `json:"project_id"`
	Changed   []string `json:"changed"`
	Added     []string `json:"added,omitempty"`
	Removed   []string `json:"removed,omitempty"`
}

// BuildPhaseProps reports the current phase in the build pipeline. Phase
// values are a closed enum documented in §B1 of the deep plan.
type BuildPhaseProps struct {
	ProjectID  string `json:"project_id"`
	Phase      string `json:"phase"`
	PreviewURL string `json:"preview_url,omitempty"`
	Merged     bool   `json:"merged,omitempty"`
}

// BuildProgressProps is a finer-grained progress within a phase. Fraction
// is [0,1]; Label is a short caption.
type BuildProgressProps struct {
	ProjectID string  `json:"project_id"`
	Phase     string  `json:"phase"`
	Fraction  float64 `json:"fraction"`
	Label     string  `json:"label,omitempty"`
}

// GitHubRepoCreatedProps fires after gh_create_repo completes.
type GitHubRepoCreatedProps struct {
	ProjectID string `json:"project_id"`
	Repo      string `json:"repo"`    // "owner/name"
	HTMLURL   string `json:"html_url,omitempty"`
	Private   bool   `json:"private,omitempty"`
}

// GitHubPROpenedProps fires after gh_open_pr completes.
type GitHubPROpenedProps struct {
	ProjectID string `json:"project_id"`
	Repo      string `json:"repo"`
	Number    int    `json:"number"`
	HTMLURL   string `json:"html_url,omitempty"`
	Title     string `json:"title,omitempty"`
}

// GitHubCIStatusProps fires on each CI poll. Conclusion is "pending" until
// the run resolves.
type GitHubCIStatusProps struct {
	ProjectID  string `json:"project_id"`
	Repo       string `json:"repo"`
	PRNumber   int    `json:"pr_number,omitempty"`
	Status     string `json:"status"`     // "queued"|"in_progress"|"completed"
	Conclusion string `json:"conclusion"` // "success"|"failure"|"neutral"|"cancelled"|"pending"
}

// GitHubPRReadyProps fires when a PR arrives via webhook and is ready for review.
// The web UI renders this as an inline approval card in Prime's chat feed.
type GitHubPRReadyProps struct {
	PRNumber       int    `json:"pr_number"`
	PRTitle        string `json:"pr_title"`
	PRURL          string `json:"pr_url"`
	HeadBranch     string `json:"head_branch"`
	BaseBranch     string `json:"base_branch"`
	DiffAdditions  int    `json:"diff_additions"`
	DiffDeletions  int    `json:"diff_deletions"`
	CIStatus       string `json:"ci_status"` // "pending"|"passing"|"failing"
	Owner          string `json:"owner"`
	Repo           string `json:"repo"`
	AgentID        string `json:"agent_id"`
	ApprovalID     string `json:"approval_id,omitempty"`
}

// GitHubCommitPendingProps fires when the agent wants to push one or more files.
// The web UI renders this as a Commit Approval card in chat.
type GitHubCommitPendingProps struct {
	Files         []CommitFileItem `json:"files"`
	CommitMessage string           `json:"commit_message"`
	Branch        string           `json:"branch"`
	ApprovalID    string           `json:"approval_id"`
	AgentID       string           `json:"agent_id"`
}

// CommitFileItem is a single file in a pending commit.
type CommitFileItem struct {
	Path      string `json:"path"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
}

// PreviewReadyProps fires when a web project's dev server is reachable.
type PreviewReadyProps struct {
	ProjectID string `json:"project_id"`
	URL       string `json:"url"`
	Framework string `json:"framework,omitempty"`
}

// LSPDiagnosticsProps aggregates LSP diagnostics for one file.
type LSPDiagnosticsProps struct {
	ProjectID   string        `json:"project_id"`
	Path        string        `json:"path"`
	Diagnostics []Diagnostic  `json:"diagnostics"`
	Source      string        `json:"source,omitempty"` // "gopls"|"typescript-language-server"|...
}

// Diagnostic is a single LSP diagnostic entry. Mirrors LSP's core shape.
type Diagnostic struct {
	Line      int    `json:"line"`
	Column    int    `json:"column"`
	EndLine   int    `json:"end_line,omitempty"`
	EndColumn int    `json:"end_column,omitempty"`
	Severity  string `json:"severity"` // "error"|"warn"|"info"|"hint"
	Message   string `json:"message"`
	Code      string `json:"code,omitempty"`
}

// PermissionRequestedProps asks the user to authorize a dangerous tool call.
// Until a matching PermissionRepliedProps comes back, the tool runner blocks.
type PermissionRequestedProps struct {
	RequestID string         `json:"request_id"`
	SessionID string         `json:"session_id"`
	AgentKey  string         `json:"agent_key,omitempty"`
	Tool      string         `json:"tool"`
	Args      map[string]any `json:"args"`
	Reason    string         `json:"reason,omitempty"`
	// AutoApproveAfter lets the client auto-approve after N ms when the
	// user is AFK. Zero means "block until replied".
	AutoApproveAfterMS int `json:"auto_approve_after_ms,omitempty"`
}

// PermissionRepliedProps is the user's verdict. Decision is "allow" | "deny".
type PermissionRepliedProps struct {
	RequestID string `json:"request_id"`
	Decision  string `json:"decision"` // "allow" | "deny"
	Note      string `json:"note,omitempty"`
	Actor     string `json:"actor,omitempty"`
}

// TodoUpdatedProps mirrors our task-tracking state. Kept open-shape because
// TodoCreate/Update already define their own schema in backend/internal/tasks.
type TodoUpdatedProps struct {
	TaskID  string         `json:"task_id"`
	Subject string         `json:"subject,omitempty"`
	Status  string         `json:"status"`
	Changed map[string]any `json:"changed,omitempty"`
}

// GraphNodeBase is the shared shape for every graph.node_* event. It's
// embedded by the lifecycle-specific props below so clients can match
// on PlanID/NodeID before drilling into kind-specific fields.
type GraphNodeBase struct {
	PlanID    string `json:"plan_id"`
	NodeID    string `json:"node_id"`
	Kind      string `json:"kind"`                // e.g. "planner" | "human_feedback" | "agent_task"
	Title     string `json:"title,omitempty"`
	AgentKey  string `json:"agent_key,omitempty"` // populated for agent_task nodes
	Actor     string `json:"actor,omitempty"`     // human who triggered the run (resolved_by from approval)
}

// GraphNodeStartedProps fires when the runtime transitions a node from
// pending (or blocked after a resume) into running.
type GraphNodeStartedProps struct {
	GraphNodeBase
}

// GraphNodeCompletedProps fires when a node terminates successfully.
// Outcome carries the handler's verdict (e.g. "on_success", "approved").
// ArtifactsExcerpt is a truncated view of the node's artifacts — full
// artifacts live on the plan_nodes row.
type GraphNodeCompletedProps struct {
	GraphNodeBase
	Outcome          string         `json:"outcome,omitempty"`
	ArtifactsExcerpt map[string]any `json:"artifacts_excerpt,omitempty"`
}

// GraphNodePausedProps fires when a node returns a PauseSignal.
// Reason is the human-readable pause tag; ApprovalID, when set, points
// at the approvals row blocking the resume.
type GraphNodePausedProps struct {
	GraphNodeBase
	Reason     string `json:"reason,omitempty"`
	ApprovalID string `json:"approval_id,omitempty"`
}

// GraphNodeFailedProps fires when a handler returns a fatal error (or
// OutcomeError with no on_error edge). Error is the short message;
// full detail is on the plan_nodes row.
type GraphNodeFailedProps struct {
	GraphNodeBase
	Error string `json:"error"`
}

// typeRegistry validates emit-time type/payload pairings. Adding a new event
// without registering it triggers a test failure in TestRegistryCoverage.
var typeRegistry = map[Type]func() any{
	TypeSessionCreated:        func() any { return &SessionCreatedProps{} },
	TypeSessionUpdated:        func() any { return &SessionUpdatedProps{} },
	TypeSessionIdle:           func() any { return &SessionIdleProps{} },
	TypeSessionError:          func() any { return &SessionErrorProps{} },
	TypeSessionCancelled:      func() any { return &SessionCancelledProps{} },
	TypeMessageUpdated:        func() any { return &MessageUpdatedProps{} },
	TypeMessagePartUpdated:    func() any { return &MessagePartUpdatedProps{} },
	TypeMessagePartRemoved:    func() any { return &MessagePartRemovedProps{} },
	TypePlanProposed:          func() any { return &PlanProposedProps{} },
	TypePlanApproved:          func() any { return &PlanApprovedProps{} },
	TypePlanRejected:          func() any { return &PlanRejectedProps{} },
	TypePlanRevisionRequested: func() any { return &PlanRevisionRequestedProps{} },
	TypeAgentSpawned:          func() any { return &AgentSpawnedProps{} },
	TypeAgentStarted:          func() any { return &AgentStartedProps{} },
	TypeAgentProgress:         func() any { return &AgentProgressProps{} },
	TypeAgentCompleted:        func() any { return &AgentCompletedProps{} },
	TypeAgentError:            func() any { return &AgentErrorProps{} },
	TypeRoomPosted:            func() any { return &RoomPostedProps{} },
	TypeRoomDecision:          func() any { return &RoomDecisionProps{} },
	TypeFileEdited:            func() any { return &FileEditedProps{} },
	TypeFileWatcherUpdated:    func() any { return &FileWatcherUpdatedProps{} },
	TypeBuildPhase:            func() any { return &BuildPhaseProps{} },
	TypeBuildProgress:         func() any { return &BuildProgressProps{} },
	TypeGitHubRepoCreated:     func() any { return &GitHubRepoCreatedProps{} },
	TypeGitHubPROpened:        func() any { return &GitHubPROpenedProps{} },
	TypeGitHubPRReady:         func() any { return &GitHubPRReadyProps{} },
	TypeGitHubCIStatus:        func() any { return &GitHubCIStatusProps{} },
	TypeGitHubCommitPending:   func() any { return &GitHubCommitPendingProps{} },
	TypePreviewReady:          func() any { return &PreviewReadyProps{} },
	TypeLSPDiagnostics:        func() any { return &LSPDiagnosticsProps{} },
	TypePermissionRequested:   func() any { return &PermissionRequestedProps{} },
	TypePermissionReplied:     func() any { return &PermissionRepliedProps{} },
	TypeTodoUpdated:           func() any { return &TodoUpdatedProps{} },
	TypeGraphNodeStarted:      func() any { return &GraphNodeStartedProps{} },
	TypeGraphNodeCompleted:    func() any { return &GraphNodeCompletedProps{} },
	TypeGraphNodePaused:       func() any { return &GraphNodePausedProps{} },
	TypeGraphNodeFailed:       func() any { return &GraphNodeFailedProps{} },
}

// IsKnown returns true if t is a registered type.
func IsKnown(t Type) bool {
	_, ok := typeRegistry[t]
	return ok
}

// AllTypes returns every registered type. Used by tests and for
// documentation generation.
func AllTypes() []Type {
	out := make([]Type, 0, len(typeRegistry))
	for t := range typeRegistry {
		out = append(out, t)
	}
	return out
}

// NewPropsFor returns a zero-valued pointer to the payload struct matching t,
// or nil if t is unknown. Used by generic decoders that want to typed-decode
// without a giant switch on the caller side.
func NewPropsFor(t Type) any {
	if f, ok := typeRegistry[t]; ok {
		return f()
	}
	return nil
}

// legacyAliases mapped pre-Phase-1 flat event names to canonical types.
// Emptied in Phase 3 (FU-019) — all emitters now emit canonical types
// directly. The map and CanonicalType() are retained so the de-dup
// logic in dualwire_integration_test.go compiles unchanged.
var legacyAliases = map[string]Type{
	"agent_snapshot": TypeAgentSnapshot,
}

// CanonicalType normalizes a type string. Namespaced names (containing
// a ".") pass through unchanged. Known legacy flat names map to their
// canonical namespaced Type. Unknown inputs are returned as-is (as a
// Type) so the caller's de-dup and handler paths still see a stable
// discriminator.
func CanonicalType(t string) Type {
	if t == "" {
		return ""
	}
	if strings.Contains(t, ".") {
		return Type(t)
	}
	if tt, ok := legacyAliases[t]; ok {
		return tt
	}
	return Type("legacy." + t)
}
