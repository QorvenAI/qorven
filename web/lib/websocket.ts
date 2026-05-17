// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useStore } from '@/store';
import type { WSEvent, LiveEvent } from '@/types';
import {
  isTelemetryEventType,
  type TelemetryEvent,
  EVT_PERMISSION_REQUESTED,
  EVT_PERMISSION_REPLIED,
  type PermissionRequestedProps,
  type PermissionRepliedProps,
} from '@/lib/graph-events';
import type { GitHubPRReadyProps, GitHubCommitPendingProps } from '@/lib/events';
import { nextBackoffMs } from '@/lib/resilience';

let ws: WebSocket | null = null;
let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
// Attempt counter drives exponential backoff. Reset to 0 on successful
// connect; incremented on every close. Also surfaced to the store so
// the UI can show "Reconnecting… (attempt N)" in the banner.
let reconnectAttempt = 0;
let disconnectedAt: number | undefined = undefined;
// Online listener registered lazily — we don't want to add it during
// SSR (no window) and we only want one listener no matter how many
// times connectWebSocket() is called.
let onlineListenerInstalled = false;
// Discovered API URL — starts as the page origin (correct for the
// static-embedded production case) or the env default (correct for
// local dev where Next dev server ≠ backend port). Gets updated by
// discoverApiUrl() before every WS connect.
//
// Seed with NEXT_PUBLIC_API_URL when it is explicitly set — that means
// we're in dev mode with a separate backend. The page origin would be
// the Next.js dev server (wrong for WS). In production NEXT_PUBLIC_API_URL
// is unset so we correctly fall through to window.location.origin.
// currentApiUrl is the backend URL used for WebSocket connections.
//   production : page origin (Go binary serves static + API on same origin)
//   dev        : NEXT_PUBLIC_API_URL (Next.js rewrites proxy REST but NOT WS upgrades)
//
// REST calls in api.ts always use /api/v1 (proxied by Next.js rewrites).
// Only WebSocket and health probes need to hit the backend directly in dev.
let currentApiUrl = (() => {
  const envUrl = process.env.NEXT_PUBLIC_API_URL;
  if (envUrl) return envUrl.replace(/\/$/, '');
  if (typeof window !== 'undefined') return window.location.origin.replace(/\/$/, '');
  return 'http://localhost:4200';
})();

function getWsToken() { return typeof window !== 'undefined' ? (localStorage.getItem('qorven_token') || process.env.NEXT_PUBLIC_API_TOKEN || '') : ''; }

function wsUrl(): string {
  return typeof window !== 'undefined'
    ? `${currentApiUrl.replace(/^http/, 'ws')}/ws/realtime`
    : '';
}

// discoverApiUrl — confirms the backend URL is still reachable.
// In dev (NEXT_PUBLIC_API_URL set) we already point at the backend directly,
// so just verify it's up. In production the backend serves the static bundle
// from the same origin, so we probe the page origin.
async function discoverApiUrl(): Promise<void> {
  if (typeof window === 'undefined') return;
  const isSecurePage = window.location.protocol === 'https:';
  // In dev, NEXT_PUBLIC_API_URL is set and currentApiUrl already points at
  // the backend — Next.js cannot proxy WS upgrades through rewrites.
  // In production NEXT_PUBLIC_API_URL is unset; confirm page origin serves backend.
  const probeBase = process.env.NEXT_PUBLIC_API_URL
    ? currentApiUrl
    : window.location.origin.replace(/\/$/, '');
  try {
    const ctrl = new AbortController();
    const timer = setTimeout(() => ctrl.abort(), 1500);
    const res = await fetch(`${probeBase}/__qorven_runtime`, { signal: ctrl.signal, cache: 'no-store' });
    clearTimeout(timer);
    if (res.ok && (!isSecurePage || probeBase.startsWith('https:'))) {
      currentApiUrl = probeBase;
    }
  } catch { /* keep currentApiUrl as-is */ }
}

