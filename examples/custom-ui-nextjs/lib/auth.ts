/**
 * Auth store + helpers for Qorven's JWT flow.
 *
 * ## Contract (verified against backend/internal/gateway/auth_handlers.go)
 *
 * - `POST /auth/login` body `{ username, password }` → 200 with
 *   `{ token, user }` AND sets the `qorven_session` httpOnly cookie.
 *   The cookie gives us server-side auth for classic form submits;
 *   the token gives us per-request auth for `fetch()` / WebSocket.
 * - `GET /auth/me` with `Authorization: Bearer <token>` → the user.
 * - `POST /auth/logout` clears the cookie.
 *
 * ## Why the token lives in memory, not localStorage
 *
 * Putting a JWT in localStorage exposes it to every third-party script
 * (analytics, ads, sentry, any lib with a compromised supply chain).
 * The httpOnly cookie is the durable carrier; the in-memory copy is
 * just so we can attach `Authorization: Bearer <jwt>` to `fetch()`
 * and WebSocket URLs without making the server re-issue on every
 * request. Reload = we have to re-login. That's the correct trade.
 */

export type User = {
  id: string;
  username: string;
  email?: string;
  role: string;
  tenant_id: string;
};

type AuthState = {
  token: string | null;
  user: User | null;
};

// The single in-process copy. Subscribing components re-render via the
// `listeners` set below; we don't pull in zustand for a 40-line store.
let state: AuthState = { token: null, user: null };
const listeners = new Set<() => void>();

function notify() {
  for (const l of listeners) l();
}

/** Subscribe to auth changes. Returns an unsubscribe function. */
export function subscribeAuth(fn: () => void): () => void {
  listeners.add(fn);
  return () => listeners.delete(fn);
}

/** Current auth snapshot. Synchronous — React hooks use this via useSyncExternalStore. */
export function getAuth(): AuthState {
  return state;
}

function setAuth(next: AuthState) {
  state = next;
  notify();
}

/**
 * Log in. On success the token + user are stored in memory and the
 * httpOnly cookie is set by the server (automatic, via Set-Cookie on
 * the response).
 *
 * Rejects with an Error whose message is the server's error string
 * ("invalid credentials", "rate limited", etc.).
 */
export async function login(
  apiBase: string,
  username: string,
  password: string,
): Promise<User> {
  const res = await fetch(`${apiBase}/auth/login`, {
    method: "POST",
    credentials: "include", // accept the Set-Cookie response
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ username, password }),
  });
  const body: unknown = await res.json().catch(() => ({}));
  if (!res.ok) {
    const msg =
      typeof body === "object" && body && "error" in body
        ? String((body as { error: unknown }).error)
        : `login failed: HTTP ${res.status}`;
    throw new Error(msg);
  }
  const typed = body as { token: string; user: User };
  setAuth({ token: typed.token, user: typed.user });
  return typed.user;
}

/** Log out. Clears the server cookie and the in-memory token. */
export async function logout(apiBase: string): Promise<void> {
  await fetch(`${apiBase}/auth/logout`, {
    method: "POST",
    credentials: "include",
  });
  setAuth({ token: null, user: null });
}

/**
 * Probe the server for the current session. Useful on mount: if the
 * httpOnly cookie is still valid, we can reconstruct the in-memory
 * token by calling /auth/me + /auth/refresh. If not authenticated,
 * returns null and the UI can redirect to /login.
 *
 * This costs one round-trip per page load; acceptable for a reference
 * UI. A production app might cache authenticated-state briefly.
 */
export async function bootstrap(apiBase: string): Promise<User | null> {
  // /auth/me accepts the cookie (no Authorization header needed) because
  // AuthMiddlewareV2 checks cookies before bearer tokens.
  const meRes = await fetch(`${apiBase}/auth/me`, {
    credentials: "include",
  });
  if (!meRes.ok) return null;
  const user = (await meRes.json()) as User;

  // Ask the server to mint a fresh JWT we can use for Authorization
  // headers and WebSocket query params. /auth/refresh reads the cookie
  // and re-issues.
  const refreshRes = await fetch(`${apiBase}/auth/refresh`, {
    method: "POST",
    credentials: "include",
  });
  if (!refreshRes.ok) return null;
  const { token } = (await refreshRes.json()) as { token: string };

  setAuth({ token, user });
  return user;
}
