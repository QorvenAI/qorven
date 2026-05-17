// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

// Central API URL helpers — the only place in the codebase that decides
// how client-side code addresses the backend.
//
// RULES:
//   - In the browser: always use root-relative paths (/api/v1/...) so
//     the bundle is completely origin-agnostic. The Go gateway strips the
//     /api prefix via apiPrefixAlias; in dev the Next.js server rewrites
//     them to the backend. Either way the correct host/port/scheme comes
//     from the browser, not from a build-time env var.
//
//   - WebSocket URLs are the one exception: browsers require an absolute
//     ws:// or wss:// URL. We derive them from window.location so the
//     scheme (ws vs wss) matches the page — never hardcoded.
//
//   - On the server (SSR / Node) we fall back to NEXT_PUBLIC_API_URL
//     because there is no window.location to infer from.
//
// Do NOT call these helpers during module initialisation outside a
// function body — window is undefined during SSR.

/** REST base path for browser requests: /api/v1 */
export const API_V1 = '/api/v1';

/** Auth base path for browser requests: /api/auth */
export const API_AUTH = '/api/auth';

/**
 * Returns the base URL for REST calls.
 *   - Browser: root-relative '/api/v1' — works from any host/port/scheme.
 *   - Server:  full URL from NEXT_PUBLIC_API_URL (needed for SSR fetches).
 */
export function apiBase(): string {
  if (typeof window === 'undefined') {
    return (process.env.NEXT_PUBLIC_API_URL ?? 'http://localhost:4200') + '/v1';
  }
  return API_V1;
}

/**
 * Returns the auth base URL.
 *   - Browser: '/api/auth'
 *   - Server:  full URL from NEXT_PUBLIC_API_URL.
 */
export function authBase(): string {
  if (typeof window === 'undefined') {
    return (process.env.NEXT_PUBLIC_API_URL ?? 'http://localhost:4200') + '/auth';
  }
  return API_AUTH;
}

/**
 * Converts a REST path to an absolute WebSocket URL, matching the page
 * scheme (http→ws, https→wss).
 *
 * Usage:
 *   wsBase('/ws/realtime')        → 'wss://example.com/ws/realtime'
 *   wsBase('/v1/terminal/ws')     → 'wss://example.com/v1/terminal/ws'
 *
 * The path is served directly by the Go gateway (no /api prefix needed
 * for WebSocket routes — they live under /ws/... or /v1/...  directly).
 * In dev, Next.js proxies /ws/... and /v1/... paths to the backend.
 */
export function wsBase(path: string): string {
  if (typeof window === 'undefined') return '';
  const { protocol, host } = window.location;
  const wsProtocol = protocol === 'https:' ? 'wss:' : 'ws:';
  return `${wsProtocol}//${host}${path}`;
}
