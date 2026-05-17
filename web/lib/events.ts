// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

/**
 * Qorven canonical event envelope + typed dispatcher.
 *
 * Mirrors backend/internal/api/events/types.go. Every event that the
 * gateway emits over SSE is decoded here as a discriminated union; call
 * sites switch on `.type` and TypeScript narrows the `.properties`
 * payload automatically.
 *
 * Any change to the backend taxonomy must be reflected here AND in
 * §9.3 of the app-builder plan. The two files drift together; CI
 * should grep the Go source for EventType constants and diff against
 * this file to prevent silent drift.
 */

// ─────────────────────────── Type discriminator ──────────────────────────

export type EventType =
  // Session lifecycle
  | 'session.created'
  | 'session.updated'
  | 'session.idle'
  | 'session.error'
  | 'session.cancelled'
  // Messages
  | 'message.updated'
  | 'message.part.updated'
  | 'message.part.removed'
  // Plan lifecycle
  | 'plan.proposed'
  | 'plan.approved'
  | 'plan.rejected'
  | 'plan.revision_requested'
  // Sub-agents
  | 'agent.spawned'
  | 'agent.started'
  | 'agent.progress'
  | 'agent.completed'
  | 'agent.error'
  // Rooms
  | 'room.posted'
  | 'room.decision'
  // Files
  | 'file.edited'
  | 'file.watcher.updated'
  // Build pipeline
  | 'build.phase'
  | 'build.progress'
  // GitHub
  | 'github.repo_created'
  | 'github.pr_opened'
  | 'github.ci_status'
  | 'github.pr_ready'
  | 'github.commit_pending'
  // Preview
  | 'preview.ready'
  // LSP
  | 'lsp.diagnostics'
  // Permission gate
  | 'permission.requested'
  | 'permission.replied'
  // Todo tracker
  | 'todo.updated'
  // Graph-runtime node lifecycle (Phase 3, FU-025). During the
  // migration window these fire in parallel with agent.progress;
  // dedupe on envelope id.
  | 'graph.node_started'
  | 'graph.node_completed'
  | 'graph.node_paused'
  | 'graph.node_failed';

// Allow server-emitted legacy types during the Phase 1 migration window.
// They arrive on the stream namespaced under "legacy." for anything that
// isn't yet mapped.
export type UnknownType = `legacy.${string}` | `${string}.${string}`;

// ─────────────────────────── Payload shapes ──────────────────────────────

export interface SessionCreatedProps {
  session_id: string;
  agent_id: string;
  channel?: string;
}
export interface SessionUpdatedProps {
  session_id: string;
  changes: Record<string, unknown>;
}
export interface SessionIdleProps {
  session_id: string;
  actor?: string;
}
export interface SessionErrorProps {
  session_id: string;
  message: string;
  code?: string;
  severity?: 'warn' | 'error' | 'fatal';
}
export interface SessionCancelledProps {
  session_id: string;
  actor?: string;
  reason?: string;
  /** "user_abort" | "admin_abort" | "timeout" | "shutdown" */
  code?: string;
}

export interface MessageUpdatedProps {
  session_id: string;
  message_id: string;
  role: string;
  model_id?: string;
  cost?: Record<string, unknown>;
  tokens?: Record<string, unknown>;
}

export interface MessagePartUpdatedProps {
  message_id: string;
  part_id: string;
  kind: 'text' | 'file' | 'tool_call' | 'tool_result' | 'snapshot';
  order: number;
  payload: unknown;
  final?: boolean;
}
export interface MessagePartRemovedProps {
  message_id: string;
  part_id: string;
}

export interface PlanProposedProps {
  project_id: string;
  plan_id: string;
  plan: unknown;
  raw?: string;
  summary?: string;
}
export interface PlanApprovedProps {
  project_id: string;
  plan_id: string;
  actor?: string;
}
export interface PlanRejectedProps {
  project_id: string;
  plan_id: string;
  actor?: string;
  comment?: string;
}
export interface PlanRevisionRequestedProps {
  project_id: string;
  plan_id: string;
  actor?: string;
  comment: string;
}

export interface AgentSpawnedProps {
  project_id: string;
  plan_id?: string;
  agent_key: string;
  role: string;
  model?: string;
}
export interface AgentStartedProps {
  project_id: string;
  plan_id?: string;
  agent_key: string;
  role?: string;
  task?: string;
}
export interface AgentProgressProps {
  project_id?: string;
  agent_key: string;
  kind: 'file_created' | 'tool_start' | 'tool_end' | 'text' | string;
  detail?: Record<string, unknown>;
}
export interface AgentCompletedProps {
  project_id?: string;
  agent_key: string;
  summary?: string;
}
export interface AgentErrorProps {
  project_id?: string;
  agent_key: string;
  error: string;
  fatal?: boolean;
}