function handleEvent(event: WSEvent) {
  const store = useStore.getState();
  const liveEvent: LiveEvent = {
    id: crypto.randomUUID(),
    type: event.type,
    timestamp: Date.now(),
    data: event.data,
    agent_id: (event.data as Record<string, string>)?.agent_id,
    soul_key: (event.data as Record<string, string>)?.soul_key,
    detail: (event.data as Record<string, string>)?.detail,
  };

  switch (event.type) {
    case 'soul_activity':
      if (liveEvent.agent_id) {
        store.updateSoulActivity(
          liveEvent.agent_id,
          (event.data as Record<string, string>).status as 'idle' | 'thinking' | 'running',
          liveEvent.detail,
        );
      }
      store.pushEvent(liveEvent);
      break;

    case 'approval_required': {
      const d = event.data as Record<string, string>;
      store.incrementPendingApprovals();
      store.pushEvent(liveEvent);
      break;
    }

    case 'new_message': {
      const d = event.data as Record<string, string>;
      const sessionId = (event as any).session_id ?? d.session_id ?? '';
      const agentId = (event as any).agent_id ?? d.agent_id ?? '';
      if (d.content && (sessionId || d.session_id)) {
        store.pushIncomingMessage({
          sessionId: sessionId || d.session_id,
          agentId: agentId || d.agent_id || '',
          role: d.role ?? 'assistant',
          content: d.content,
          source: d.channel ?? d.source,
        });
      }
      store.pushEvent(liveEvent);
      break;
    }

    case 'stream_start': {
      // Room agent starts streaming — push a placeholder into roomIncomingMessages
      // so the chat panel shows the bubble immediately, then stream_delta fills it.
      const d = event.data as Record<string, string>;
      const roomId = d?.room_id;
      const msgId = d?.msg_id ?? crypto.randomUUID();
      if (roomId) {
        store.pushRoomMessage(roomId, {
          id: msgId,
          sender_id: d?.soul_key ?? d?.agent_id ?? 'agent',
          sender_type: 'soul',
          content: '',
          streaming: true,
          created_at: new Date().toISOString(),
        });
      }
      break;
    }

    case 'stream_delta':
    case 'text_delta': {
      // event.data may be a string (bare delta) or { msg_id, content, room_id, ... }
      const raw = event.data;
      const d = (raw && typeof raw === 'object') ? raw as Record<string, string> : null;
      const token = d?.content ?? (typeof raw === 'string' ? raw : '');
      const msgId = d?.msg_id ?? 'current';
      const roomId = d?.room_id;
      if (token) store.appendToken(msgId, token);
      // Mirror token into the room message placeholder so rooms page renders without
      // reading the separate streamingTokens map.
      if (roomId && d?.msg_id && token) {
        store.updateRoomMessage(roomId, d.msg_id, {
          content: (store.streamingTokens[msgId] ?? '') + token,
        });
      }
      break;
    }

    case 'stream_end':
    case 'done': {
      const d = event.data as Record<string, string>;
      const roomId = d?.room_id;
      const msgId = d?.msg_id ?? 'current';
      // Mark the streaming placeholder as complete
      if (roomId && d.msg_id) {
        store.updateRoomMessage(roomId, d.msg_id, { streaming: false });
      }
      store.clearStream(msgId);
      break;
    }

    case 'room_message': {
      // Broadcast from POST /v1/rooms/{id}/messages so every open
      // /rooms/[id] page sees new messages without polling.
      // Normalize field names here so consumers can rely on a stable shape:
      //   id (server field may be message_id or id), sender_id (may arrive as sender),
      //   sender_type, content, created_at.
      const d = event.data as Record<string, unknown>;
      const roomId = (d?.room_id as string) ?? '';
      if (roomId) {
        const normalized = {
          ...d,
          id: (d.id ?? d.message_id ?? crypto.randomUUID()) as string,
          sender_id: (d.sender_id ?? d.sender ?? 'unknown') as string,
          sender_type: (d.sender_type ?? (d.sender_id === 'user' || d.sender === 'user' ? 'user' : 'soul')) as string,
          content: (d.content ?? '') as string,
          created_at: (d.created_at ?? new Date().toISOString()) as string,
        };
        store.pushRoomMessage(roomId, normalized);
      }
      store.pushEvent(liveEvent);
      break;
    }

    case 'room_typing_start':
    case 'room_typing_stop': {
      // Typing indicator — backend emits start/stop with
      // { room_id, agent_id }. We update the per-room typing set
      // and let the UI re-render chips.
      const d = event.data as Record<string, string>;
      const roomId = d?.room_id;
      const agentId = d?.agent_id;
      if (roomId && agentId) {
        store.setRoomTyping(roomId, agentId, event.type === 'room_typing_start');
      }
      break;
    }

    case EVT_PERMISSION_REQUESTED: {
      // Phase 9 Step 2 — inject an approval card into the chat feed
      // that belongs to this request's session_id. The UI layer
      // reads from store.approvals; we normalize+store here.
      const d = event.data as PermissionRequestedProps;
      if (d?.request_id) {
        store.upsertApproval(d);
      }
      // Also surface on the generic live-events log so the Activity
      // tab shows "permission requested" alongside other hub events.
      store.pushEvent(liveEvent);
      break;
    }
    case EVT_PERMISSION_REPLIED: {
      // The server replies can originate from a different client
      // (CLI, admin UI) — treating them as authoritative keeps all
      // observers in sync. A reply for an unknown request_id is a
      // no-op: the store's markApprovalResolved guards against it.
      const d = event.data as PermissionRepliedProps;
      const effectiveDecision = d?.decision === 'allow_always' ? 'allow' : d?.decision;
      if (d?.request_id && (effectiveDecision === 'allow' || effectiveDecision === 'deny')) {
        store.markApprovalResolved(d.request_id, effectiveDecision, {
          actor: d.actor,
          note: d.note,
        });
      }
      store.pushEvent(liveEvent);
      break;
    }

    case 'github.pr_ready': {
      const d = event.data as GitHubPRReadyProps;
      if (d?.pr_url) {
        store.upsertPRApproval(d);
      }
      store.pushEvent(liveEvent);
      break;
    }

    case 'github.commit_pending': {
      const d = event.data as GitHubCommitPendingProps;
      if (d?.branch) {
        store.upsertCommitPending(d);
      }
      store.pushEvent(liveEvent);
      break;
    }

    case 'ticket_updated': {
      store.pushEvent(liveEvent);
      if (typeof window !== 'undefined') {
        window.dispatchEvent(new CustomEvent('qorven:ticket_updated', { detail: event.data }));
      }
      break;
    }

    case 'ticket_comment': {
      store.pushEvent(liveEvent);
      if (typeof window !== 'undefined') {
        window.dispatchEvent(new CustomEvent('qorven:ticket_comment', { detail: event.data }));
      }
      break;
    }

    case 'project_updated': {
      store.pushEvent(liveEvent);
      if (typeof window !== 'undefined') {
        window.dispatchEvent(new CustomEvent('qorven:project_updated', { detail: event.data }));
      }
      break;
    }

    case 'budget_warning': {
      store.pushEvent(liveEvent);
      if (typeof window !== 'undefined') {
        window.dispatchEvent(new CustomEvent('qorven:budget_warning', { detail: event.data }));
      }
      break;
    }

    case 'runtime_state_changed': {
      store.pushEvent(liveEvent);
      if (typeof window !== 'undefined') {
        window.dispatchEvent(new CustomEvent('qorven:runtime_state_changed', { detail: event.data }));
      }
      break;
    }

    case 'task_progress': {
      store.pushEvent(liveEvent);
      if (typeof window !== 'undefined') {
        window.dispatchEvent(new CustomEvent('qorven:task_progress', { detail: event.data }));
      }
      break;
    }

    case 'task_done': {
      store.pushEvent(liveEvent);
      if (typeof window !== 'undefined') {
        window.dispatchEvent(new CustomEvent('qorven:task_done', { detail: event.data }));
      }
      break;
    }

    case 'task_blocked': {
      store.pushEvent(liveEvent);
      if (typeof window !== 'undefined') {
        window.dispatchEvent(new CustomEvent('qorven:task_blocked', { detail: event.data }));
      }
      break;
    }

    case 'service_health': {
      // Broadcast from the backend health monitor when DB state changes.
      // Update store so the banner can show a specific degradation message
      // while the WS itself remains open.
      const d = event.data as { database?: string; status?: string };
      store.setServiceHealth({
        database: (d?.database === 'ok' || d?.database === 'unavailable') ? d.database : 'unknown',
        status: (d?.status === 'healthy' || d?.status === 'degraded') ? d.status : 'unknown',
      });
      store.pushEvent(liveEvent);
      break;
    }

    default: {
      // Phase 9 Step 1 — orchestrator telemetry.
      // `graph.node_*` + `agent.progress` events carry a session_id
      // at the envelope level. Push them into the per-session
      // telemetry slice so in-chat renderers can show a live
      // execution log instead of a generic spinner. We ALSO fall
      // through to pushEvent so the Activity tab continues to see
      // them — this is strictly additive.
      if (isTelemetryEventType(event.type)) {
        const sessionId =
          (event as { session_id?: string }).session_id ??
          (event.data as { session_id?: string } | undefined)?.session_id ??
          '';
        if (sessionId) {
          store.pushTelemetry(sessionId, event as unknown as TelemetryEvent);
        }
      }
      store.pushEvent(liveEvent);
    }
  }
}

