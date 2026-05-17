// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.
//
// Unit tests for web/lib/resilience.ts. Runs with Node's built-in test
// runner (node 24 has native TypeScript execution), no vitest/jest
// dependency required.
//
// Run with: pnpm test:unit

import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  nextBackoffMs,
  retryDelayMs,
  isIdempotentMethod,
  isNetworkError,
} from '../../lib/resilience.ts';

// nextBackoffMs ────────────────────────────────────────────────────

test('nextBackoffMs grows exponentially for small attempts', () => {
  // No jitter (randomFn = 0) so values are deterministic.
  const r0 = nextBackoffMs(0, { randomFn: () => 0 });
  const r1 = nextBackoffMs(1, { randomFn: () => 0 });
  const r2 = nextBackoffMs(2, { randomFn: () => 0 });
  const r3 = nextBackoffMs(3, { randomFn: () => 0 });

  assert.equal(r0, 1000);
  assert.equal(r1, 2000);
  assert.equal(r2, 4000);
  assert.equal(r3, 8000);
});

test('nextBackoffMs caps at 30s regardless of attempt', () => {
  // 2^10 * 1000 = ~1M ms — far over the cap. Must clamp.
  const capped = nextBackoffMs(10, { randomFn: () => 0 });
  assert.equal(capped, 30_000);
  const extreme = nextBackoffMs(100, { randomFn: () => 0 });
  assert.equal(extreme, 30_000);
});

test('nextBackoffMs jitter stays within [0, jitterMs]', () => {
  // With randomFn returning 0.5 and default jitterMs = 1000, expect
  // base + 500.
  const v = nextBackoffMs(2, { randomFn: () => 0.5 });
  assert.equal(v, 4500);
});

test('nextBackoffMs handles negative attempt as 0', () => {
  // Guards against accidental negative values leaking from store state.
  const v = nextBackoffMs(-1, { randomFn: () => 0 });
  assert.equal(v, 1000);
});

test('nextBackoffMs respects custom cap', () => {
  // WS uses 30s; some callers (tests, specific adapters) might want a
  // tighter cap.
  const v = nextBackoffMs(100, { capMs: 5000, randomFn: () => 0 });
  assert.equal(v, 5000);
});

// retryDelayMs ─────────────────────────────────────────────────────

test('retryDelayMs uses tighter defaults than WS backoff', () => {
  // REST callers block UI — 300ms base, much shorter than WS 1s.
  const r0 = retryDelayMs(0, { randomFn: () => 0 });
  const r1 = retryDelayMs(1, { randomFn: () => 0 });
  const r2 = retryDelayMs(2, { randomFn: () => 0 });
  assert.equal(r0, 300);
  assert.equal(r1, 600);
  assert.equal(r2, 1200);
});

test('retryDelayMs with max jitter', () => {
  const v = retryDelayMs(1, { randomFn: () => 1 });
  assert.equal(v, 800); // 600 base + 200 jitter
});

// isIdempotentMethod ───────────────────────────────────────────────

test('isIdempotentMethod: GET and HEAD only', () => {
  assert.equal(isIdempotentMethod('GET'), true);
  assert.equal(isIdempotentMethod('HEAD'), true);
  assert.equal(isIdempotentMethod('get'), true, 'lowercase normalised');
  assert.equal(isIdempotentMethod('POST'), false);
  assert.equal(isIdempotentMethod('PUT'), false);
  assert.equal(isIdempotentMethod('DELETE'), false);
  assert.equal(isIdempotentMethod('PATCH'), false);
  // OPTIONS is technically safe but rarely hand-issued by app code.
  // Not retrying it is the conservative default.
  assert.equal(isIdempotentMethod('OPTIONS'), false);
});

// isNetworkError ───────────────────────────────────────────────────

test('isNetworkError: TypeError is a network error', () => {
  assert.equal(isNetworkError(new TypeError('fetch failed')), true);
});

test('isNetworkError: AbortError must NOT be retried', () => {
  // The caller explicitly cancelled (AbortController.abort()). Silently
  // retrying would ignore their signal.
  const err = new Error('aborted');
  err.name = 'AbortError';
  assert.equal(isNetworkError(err), false);
});

test('isNetworkError: message-based detection', () => {
  // Some runtimes wrap the fetch error and lose the TypeError type.
  // Fall back to message matching for the common shapes.
  assert.equal(isNetworkError(new Error('ECONNREFUSED')), true);
  assert.equal(isNetworkError(new Error('ENOTFOUND foo.bar')), true);
  assert.equal(isNetworkError(new Error('network request failed')), true);
  assert.equal(isNetworkError(new Error('fetch failed')), true);
});

test('isNetworkError: regular errors are not retryable', () => {
  assert.equal(isNetworkError(new Error('bad input')), false);
  assert.equal(isNetworkError('just a string'), false);
  assert.equal(isNetworkError(null), false);
  assert.equal(isNetworkError(undefined), false);
  assert.equal(isNetworkError(42), false);
});