export interface RoomPostedProps {
  room_id: string;
  room_name?: string;
  project_id?: string;
  author: string;
  author_is: 'user' | 'agent';
  body: string;
  ts?: number;
}
export interface RoomDecisionProps {
  room_id: string;
  decision: string;
  rationale?: string;
  actor?: string;
}

export interface FileEditedProps {
  project_id: string;
  path: string;
  diff?: string;
  snapshot_id?: string;
  bytes_after?: number;
  actor?: string;
}
export interface FileWatcherUpdatedProps {
  project_id: string;
  changed: string[];
  added?: string[];
  removed?: string[];
}

export interface BuildPhaseProps {
  project_id?: string;
  phase: string;
  preview_url?: string;
  merged?: boolean;
}
export interface BuildProgressProps {
  project_id: string;
  phase: string;
  fraction: number;
  label?: string;
}

export interface GitHubRepoCreatedProps {
  project_id: string;
  repo: string;
  html_url?: string;
  private?: boolean;
}
export interface GitHubPROpenedProps {
  project_id?: string;
  repo?: string;
  number?: number;
  html_url?: string;
  title?: string;
  // legacy shape also carried this
  output?: string;
}
export interface GitHubCIStatusProps {
  project_id?: string;
  repo?: string;
  pr_number?: number;
  status: string;
  conclusion: string;
  // legacy fields:
  pr?: string;
}

export interface GitHubPRReadyProps {
  pr_number: number;
  pr_title: string;
  pr_url: string;
  head_branch: string;
  base_branch: string;
  diff_additions: number;
  diff_deletions: number;
  ci_status: string;
  owner: string;
  repo: string;
  agent_id?: string;
  approval_id?: string;
  session_id?: string;
}

export interface GitHubCommitPendingProps {
  files: { path: string; additions: number; deletions: number }[];
  commit_message: string;
  branch: string;
  approval_id?: string;
  agent_id?: string;
  session_id?: string;
}

export interface PreviewReadyProps {
  project_id?: string;
  url: string;
  framework?: string;
}

export interface Diagnostic {
  line: number;
  column: number;
  end_line?: number;
  end_column?: number;
  severity: 'error' | 'warn' | 'info' | 'hint';
  message: string;
  code?: string;
}
export interface LSPDiagnosticsProps {
  project_id: string;
  path: string;
  diagnostics: Diagnostic[];
  source?: string;
}

export interface PermissionRequestedProps {
  request_id: string;
  session_id: string;
  agent_key?: string;
  tool: string;
  args: Record<string, unknown>;
  reason?: string;
  auto_approve_after_ms?: number;
}
export interface PermissionRepliedProps {
  request_id: string;
  decision: 'allow' | 'allow_always' | 'deny';
  note?: string;
  actor?: string;
}

export interface TodoUpdatedProps {
  task_id: string;
  subject?: string;
  status: string;
  changed?: Record<string, unknown>;
}

export interface GraphNodeBase {
  plan_id: string;
  node_id: string;
  kind?: string;
  title?: string;
  agent_key?: string;
}
export type GraphNodeStartedProps = GraphNodeBase;
export interface GraphNodeCompletedProps extends GraphNodeBase {
  outcome?: string;
  artifacts_excerpt?: Record<string, unknown>;
}
export interface GraphNodePausedProps extends GraphNodeBase {
  reason?: string;
  approval_id?: string;
}
export interface GraphNodeFailedProps extends GraphNodeBase {
  error: string;
}

// ───────────────────────── Discriminated union ───────────────────────────

