// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

// Client-side: use /api/v1 proxy path — in dev, Next.js rewrites it to
// the backend; in the static export, the Go gateway's apiPrefixAlias
// middleware strips the /api prefix, so both cases work from same origin.
// Server-side: call backend directly via NEXT_PUBLIC_API_URL.
export const BASE = typeof window !== 'undefined' ? '/api/v1' : (process.env.NEXT_PUBLIC_API_URL + '/v1');
export const STREAM_BASE = typeof window !== 'undefined'
  ? '/api/v1'
  : (process.env.NEXT_PUBLIC_API_URL + '/v1');

export const AUTH_BASE = typeof window !== 'undefined' ? '/api/auth' : (process.env.NEXT_PUBLIC_API_URL + '/auth');

// --- Error helpers ---

const ERROR_MAP: Record<string, string> = {
  'invalid credentials':       'Wrong username or password.',
  'user not found':            'Wrong username or password.',
  'account locked':            'Account locked due to too many failed attempts. Try again later.',
  'user is not active':        'This account has been deactivated.',
  'password too short':        'Password must be at least 8 characters.',
  'username already exists':   'That username is already taken.',
  'setup already completed':   'Admin account already exists.',
};

export function extractErrorMessage(raw: string): string {
  try {
    const parsed = JSON.parse(raw);
    const msg: string = parsed?.error ?? parsed?.message ?? raw;
    const lower = msg.toLowerCase();
    for (const [key, friendly] of Object.entries(ERROR_MAP)) {
      if (lower.includes(key)) return friendly;
    }
    return msg;
  } catch {
    return raw || 'Something went wrong. Please try again.';
  }
}

// --- Auth helpers ---
export function getToken(): string {
  if (typeof window === 'undefined') return process.env.NEXT_PUBLIC_API_TOKEN ?? process.env.API_TOKEN ?? '';
  return localStorage.getItem('qorven_token') ?? process.env.NEXT_PUBLIC_API_TOKEN ?? '';
}

export function setToken(token: string) {
  localStorage.setItem('qorven_token', token);
  document.cookie = `qorven_token=${token}; path=/; max-age=${7 * 24 * 3600}; SameSite=Lax`;
}

export function clearToken() {
  localStorage.removeItem('qorven_token');
  document.cookie = 'qorven_token=; path=/; max-age=0; SameSite=Lax';
}

export function isAuthenticated(): boolean {
  if (typeof window === 'undefined') return false;
  return !!localStorage.getItem('qorven_token');
}

import { isIdempotentMethod, isNetworkError, retryDelayMs } from './resilience';

export async function fetchWithRetry(url: string, init: RequestInit): Promise<Response> {
  const method = (init.method ?? 'GET').toUpperCase();
  const canRetry = isIdempotentMethod(method);
  const maxAttempts = canRetry ? 3 : 1;

  let lastErr: unknown;
  for (let attempt = 0; attempt < maxAttempts; attempt++) {
    try {
      return await fetch(url, init);
    } catch (err) {
      lastErr = err;
      if (!canRetry) throw err;
      if (!isNetworkError(err)) throw err;
      if (attempt < maxAttempts - 1) {
        await new Promise((r) => setTimeout(r, retryDelayMs(attempt)));
        continue;
      }
    }
  }
  throw lastErr instanceof Error ? lastErr : new Error('Network error');
}

export async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetchWithRetry(`${BASE}${path}`, {
    ...init,
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${getToken()}`,
      ...init?.headers,
    },
  });
  if (res.status === 401 && typeof window !== 'undefined') {
    if (localStorage.getItem('qorven_token')) {
      clearToken();
      const next = encodeURIComponent(window.location.pathname + window.location.search);
      window.location.href = `/login?next=${next}`;
    }
    throw new Error('Unauthorized');
  }
  const text = await res.text();
  if (res.status === 403 && typeof window !== 'undefined') {
    let body: unknown;
    try { body = JSON.parse(text); } catch { body = null; }
    if (body && typeof body === 'object' && (body as Record<string, unknown>).setup_required === true) {
      window.location.href = '/setup';
      throw new Error('Setup required');
    }
  }
  if (!res.ok) throw new Error(extractErrorMessage(text));
  if (!text || res.status === 204) return null as T;
  if (!text.trimStart().startsWith('{') && !text.trimStart().startsWith('[')) {
    throw new Error(`API returned non-JSON (${res.status}): ${text.slice(0, 120)}`);
  }
  return JSON.parse(text);
}

export function listRequest<T>(path: string): Promise<T[]> {
  return request<any>(path).then((d) => {
    if (Array.isArray(d)) return d;
    const values = Object.values(d);
    for (const v of values) {
      if (Array.isArray(v)) return v;
    }
    return [];
  });
}

export const auth = {
  status: () =>
    fetch(`${AUTH_BASE}/setup-check`).then((r) => r.json()) as Promise<{ setup_required: boolean }>,
  setup: (body: { username: string; password: string; email?: string }) =>
    fetch(`${AUTH_BASE}/setup`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    }).then(async (r) => {
      if (!r.ok) throw new Error(extractErrorMessage(await r.text()));
      return r.json() as Promise<{ user: { id: string; username: string; role: string }; message: string }>;
    }),
  login: (body: { username: string; password: string }) =>
    fetch(`${AUTH_BASE}/login`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    }).then(async (r) => {
      if (!r.ok) throw new Error(extractErrorMessage(await r.text()));
      return r.json() as Promise<{ token: string; user: { id: string; username: string; role: string } }>;
    }),
  logout: () => {
    clearToken();
    window.location.href = '/login';
  },
};
