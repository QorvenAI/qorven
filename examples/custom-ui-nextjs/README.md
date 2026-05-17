# Qorven Custom UI (Next.js) — Reference Example

This directory is the **golden template** for building a third-party Next.js frontend that drives a Qorven gateway. AI coding agents and human developers start here when a user asks for a custom dashboard, embedded widget, or integration UI.

> **Scope:** this is a minimal, copy-ready app. It does NOT ship production concerns (i18n, error-reporting, CSP headers, telemetry). Add those on top of this skeleton for your own deployment — the surface that talks to Qorven is what we lock down here.

## What it demonstrates

1. **Secure token-based auth.** `POST /auth/login` → JWT stored in memory (not `localStorage`) + httpOnly cookie set by the server. No hardcoded tokens anywhere.
2. **WebSocket subscription** to `/ws/realtime` with:
   - Correct token handoff (query param, because browsers cannot set `Authorization` headers on WebSocket upgrades).
   - Ping/pong keep-alive at 20-second intervals.
   - Exponential backoff reconnection (1s → 2s → 4s → … capped at 30s).
   - Graceful cleanup on unmount.
3. **Typed event model.** Every event the backend emits (`agent.progress`, `graph.node_started`, `graph.node_completed`, `graph.node_paused`, `graph.node_failed`) has a matching TypeScript discriminant so `switch(event.type)` is type-safe.
4. **Live graph visualization.** A single page that fetches a plan + its nodes, then mutates the local state tree *only* from WebSocket events. No polling, no race conditions.

## Architecture map

```
examples/custom-ui-nextjs/
├── lib/
│   ├── api.ts          ← typed fetch wrapper + authFetch()
│   ├── auth.ts         ← in-memory token store + login/logout helpers
│   └── events.ts       ← event type constants + TS discriminated union
├── hooks/
│   ├── useQorvenSocket.ts   ← WebSocket lifecycle (backoff, ping/pong)
│   └── useGraphState.ts     ← derives plan-graph state from event stream
├── components/
│   └── GraphVisualizer.tsx  ← renders node boxes by state
└── app/
    ├── login/page.tsx       ← login form
    ├── plans/[id]/page.tsx  ← live plan-execution view
    └── page.tsx             ← home: list of recent plans
```

## Getting started

```bash
cd examples/custom-ui-nextjs
cp .env.example .env.local
# point NEXT_PUBLIC_QORVEN_API_BASE at your gateway, e.g. http://localhost:8080
pnpm install
pnpm dev
```

Open <http://localhost:3000>, log in with the same credentials as the Qorven gateway, and navigate to a plan id to see the real-time graph.

## Key contracts (wire-level truth)

All documented in the root repo's `AGENTS.md` and verified against the current backend source. If you add a new event type in the backend, update `lib/events.ts` here too — the TypeScript union is the only place agents should consult for the canonical set.

### Auth (`lib/auth.ts`)

- `POST /auth/login` body `{ username, password }` returns `{ token, user }` and sets a `qorven_session` httpOnly cookie.
- `GET /auth/me` with `Authorization: Bearer <token>` returns the user.
- `POST /auth/logout` clears the cookie.
- Token storage: in-memory (`authStore`). We intentionally do NOT stash the JWT in `localStorage` — that exposes it to every third-party script on the page. The cookie handles persistence server-to-server; the in-memory copy handles per-session API calls.

### Realtime (`hooks/useQorvenSocket.ts`)

- `GET /ws/realtime?token=<jwt>` upgrades to WebSocket. The gateway's `wsAuth` middleware accepts the token via query param (browsers can't set headers on WS upgrades).
- Server sends JSON envelopes: `{ type, session_id?, agent_id?, data?, timestamp, seq }`.
- The hook sends a ping frame every 20s (WebSocket control frame, not a JSON message). If three ping intervals go by with no server traffic, the socket is declared dead and the backoff loop kicks in.
- Reconnect backoff: `min(30s, 1s × 2^attempts)`. Reset on every successful `onopen`.

### Event types (`lib/events.ts`)

Mirror of `backend/internal/api/events/types.go:55-110`. The union type covers the five lifecycle events a graph visualizer actually needs:

| Event type | Fires when |
| --- | --- |
| `agent.progress` | Legacy all-purpose agent heartbeat. Keep as fallback. |
| `graph.node_started` | A plan node enters `running`. |
| `graph.node_completed` | A plan node completes successfully. |
| `graph.node_paused` | A node blocks pending human approval. |
| `graph.node_failed` | A node errors terminally. |

Other event types exist (tool_start, session_idle, etc.) — this example ignores them to keep the hook focused. Subscribe to more as your UI needs them.

## What this example deliberately does NOT do

- **No SSR.** All data fetches run client-side so the auth token can live in memory. SSR would require a different token-handoff strategy (server-side cookie read + proxy).
- **No state library.** Zustand / Redux / Jotai are fine additions; the hooks here use `useState` + `useReducer` so the patterns are legible without a framework overlay.
- **No styling framework.** Plain CSS modules. Drop Tailwind / Radix / shadcn on top if you want — nothing here blocks it.
- **No tests.** The backend has the contract tests. A user forking this template owns the UI tests for their app.

## When you're ready to ship

Read the root `AGENTS.md` section §6 for the full frontend extensibility rules. In particular:

- You are in the **multi-tenant** world. Every request carries the JWT; the gateway scopes the Postgres transaction to your tenant via `TenantScopeMiddleware`. If your component ever needs data it's not seeing, the issue is almost always either (a) you forgot the `Authorization` header, or (b) you're querying a table that isn't RLS-wired. Start from those two before suspecting the backend.
- Never ship a token to a logging / analytics service. The in-memory store makes that harder to do by accident; keep it that way.