// probeDisconnectReason — runs after the WS closes to find out *why*,
// so the banner can show a specific message instead of "Reconnecting…".
// We try /livez first (cheap, never touches DB — if this fails the
// process is gone or the network is broken). If /livez is OK we check
// /readyz to see if the DB is down.
async function probeDisconnectReason(): Promise<void> {
  const store = useStore.getState();
  const base = currentApiUrl;
  try {
    const ctrl = new AbortController();
    const t = setTimeout(() => ctrl.abort(), 3000);
    const livez = await fetch(`${base}/livez`, { signal: ctrl.signal, cache: 'no-store' });
    clearTimeout(t);
    if (!livez.ok) {
      store.setWsDisconnectReason('backend_down');
      return;
    }
    // Process alive — check readiness (DB)
    const ctrl2 = new AbortController();
    const t2 = setTimeout(() => ctrl2.abort(), 3000);
    const readyz = await fetch(`${base}/readyz`, { signal: ctrl2.signal, cache: 'no-store' });
    clearTimeout(t2);
    if (!readyz.ok) {
      store.setWsDisconnectReason('db_down');
      store.setServiceHealth({ database: 'unavailable', status: 'degraded' });
    } else {
      store.setWsDisconnectReason('degraded');
    }
  } catch {
    // fetch threw — either the backend is unreachable or network is gone.
    // Check navigator.onLine to distinguish the two.
    if (typeof navigator !== 'undefined' && !navigator.onLine) {
      store.setWsDisconnectReason('network');
    } else {
      store.setWsDisconnectReason('backend_down');
    }
  }
}

