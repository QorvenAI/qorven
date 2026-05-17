# AGENTS.md — `examples/custom-ui-nextjs`

> **Audience:** an AI coding agent extending THIS Next.js reference app — not the Qorven core. For platform-wide rules see `../../AGENTS.md`.

## Orientation (read in this order)

1. `README.md` — why this example exists, what it demonstrates, what it deliberately skips.
2. `lib/auth.ts` — how the JWT flow actually works. In-memory token + httpOnly cookie. **Do not** add localStorage persistence.
3. `lib/events.ts` — the canonical set of event types. If you need a new event, add it here AND verify the backend emits it (`backend/internal/api/events/types.go`).
4. `hooks/useQorvenSocket.ts` — the WebSocket lifecycle. Touch with care; the ping/pong + backoff logic is the only thing that keeps us resilient on flaky networks.

## Non-negotiable rules

1. **Never store the JWT in `localStorage` or `sessionStorage`.** The in-memory pattern is load-bearing; a swap to storage is a security regression.
2. **Always use `authFetch` from `lib/api.ts`** for `/v1/*` calls. Direct `fetch()` skips the auth header + cookie + error normalization.
3. **The WebSocket token goes in the URL query string, not a header.** Browsers can't set `Authorization` on a WebSocket upgrade — the backend (`gateway/middleware.go:wsAuth`) reads `?token=`.
4. **`useGraphState` only MUTATES nodes, never CREATES them.** The server's REST response is the source of truth for the node set; realtime events only flip states. If you need new-node events, add a dedicated "node.created" type to the backend first — don't race the REST refresh.

## Adding a new event type

1. Add the constant + data-shape to `lib/events.ts` and extend the `QorvenEvent` union.
2. If it should flip node state, add a case in `hooks/useGraphState.ts`'s switch.
3. If it's session-scoped and filtering matters, consult `README.md` — today we filter by `data.plan_id` client-side.
4. Verify the backend emits it by grepping `backend/internal/api/events/types.go` for the constant. Never add a frontend type for an event the server does not emit.

## Adding a new page

1. Create `app/<path>/page.tsx`. Mark `"use client"` if you need hooks.
2. Call `bootstrap(apiBase())` in an effect; redirect to `/login` on null user. Keep auth-gated pages consistent.
3. Use `useQorvenSocket()` directly if the page wants realtime — the hook tolerates multiple mounts; each gets its own socket.

## Things NOT to do

- Don't reach into other tenants' data by URL-hacking. RLS will return empty/404; the UI shouldn't try anyway.
- Don't pre-fetch /v1 routes during SSR in this template — the auth token is client-side-only. A production app with SSR needs a different handoff (reverse-proxy with cookie pass-through, or server-side token store).
- Don't expose the `NEXT_PUBLIC_QORVEN_API_BASE` to a production build pointing at localhost. The `.env.example` is for local dev; production sets `NEXT_PUBLIC_QORVEN_API_BASE=https://your-gateway.tld`.
