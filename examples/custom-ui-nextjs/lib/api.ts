/**
 * Typed fetch wrapper that attaches the auth token + handles the
 * common error shapes Qorven returns.
 *
 * ## The Qorven /v1/* error envelope
 *
 * On 4xx/5xx the gateway returns `{ error: string, code?: string }`.
 * `authFetch` wraps that into a typed exception so callers can
 * `try { ... } catch (e: ApiError) { ... }` instead of unwrapping
 * response JSON everywhere.
 *
 * ## Multi-tenant note
 *
 * Every /v1 request the user makes is wrapped server-side in a
 * Postgres transaction scoped to their tenant_id. This wrapper has
 * no say in that — the gateway's TenantScopeMiddleware handles
 * everything. Just make sure your fetch carries the Authorization
 * header OR the session cookie (either works) and you'll be scoped
 * correctly.
 */

import { getAuth } from "./auth";

export class ApiError extends Error {
  status: number;
  code?: string;
  constructor(status: number, message: string, code?: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
  }
}

/**
 * Returns the API base URL from env. Falls back to a placeholder
 * during server-side rendering / build — we can't throw here because
 * Next 15 evaluates JSX at build time for route-metadata collection,
 * and crashing poisons the build. At runtime the first actual fetch
 * against the placeholder will fail with a clear "refused / DNS"
 * error, which is the signal a developer needs.
 *
 * If you want stricter behavior in your own app, switch this to throw
 * and avoid referencing apiBase() inside JSX.
 */
const PLACEHOLDER_API_BASE = "http://localhost:8080";

export function apiBase(): string {
  const v = process.env.NEXT_PUBLIC_QORVEN_API_BASE;
  if (!v) {
    if (typeof window !== "undefined") {
      // Runtime (browser) with no env var — louder warning so a
      // misconfigured deploy is obvious.
      console.warn(
        "[qorven] NEXT_PUBLIC_QORVEN_API_BASE is not set; " +
          "falling back to http://localhost:8080. " +
          "Copy .env.example to .env.local and fill it in.",
      );
    }
    return PLACEHOLDER_API_BASE;
  }
  return v.replace(/\/$/, "");
}

/**
 * Derives the WebSocket base from the HTTP base, or uses the
 * explicit NEXT_PUBLIC_QORVEN_WS_BASE if provided.
 */
export function wsBase(): string {
  const explicit = process.env.NEXT_PUBLIC_QORVEN_WS_BASE;
  if (explicit) return explicit.replace(/\/$/, "");
  const http = apiBase();
  return http.replace(/^http/, "ws");
}

/**
 * authFetch is a small fetch wrapper that:
 *   - attaches `Authorization: Bearer <token>` from the auth store,
 *   - sends cookies via `credentials: "include"` for the session cookie,
 *   - parses JSON responses,
 *   - normalizes non-2xx responses into ApiError.
 *
 * Callers pass relative paths (`/v1/plans/123`) and the apiBase()
 * prefix is attached here.
 */
export async function authFetch<T = unknown>(
  path: string,
  init: RequestInit = {},
): Promise<T> {
  const { token } = getAuth();
  const headers = new Headers(init.headers);
  if (token) headers.set("Authorization", `Bearer ${token}`);
  // Only set Content-Type on requests with a body; avoids forcing a
  // CORS preflight on simple GETs.
  if (init.body && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }

  const res = await fetch(`${apiBase()}${path}`, {
    ...init,
    credentials: "include",
    headers,
  });

  // No-content success (204).
  if (res.status === 204) return undefined as T;

  const text = await res.text();
  const body = text ? safeJSON(text) : undefined;

  if (!res.ok) {
    const msg =
      body && typeof body === "object" && "error" in body
        ? String((body as { error: unknown }).error)
        : `HTTP ${res.status} ${res.statusText}`;
    const code =
      body && typeof body === "object" && "code" in body
        ? String((body as { code: unknown }).code)
        : undefined;
    throw new ApiError(res.status, msg, code);
  }
  return body as T;
}

function safeJSON(s: string): unknown {
  try {
    return JSON.parse(s);
  } catch {
    return s;
  }
}

// ─────────────────── Domain types ───────────────────

export type Plan = {
  id: string;
  tenant_id: string;
  project_id: string;
  session_id: string;
  title: string;
  status:
    | "draft"
    | "pending_approval"
    | "approved"
    | "running"
    | "done"
    | "failed"
    | "cancelled"
    | "rejected";
  summary: string;
  created_by: string;
  created_at: string;
  updated_at: string;
};

export type PlanNode = {
  id: string;
  plan_id: string;
  kind: "planner" | "human_feedback" | "agent_task" | string;
  title: string;
  state: "pending" | "running" | "blocked" | "done" | "failed" | "cancelled";
  assignee_soul?: string;
  created_at: string;
  updated_at: string;
};

// ─────────────────── Typed endpoint helpers ───────────────────

export function getPlan(id: string): Promise<Plan> {
  return authFetch<Plan>(`/v1/plans/${encodeURIComponent(id)}`);
}

export function listPlanNodes(planID: string): Promise<PlanNode[]> {
  return authFetch<PlanNode[]>(
    `/v1/plans/${encodeURIComponent(planID)}/nodes`,
  );
}