// Lazy install: force-reconnect when the browser flips back online.
// Without this, a laptop waking from sleep sits in its backoff window
// (up to 30s) before trying again — feels broken to the user.
function ensureOnlineListener() {
  if (onlineListenerInstalled || typeof window === 'undefined') return;
  onlineListenerInstalled = true;
  window.addEventListener('online', () => {
    // Reset backoff and try immediately. The attempt counter is still
    // useful as a signal — the banner will briefly show "Reconnecting
    // (1)" then clear on open.
    reconnectAttempt = 0;
    if (reconnectTimer) { clearTimeout(reconnectTimer); reconnectTimer = null; }
    connectWebSocket();
  });
}

export async function connectWebSocket() {
  if (ws?.readyState === WebSocket.OPEN || ws?.readyState === WebSocket.CONNECTING) return;
  ensureOnlineListener();

  // On first connect (or after discovery returns a changed port),
  // refresh currentApiUrl. Discovery is cheap (~1 RTT on localhost)
  // and silent-failing, so we run it on every reconnect. In production
  // deployments with stable ports, it's a no-op.
  await discoverApiUrl();

  const tk = getWsToken();
  const base = wsUrl();
  if (!base) return;
  const url = tk ? `${base}?token=${tk}` : base;
  ws = new WebSocket(url);

  ws.onopen = () => {
    const store = useStore.getState();
    const wasReconnecting = reconnectAttempt > 0;
    reconnectAttempt = 0;
    disconnectedAt = undefined;
    store.setWsConnected(true);
    store.setWsReconnecting(0, undefined);
    store.setWsDisconnectReason(null);
    store.setServiceHealth({ database: 'ok', status: 'healthy' });
    // Trigger catch-up on any reconnect (not just the ones with a
    // timer pending — a quick disconnect may resolve before the timer
    // fires, and we still want to refresh server-side state).
    if (wasReconnecting) store.triggerCatchUp();
  };

  ws.onmessage = (e) => {
    try {
      handleEvent(JSON.parse(e.data));
    } catch { /* ignore malformed */ }
  };

  ws.onclose = () => {
    ws = null;
    const store = useStore.getState();
    store.setWsConnected(false);
    if (disconnectedAt === undefined) disconnectedAt = Date.now();
    Object.keys(store.streamingTokens).forEach((msgId) => store.clearStream(msgId));
    Object.keys(store.soulStates).forEach((agentId) => {
      if (store.soulStates[agentId]?.activity === 'thinking' || store.soulStates[agentId]?.activity === 'running') {
        store.updateSoulActivity(agentId, 'idle');
      }
    });

    reconnectAttempt += 1;
    store.setWsReconnecting(reconnectAttempt, disconnectedAt);
    const delay = nextBackoffMs(reconnectAttempt - 1);
    reconnectTimer = setTimeout(() => { reconnectTimer = null; connectWebSocket(); }, delay);

    // Probe why the connection dropped. Fire-and-forget — the banner
    // will update when the probe resolves (usually <1s on LAN).
    probeDisconnectReason().catch(() => {});
  };

  ws.onerror = () => ws?.close();
}

export function disconnectWebSocket() {
  if (reconnectTimer) clearTimeout(reconnectTimer);
  reconnectAttempt = 0;
  disconnectedAt = undefined;
  const store = useStore.getState();
  store.setWsReconnecting(0, undefined);
  ws?.close();
  ws = null;
}

// Exported for callers that need to know where the backend currently
// lives (e.g. api.ts REST client after a port shift).
export function currentBackendUrl(): string {
  return currentApiUrl;
}
