// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

/**
 * Qorven canonical command client.
 *
 * Mirrors backend/internal/api/commands/{types,handler}.go. Every UI
 * action (send a prompt, open the model picker, abort a run, resize the
 * TUI) is one POST to this API. The web /code page and the Go TUI both
 * hit these endpoints — zero drift.
 *
 * See QORVEN-APP-BUILDER-DEEP-PLAN.md §9.3 for the full capability matrix.
 */

// ─────────────────────────── Request shapes ─────────────────────────────

export interface AppendPromptRequest {
  session_id: string;
  text: string;
}
export interface ClearPromptRequest {
  session_id: string;
}
export interface SubmitPromptRequest {
  session_id: string;
  agent_id?: string;
  text?: string;
  metadata?: Record<string, string>;
}
export interface ExecuteCommandRequest {
  command: string;
  args?: Record<string, unknown>;
}
export interface OpenSessionsRequest {
  agent_id?: string;
  filter?: string;
}
export interface OpenModelsRequest {
  scope?: string;
}
export interface OpenThemesRequest {
  /* no fields yet */
}
export interface ShowToastRequest {
  level: 'info' | 'success' | 'warn' | 'error';
  message: string;
  duration_ms?: number;
  actor?: string;
}
export interface ResizeRequest {
  session_id?: string;
  cols: number;
  rows: number;
}

// ─────────────────────────── Response shape ─────────────────────────────

export interface CommandResponse<T = Record<string, unknown>> {
  ok: boolean;
  error?: string;
  code?: string;
  data?: T;
}

export class CommandError extends Error {
  code?: string;
  status: number;
  constructor(message: string, opts: { code?: string; status: number }) {
    super(message);
    this.name = 'CommandError';
    this.code = opts.code;
    this.status = opts.status;
  }
}

// ─────────────────────────── Transport ─────────────────────────────────

const DEFAULT_BASE =
  typeof window !== 'undefined'
    ? '/api/v1/commands'
    : (process.env.NEXT_PUBLIC_API_URL || '') + '/v1/commands';

function authHeaders(): Record<string, string> {
  const h: Record<string, string> = { 'Content-Type': 'application/json' };
  if (typeof window !== 'undefined') {
    const token = localStorage.getItem('qorven_token');
    if (token) h.Authorization = `Bearer ${token}`;
  } else {
    const token = process.env.NEXT_PUBLIC_API_TOKEN;
    if (token) h.Authorization = `Bearer ${token}`;
  }
  return h;
}

interface CallOpts {
  base?: string;
  signal?: AbortSignal;
  retries?: number;
  retryDelayMS?: number;
}

async function call<TResp, TReq extends object>(
  path: string,
  body: TReq,
  opts?: CallOpts,
): Promise<CommandResponse<TResp>> {
  const base = opts?.base ?? DEFAULT_BASE;
  const url = `${base}${path}`;
  const maxAttempts = Math.max(1, (opts?.retries ?? 2) + 1);
  const backoff = opts?.retryDelayMS ?? 300;

  let lastErr: unknown = null;
  for (let attempt = 1; attempt <= maxAttempts; attempt++) {
    try {
      const resp = await fetch(url, {
        method: 'POST',
        headers: authHeaders(),
        body: JSON.stringify(body),
        signal: opts?.signal,
        credentials: 'include',
      });

      let parsed: CommandResponse<TResp> = { ok: false };
      try {
        parsed = (await resp.json()) as CommandResponse<TResp>;
      } catch {
        // Non-JSON error response — synthesize from status.
        parsed = { ok: resp.ok, error: resp.ok ? undefined : resp.statusText };
      }

      if (!resp.ok || !parsed.ok) {
        // 4xx: terminal; do not retry.
        if (resp.status >= 400 && resp.status < 500) {
          throw new CommandError(parsed.error ?? resp.statusText, {
            status: resp.status,
            code: parsed.code,
          });
        }
        // 5xx: retry with backoff.
        lastErr = new CommandError(parsed.error ?? resp.statusText, {
          status: resp.status,
          code: parsed.code,
        });
        if (attempt < maxAttempts) {
          await sleep(backoff * attempt);
          continue;
        }
        throw lastErr;
      }
      return parsed;
    } catch (err) {
      // Abort: surface immediately.
      if ((err as { name?: string })?.name === 'AbortError') throw err;
      // CommandError with 4xx already thrown above; rethrow here.
      if (err instanceof CommandError && err.status < 500) throw err;
      lastErr = err;
      if (attempt < maxAttempts) {
        await sleep(backoff * attempt);
        continue;
      }
      throw err;
    }
  }
  // Unreachable — loop always returns or throws.
  throw lastErr ?? new Error('command failed');
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

// ────────────────────────────── API ────────────────────────────────────

export const commands = {
  appendPrompt(req: AppendPromptRequest, opts?: CallOpts) {
    return call<{ session_id: string; draft_runes: number }, AppendPromptRequest>(
      '/append_prompt',
      req,
      opts,
    );
  },
  clearPrompt(req: ClearPromptRequest, opts?: CallOpts) {
    return call<{ session_id: string }, ClearPromptRequest>('/clear_prompt', req, opts);
  },
  submitPrompt(req: SubmitPromptRequest, opts?: CallOpts) {
    return call<{ session_id: string; message_id: string }, SubmitPromptRequest>(
      '/submit_prompt',
      req,
      opts,
    );
  },
  execute(req: ExecuteCommandRequest, opts?: CallOpts) {
    return call<Record<string, unknown>, ExecuteCommandRequest>('/execute', req, opts);
  },
  openSessions(req: OpenSessionsRequest = {}, opts?: CallOpts) {
    return call<{ picker: 'sessions' }, OpenSessionsRequest>('/open_sessions', req, opts);
  },
  openModels(req: OpenModelsRequest = {}, opts?: CallOpts) {
    return call<{ picker: 'models' }, OpenModelsRequest>('/open_models', req, opts);
  },
  openThemes(req: OpenThemesRequest = {}, opts?: CallOpts) {
    return call<{ picker: 'themes' }, OpenThemesRequest>('/open_themes', req, opts);
  },
  showToast(req: ShowToastRequest, opts?: CallOpts) {
    return call<Record<string, unknown>, ShowToastRequest>('/toast', req, opts);
  },
  resize(req: ResizeRequest, opts?: CallOpts) {
    return call<{ cols: number; rows: number }, ResizeRequest>('/resize', req, opts);
  },
};

export type CommandsAPI = typeof commands;
