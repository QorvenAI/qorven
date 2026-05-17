// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import type { DateFormat } from './theme-provider';

const STORAGE_KEY = 'qorven-theme';

function getPrefs(): { dateFormat: DateFormat; timezone: string } {
  if (typeof window === 'undefined') return { dateFormat: 'relative', timezone: 'UTC' };
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (stored) {
      const p = JSON.parse(stored);
      return {
        dateFormat: p.dateFormat ?? 'relative',
        timezone: p.timezone ?? Intl.DateTimeFormat().resolvedOptions().timeZone,
      };
    }
  } catch {}
  return {
    dateFormat: 'relative',
    timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
  };
}

export function formatDate(
  value: string | number | Date | null | undefined,
  overrideFormat?: DateFormat,
  overrideTz?: string,
): string {
  if (value == null || value === '') return '—';
  const date = value instanceof Date ? value : new Date(value);
  if (isNaN(date.getTime())) return '—';

  const { dateFormat, timezone } = getPrefs();
  const fmt = overrideFormat ?? dateFormat;
  const tz = overrideTz ?? timezone;

  if (fmt === 'relative') {
    const diffMs = Date.now() - date.getTime();
    const abs = Math.abs(diffMs);
    const future = diffMs < 0;
    const prefix = future ? 'in ' : '';
    const suffix = future ? '' : ' ago';
    if (abs < 60_000) return 'just now';
    if (abs < 3_600_000) {
      const m = Math.round(abs / 60_000);
      return `${prefix}${m}m${suffix}`;
    }
    if (abs < 86_400_000) {
      const h = Math.round(abs / 3_600_000);
      return `${prefix}${h}h${suffix}`;
    }
    if (abs < 7 * 86_400_000) {
      const d = Math.round(abs / 86_400_000);
      return `${prefix}${d}d${suffix}`;
    }
    // Fall through to short for older dates
    return date.toLocaleDateString(undefined, { timeZone: tz, month: 'short', day: 'numeric', year: 'numeric' });
  }

  if (fmt === 'short') {
    return date.toLocaleString(undefined, {
      timeZone: tz, month: 'short', day: 'numeric',
      hour: 'numeric', minute: '2-digit',
    });
  }

  if (fmt === 'long') {
    return date.toLocaleString(undefined, {
      timeZone: tz, dateStyle: 'medium', timeStyle: 'short',
    });
  }

  // iso
  return date.toISOString();
}