export type Event =
  | { type: 'session.created'; properties: SessionCreatedProps; id?: string; ts?: number }
  | { type: 'session.updated'; properties: SessionUpdatedProps; id?: string; ts?: number }
  | { type: 'session.idle'; properties: SessionIdleProps; id?: string; ts?: number }
  | { type: 'session.error'; properties: SessionErrorProps; id?: string; ts?: number }
  | { type: 'session.cancelled'; properties: SessionCancelledProps; id?: string; ts?: number }
  | { type: 'message.updated'; properties: MessageUpdatedProps; id?: string; ts?: number }
  | { type: 'message.part.updated'; properties: MessagePartUpdatedProps; id?: string; ts?: number }
  | { type: 'message.part.removed'; properties: MessagePartRemovedProps; id?: string; ts?: number }
  | { type: 'plan.proposed'; properties: PlanProposedProps; id?: string; ts?: number }
  | { type: 'plan.approved'; properties: PlanApprovedProps; id?: string; ts?: number }
  | { type: 'plan.rejected'; properties: PlanRejectedProps; id?: string; ts?: number }
  | { type: 'plan.revision_requested'; properties: PlanRevisionRequestedProps; id?: string; ts?: number }
  | { type: 'agent.spawned'; properties: AgentSpawnedProps; id?: string; ts?: number }
  | { type: 'agent.started'; properties: AgentStartedProps; id?: string; ts?: number }
  | { type: 'agent.progress'; properties: AgentProgressProps; id?: string; ts?: number }
  | { type: 'agent.completed'; properties: AgentCompletedProps; id?: string; ts?: number }
  | { type: 'agent.error'; properties: AgentErrorProps; id?: string; ts?: number }
  | { type: 'room.posted'; properties: RoomPostedProps; id?: string; ts?: number }
  | { type: 'room.decision'; properties: RoomDecisionProps; id?: string; ts?: number }
  | { type: 'file.edited'; properties: FileEditedProps; id?: string; ts?: number }
  | { type: 'file.watcher.updated'; properties: FileWatcherUpdatedProps; id?: string; ts?: number }
  | { type: 'build.phase'; properties: BuildPhaseProps; id?: string; ts?: number }
  | { type: 'build.progress'; properties: BuildProgressProps; id?: string; ts?: number }
  | { type: 'github.repo_created'; properties: GitHubRepoCreatedProps; id?: string; ts?: number }
  | { type: 'github.pr_opened'; properties: GitHubPROpenedProps; id?: string; ts?: number }
  | { type: 'github.ci_status'; properties: GitHubCIStatusProps; id?: string; ts?: number }
  | { type: 'github.pr_ready'; properties: GitHubPRReadyProps; id?: string; ts?: number }
  | { type: 'github.commit_pending'; properties: GitHubCommitPendingProps; id?: string; ts?: number }
  | { type: 'preview.ready'; properties: PreviewReadyProps; id?: string; ts?: number }
  | { type: 'lsp.diagnostics'; properties: LSPDiagnosticsProps; id?: string; ts?: number }
  | { type: 'permission.requested'; properties: PermissionRequestedProps; id?: string; ts?: number }
  | { type: 'permission.replied'; properties: PermissionRepliedProps; id?: string; ts?: number }
  | { type: 'todo.updated'; properties: TodoUpdatedProps; id?: string; ts?: number }
  | { type: 'graph.node_started'; properties: GraphNodeStartedProps; id?: string; ts?: number }
  | { type: 'graph.node_completed'; properties: GraphNodeCompletedProps; id?: string; ts?: number }
  | { type: 'graph.node_paused'; properties: GraphNodePausedProps; id?: string; ts?: number }
  | { type: 'graph.node_failed'; properties: GraphNodeFailedProps; id?: string; ts?: number };

export interface UnknownEvent {
  type: UnknownType | string;
  properties?: unknown;
  id?: string;
  ts?: number;
}

export type AnyEvent = Event | UnknownEvent;

// ─────────────────────────── Parse helpers ───────────────────────────────

/**
 * Attempt to parse a single line of SSE `data:` payload into a typed
 * envelope. Returns null on unrecognizable input. Errors are surfaced via
 * the onError callback rather than thrown — individual bad events must
 * never tear down the whole stream.
 */
export function parseEnvelope(
  dataLine: string,
  onError?: (err: unknown, raw: string) => void,
): AnyEvent | null {
  const trimmed = dataLine.trim();
  if (!trimmed || trimmed === '[DONE]') return null;
  try {
    const obj = JSON.parse(trimmed);
    if (obj && typeof obj === 'object' && typeof obj.type === 'string') {
      return obj as AnyEvent;
    }
    // Legacy shape: { type, data: {...} } — wrap into envelope so callers
    // handle one shape.
    if (obj && typeof obj.type === 'string' && 'data' in obj) {
      return { type: obj.type, properties: obj.data, id: undefined };
    }
    return null;
  } catch (err) {
    onError?.(err, dataLine);
    return null;
  }
}

/** Returns true if the given `type` is one of the known namespaced types. */
export function isKnownEventType(t: string): t is EventType {
  return KNOWN_EVENT_TYPES.has(t as EventType);
}

const KNOWN_EVENT_TYPES = new Set<EventType>([
  'session.created',
  'session.updated',
  'session.idle',
  'session.error',
  'session.cancelled',
  'message.updated',
  'message.part.updated',
  'message.part.removed',
  'plan.proposed',
  'plan.approved',
  'plan.rejected',
  'plan.revision_requested',
  'agent.spawned',
  'agent.started',
  'agent.progress',
  'agent.completed',
  'agent.error',
  'room.posted',
  'room.decision',
  'file.edited',
  'file.watcher.updated',
  'build.phase',
  'build.progress',
  'github.repo_created',
  'github.pr_opened',
  'github.ci_status',
  'github.pr_ready',
  'github.commit_pending',
  'preview.ready',
  'lsp.diagnostics',
  'permission.requested',
  'permission.replied',
  'todo.updated',
  'graph.node_started',
  'graph.node_completed',
  'graph.node_paused',
  'graph.node_failed',
]);

