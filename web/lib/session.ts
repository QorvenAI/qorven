// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

/**
 * Canonical session ID minting.
 *
 * Phase 9 Step 2 fix for the session_id provenance gap:
 * the server mints a fresh UUID for every session via
 * POST /v1/sessions and emits telemetry keyed on that UUID.
 * Clients that fabricate their own "code-1729..." style ids
 * end up with sessions.session_key != sessions.id, so every
 * server-emitted telemetry event drops on the client side.
 *
 * ensureCanonicalSessionId() guarantees the caller holds a
 * real UUID before any chat round-trip. Callers pass a mutable
 * ref whose current value may be synthetic; the helper:
 *
 *   1. Returns immediately if ref.current already looks like a UUID.
 *   2. Otherwise posts /v1/sessions, mints a UUID, writes it
 *      back into ref.current, and returns it.
 *
 * An in-flight promise guards against concurrent callers that
 * race the first mint — a single session, not N.
 */

import { sessions } from '@/lib/api';
import type { Session } from '@/types';

// UUID v4/v7 shape — matches Postgres's native uuid type and both
// gen_random_uuid() (v4) + uuid_generate_v7() outputs.
const UUID_RE =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

export function isCanonicalSessionId(id: string): boolean {
  return UUID_RE.test(id);
}

// One in-flight mint per ref. Keyed by the ref object identity so
// two unrelated refs don't collide but repeated calls for the same
// ref deduplicate to a single POST.
const inflightMints = new WeakMap<
  { current: string },
  Promise<string>
>();

export interface EnsureSessionIdOptions {
  /** agent_id required by POST /v1/sessions. Code page passes 'prime'. */
  agentId: string;
  /** channel column value — 'code' for the editor, 'web' for right panel. */
  channel?: string;
  /**
   * Optional label. Currently unused by the minting endpoint but
   * reserved for a future display-name column surfaced in UIs.
   */
  label?: string;
}

/**
 * ensureCanonicalSessionId is the single entry point. Pass a ref
 * whose current value is either empty, synthetic (`code-...`), or
 * a real UUID. Returns the canonical UUID.
 *
 * The function mutates ref.current in place so subsequent
 * synchronous reads (e.g. inside a retry) see the minted id.
 */
export async function ensureCanonicalSessionId(
  ref: { current: string },
  opts: EnsureSessionIdOptions,
): Promise<string> {
  if (isCanonicalSessionId(ref.current)) {
    return ref.current;
  }
  const existing = inflightMints.get(ref);
  if (existing) return existing;

  const p = (async () => {
    try {
      const session: Session = await sessions.create({
        agent_id: opts.agentId,
        channel: opts.channel ?? 'web',
      });
      if (!session?.id || !isCanonicalSessionId(session.id)) {
        // Server returned a non-UUID id — this shouldn't happen
        // (handlers.go:902 mints via uuid.New) but we guard so a
        // server-side regression surfaces as a clear error rather
        // than a silent telemetry drop.
        throw new Error(`sessions.create returned non-UUID id: ${session?.id}`);
      }
      ref.current = session.id;
      return session.id;
    } finally {
      inflightMints.delete(ref);
    }
  })();
  inflightMints.set(ref, p);
  return p;
}
