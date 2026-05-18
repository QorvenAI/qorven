// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

export function relativeTime(ts: string | number | Date): string {
  const now = Date.now();
  const date = new Date(ts);
  const diff = now - date.getTime();
  const sec = Math.floor(diff / 1000);
  const min = Math.floor(sec / 60);
  const hr = Math.floor(min / 60);
  const day = Math.floor(hr / 24);

  if (sec < 10) return 'now';
  if (sec < 60) return `${sec}s ago`;
  if (min < 60) return `${min}m ago`;
  if (hr < 24) return `${hr}h ago`;

  const isToday = new Date().toDateString() === date.toDateString();
  const yesterday = new Date(now - 86400000);
  const isYesterday = yesterday.toDateString() === date.toDateString();

  const time = date.toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' });
  if (isToday) return `today ${time}`;
  if (isYesterday) return `yesterday ${time}`;
  if (day < 7) return `${date.toLocaleDateString([], { weekday: 'short' })} ${time}`;
  return date.toLocaleDateString([], { month: 'short', day: 'numeric' }) + ` ${time}`;
}