// ─────────────────────────── SSE consumer ────────────────────────────────

export interface ConsumeOptions {
  /** Called for every envelope, including legacy and unknown. */
  onEvent: (evt: AnyEvent) => void;
  /** Called when the stream terminates cleanly (either EOF or [DONE]). */
  onDone?: () => void;
  /** Called on fetch or decode errors. The consumer does not re-throw. */
  onError?: (err: unknown) => void;
  /** Abort signal — cancels the fetch. */
  signal?: AbortSignal;
  /** Authorization header override. Defaults to reading qorven_token from localStorage. */
  authToken?: string;
}

/**
 * Consume an SSE response body, dispatching each envelope to the caller.
 * Handles the `data: `, blank-line framing, and comment lines. Legacy
 * `{type, data}` frames are upgraded to `{type, properties}` on the fly
 * so call-site switches don't have to know about the Phase 1 migration.
 *
 * This is a streaming consumer — it returns after the body closes.
 */
export async function consumeEventStream(
  response: Response,
  opts: ConsumeOptions,
): Promise<void> {
  if (!response.body) {
    opts.onError?.(new Error('response has no body'));
    return;
  }
  const reader = response.body.getReader();
  const decoder = new TextDecoder('utf-8');
  let buffer = '';
  // Per-event accumulator: multiple `data:` lines in one event must be
  // newline-joined before JSON.parse.
  let dataLines: string[] = [];

  const dispatch = () => {
    if (dataLines.length === 0) return;
    const joined = dataLines.join('\n');
    dataLines = [];
    const env = parseEnvelope(joined, opts.onError);
    if (env) opts.onEvent(env);
  };

  try {
    // eslint-disable-next-line no-constant-condition
    while (true) {
      const { value, done } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });
      let idx: number;
      // Split on \n — SSE allows \r\n, \n, or \r; normalize \r to \n first.
      buffer = buffer.replace(/\r\n/g, '\n').replace(/\r/g, '\n');
      while ((idx = buffer.indexOf('\n')) >= 0) {
        const line = buffer.slice(0, idx);
        buffer = buffer.slice(idx + 1);
        if (line === '') {
          dispatch();
          continue;
        }
        if (line.startsWith(':')) continue; // comment
        const colon = line.indexOf(':');
        const field = colon >= 0 ? line.slice(0, colon) : line;
        let value = colon >= 0 ? line.slice(colon + 1) : '';
        if (value.startsWith(' ')) value = value.slice(1);
        if (field === 'data') {
          dataLines.push(value);
        }
        // event/id/retry are ignored — the envelope carries its own type.
      }
    }
    // Flush any trailing event without a final blank line.
    dispatch();
    opts.onDone?.();
  } catch (err) {
    opts.onError?.(err);
  } finally {
    try {
      reader.releaseLock();
    } catch {
      /* reader already closed */
    }
  }
}

// ────────────────────────── EventSource helper ───────────────────────────

/**
 * POST to an SSE endpoint and dispatch events with `consumeEventStream`.
 * Prefer this for POST-driven streams; for GET you can use native
 * EventSource and call `dispatchEventSourceMessage` on each `message`.
 */
export async function postEventStream(
  url: string,
  body: unknown,
  opts: Omit<ConsumeOptions, 'onEvent'> & { onEvent: ConsumeOptions['onEvent'] },
): Promise<void> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    Accept: 'text/event-stream',
  };
  const token =
    opts.authToken ?? (typeof window !== 'undefined' ? localStorage.getItem('qorven_token') : null);
  if (token) headers.Authorization = `Bearer ${token}`;

  const resp = await fetch(url, {
    method: 'POST',
    headers,
    body: JSON.stringify(body),
    signal: opts.signal,
    credentials: 'include',
  });

  if (!resp.ok) {
    opts.onError?.(new Error(`stream POST failed: ${resp.status} ${resp.statusText}`));
    return;
  }
  await consumeEventStream(resp, opts);
}

/** Small helper for native EventSource consumers. */
export function dispatchEventSourceMessage(
  ev: MessageEvent<string>,
  onEvent: (evt: AnyEvent) => void,
  onError?: (err: unknown, raw: string) => void,
) {
  const env = parseEnvelope(ev.data, onError);
  if (env) onEvent(env);
}
