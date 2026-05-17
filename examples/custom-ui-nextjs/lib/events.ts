/**
 * Canonical event types that Qorven's realtime hub broadcasts over
 * /ws/realtime. Mirrors backend/internal/api/events/types.go:55-110.
 *
 * This file is the TYPE SYSTEM's source of truth for the wire format.
 * If the backend adds a new event type, add it here too — the
 * discriminated union below will force every consumer to handle it
 * (or explicitly skip via `default`) the next time the TS compiler
 * runs.
 *
 * ## Envelope shape (from hub.go)
 *
 *   {
 *     type: string,          // one of the constants below
 *     session_id?: string,
 *     agent_id?: string,
 *     data?: unknown,
 *     timestamp: number,     // unix ms
 *     seq: number            // monotonic — use for ordering
 *   }
 */

// ─────────────────── Constants ───────────────────

export const EVT_AGENT_PROGRESS = "agent.progress" as const;
export const EVT_GRAPH_NODE_STARTED = "graph.node_started" as const;
export const EVT_GRAPH_NODE_COMPLETED = "graph.node_completed" as const;
export const EVT_GRAPH_NODE_PAUSED = "graph.node_paused" as const;
export const EVT_GRAPH_NODE_FAILED = "graph.node_failed" as const;

// A pinned set the UI cares about. Everything else arriving over the
// wire is valid but ignored by the GraphVisualizer — your code can
// still receive and dispatch on it if you need other surfaces.
export const KNOWN_EVENT_TYPES = [
  EVT_AGENT_PROGRESS,
  EVT_GRAPH_NODE_STARTED,
  EVT_GRAPH_NODE_COMPLETED,
  EVT_GRAPH_NODE_PAUSED,
  EVT_GRAPH_NODE_FAILED,
] as const;

// ─────────────────── Per-type data shapes ───────────────────

/** Base props shared by every graph.node_* event. */
export type GraphNodeBaseProps = {
  plan_id: string;
  node_id: string;
  kind?: string;
  title?: string;
  agent_key?: string;
};

export type GraphNodeStartedProps = GraphNodeBaseProps;

export type GraphNodeCompletedProps = GraphNodeBaseProps & {
  outcome: string;
  artifacts_excerpt?: Record<string, unknown>;
};

export type GraphNodePausedProps = GraphNodeBaseProps & {
  reason: string;
  approval_id?: string;
};

export type GraphNodeFailedProps = GraphNodeBaseProps & {
  error: string;
};

export type AgentProgressProps = {
  agent_key: string;
  kind: string;
  detail?: Record<string, unknown>;
};

// ─────────────────── Discriminated union ───────────────────

export type QorvenEventEnvelope = {
  timestamp: number;
  seq?: number;
  session_id?: string;
  agent_id?: string;
};

export type QorvenEvent =
  | (QorvenEventEnvelope & { type: typeof EVT_AGENT_PROGRESS; data: AgentProgressProps })
  | (QorvenEventEnvelope & { type: typeof EVT_GRAPH_NODE_STARTED; data: GraphNodeStartedProps })
  | (QorvenEventEnvelope & { type: typeof EVT_GRAPH_NODE_COMPLETED; data: GraphNodeCompletedProps })
  | (QorvenEventEnvelope & { type: typeof EVT_GRAPH_NODE_PAUSED; data: GraphNodePausedProps })
  | (QorvenEventEnvelope & { type: typeof EVT_GRAPH_NODE_FAILED; data: GraphNodeFailedProps })
  | (QorvenEventEnvelope & { type: string; data?: unknown });  // fallback for unknown types
