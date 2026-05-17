// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

// resilience.ts — pure, testable helpers used by the WebSocket reconnect
// loop and the REST retry wrapper. Kept separate so a Node unit test
// can import and execute them without pulling in Next.js, Zustand, or
// browser globals.

// nextBackoffMs — exponential backoff with full jitter.
//
// Formula: base = min(1000 * 2^attempt, capMs); delay = base + random(0..jitterMs).
// Matches the "Full Jitter" variant from the AWS architecture blog
// (https://aws.amazon.com/builders-library/timeouts-retries-and-backoff-with-jitter/).
// Jitter is critical for avoiding thundering-herd reconnect storms
// after a server restart — every tab backs off for a slightly different
// amount of time.
export function nextBackoffMs(
  attempt: number,
  opts: { capMs?: number; jitterMs?: number; randomFn?: () => number } = {},
): number {
  const capMs = opts.capMs ?? 30_000;
  const jitterMs = opts.jitterMs ?? 1000;
  const rand = opts.randomFn ?? Math.random;
  const safeAttempt = Math.max(0, attempt);
  const base = Math.min(1000 * Math.pow(2, safeAttempt), capMs);
  return base + rand() * jitterMs;
}

// isIdempotentMethod — which HTTP methods are safe to retry without
// the caller's knowledge. GET/HEAD replay has no side effects; POST/
// PATCH/DELETE can double-create or double-apply. Keep this narrow —
// callers who KNOW their POST is idempotent (e.g. with an Idempotency-Key
// header) can wrap fetchWithRetry explicitly.
export function isIdempotentMethod(method: string): boolean {
  const m = method.toUpperCase();
  return m === 'GET' || m === 'HEAD';
}

// retryDelayMs — companion to fetchWithRetry. Separate function so
// tests can assert delay values without actually sleeping. The REST
// backoff uses a tighter window than the WS one: REST calls usually
// block UI, so a 30s wait would feel dead.
export function retryDelayMs(
  attempt: number,
  opts: { baseMs?: number; jitterMs?: number; randomFn?: () => number } = {},
): number {
  const baseMs = opts.baseMs ?? 300;
  const jitterMs = opts.jitterMs ?? 200;
  const rand = opts.randomFn ?? Math.random;
  return baseMs * Math.pow(2, attempt) + rand() * jitterMs;
}

// isNetworkError — classifies a thrown value from fetch(). We retry on
// connection-level failures (TypeError from fetch ≈ ECONNREFUSED or
// DNS fail) but NOT on AbortError (caller cancelled, must propagate).
export function isNetworkError(err: unknown): boolean {
  if (err instanceof Error) {
    if (err.name === 'AbortError') return false;
    // fetch spec says network failures throw TypeError; some runtimes
    // wrap them, so also check the message.
    if (err instanceof TypeError) return true;
    if (/network|fetch failed|ECONNREFUSED|ENOTFOUND/i.test(err.message)) return true;
  }
  return false;
}
