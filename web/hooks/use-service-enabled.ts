'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState } from 'react';
import { userPrefs } from '@/lib/api';

// Module-level singleton: one fetch shared across all consumers, instant
// propagation when services-settings calls notifyPrefsChange().
let cachedPrefs: Record<string, unknown> | null = null;
let fetchPromise: Promise<void> | null = null;
const listeners = new Set<() => void>();

function broadcast() {
  listeners.forEach(fn => fn());
}

// Called by services-settings immediately after saving a toggle — all
// subscribed components re-render with the new value without a page reload.
export function notifyPrefsChange(patch: Record<string, unknown>) {
  cachedPrefs = { ...(cachedPrefs ?? {}), ...patch };
  broadcast();
}

function ensureLoaded() {
  if (cachedPrefs !== null || fetchPromise !== null) return;
  fetchPromise = userPrefs
    .get()
    .then((p) => {
      cachedPrefs = (p ?? {}) as Record<string, unknown>;
      broadcast();
    })
    .catch(() => {
      cachedPrefs = {};
      broadcast();
    });
}

export function useServiceEnabled(key: string): { enabled: boolean; loading: boolean } {
  const [tick, setTick] = useState(0);

  useEffect(() => {
    const rerender = () => setTick(t => t + 1);
    listeners.add(rerender);
    ensureLoaded();
    return () => { listeners.delete(rerender); };
  }, []);

  if (cachedPrefs === null) return { enabled: false, loading: true };
  return { enabled: !!cachedPrefs[key], loading: false };
}
