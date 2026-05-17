// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

/**
 * Typed event contracts for the orchestrator's telemetry stream.
 *
 * Mirrors backend/internal/api/events/types.go:55-110 and :287-493.
 * Keep this file in sync — a new event kind on the server is invisible
 * to the UI until it's added here.
 *
 * ## Envelope
 *
 * The realtime hub broadcasts JSON with the shape:
 *   { type, session_id?, agent_id?, data?, timestamp, seq? }
 *
 * The UI only cares about five of these:
 *
 *   agent.progress        — free-form progress (tool_start, text, etc.)
 *   graph.node_started    — a plan node entered `running`
 *   graph.node_completed  — a plan node completed successfully
 *   graph.node_paused     — a plan node blocked pending approval
 *   graph.node_failed     — a plan node errored out terminally
 *
 * Everything else on the hub (soul_activity, new_message, stream_*)
 * is handled by the existing websocket.ts dispatch.
 */

// ────────── Event type constants ──────────

export const EVT_AGENT_PROGRESS = 'agent.progress' as const;
export const EVT_GRAPH_NODE_STARTED = 'graph.node_started' as const;
export const EVT_GRAPH_NODE_COMPLETED = 'graph.node_completed' as const;
export const EVT_GRAPH_NODE_PAUSED = 'graph.node_paused' as const;
export const EVT_GRAPH_NODE_FAILED = 'graph.node_failed' as const;
export const EVT_PERMISSION_REQUESTED = 'permission.requested' as const;
export const EVT_PERMISSION_REPLIED = 'permission.replied' as const;

export const TELEMETRY_EVENT_TYPES = [
  EVT_AGENT_PROGRESS,
  EVT_GRAPH_NODE_STARTED,
  EVT_GRAPH_NODE_COMPLETED,
  EVT_GRAPH_NODE_PAUSED,
  EVT_GRAPH_NODE_FAILED,
] as const;

// permission.* events live in their own bucket — they drive a
// different UI affordance (inline approval card, not a log line),
// and websocket.ts dispatches them separately from the telemetry
// stream.
export const PERMISSION_EVENT_TYPES = [
  EVT_PERMISSION_REQUESTED,
  EVT_PERMISSION_REPLIED,
] as const;

export type TelemetryEventType = (typeof TELEMETRY_EVENT_TYPES)[number];
export type PermissionEventType = (typeof PERMISSION_EVENT_TYPES)[number];

export function isTelemetryEventType(t: string): t is TelemetryEventType {
  return (TELEMETRY_EVENT_TYPES as readonly string[]).includes(t);
}
export function isPermissionEventType(t: string): t is PermissionEventType {
  return (PERMISSION_EVENT_TYPES as readonly string[]).includes(t);
}

// ────────── Per-type data shapes ──────────

/** Common fields on every graph.node_* event. */
export interface GraphNodeBase {
  plan_id: string;
  node_id: string;
  /** planner | human_feedback | agent_task | …  */
  kind?: string;
  title?: string;
  /** populated for agent_task nodes */
  agent_key?: string;
}

export interface GraphNodeStartedProps extends GraphNodeBase {}

export interface GraphNodeCompletedProps extends GraphNodeBase {
  outcome?: string;
  artifacts_excerpt?: Record<string, unknown>;
}

export interface GraphNodePausedProps extends GraphNodeBase {
  reason?: string;
  /** uuid of the approvals row, when this pause is an approval gate */
  approval_id?: string;
}

export interface GraphNodeFailedProps extends GraphNodeBase {
  error: string;
}

export interface AgentProgressProps {
  project_id?: string;
  agent_key: string;
  /** file_created | tool_start | tool_end | text | … */
  kind: string;
  detail?: Record<string, unknown>;
}

/** Emitted by the permission gate when a destructive tool asks for
 *  user consent. Mirrors backend PermissionRequestedProps (types.go:422). */
export interface PermissionRequestedProps {
  request_id: string;
  session_id: string;
  agent_key?: string;
  tool: string;
  args: Record<string, unknown>;
  reason?: string;
  /** When > 0, client may auto-approve after N ms of idle. */
  auto_approve_after_ms?: number;
}

/** Emitted when a permission request resolves. The same request_id
 *  that appeared on a prior `permission.requested` gets a verdict
 *  here. Mirrors backend PermissionRepliedProps (types.go:435). */
export interface PermissionRepliedProps {
  request_id: string;
  decision: 'allow' | 'allow_always' | 'deny';
  note?: string;
  actor?: string;
}

// ────────── Discriminated union ──────────

interface EnvelopeCommon {
  timestamp: number;
  seq?: number;
  session_id?: string;
  agent_id?: string;
}

export type TelemetryEvent =
  | (EnvelopeCommon & { type: typeof EVT_AGENT_PROGRESS; data: AgentProgressProps })
  | (EnvelopeCommon & { type: typeof EVT_GRAPH_NODE_STARTED; data: GraphNodeStartedProps })
  | (EnvelopeCommon & { type: typeof EVT_GRAPH_NODE_COMPLETED; data: GraphNodeCompletedProps })
  | (EnvelopeCommon & { type: typeof EVT_GRAPH_NODE_PAUSED; data: GraphNodePausedProps })
  | (EnvelopeCommon & { type: typeof EVT_GRAPH_NODE_FAILED; data: GraphNodeFailedProps });
