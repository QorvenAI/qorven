// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

/** Convert cron expression to human-readable text */
export function cronToHuman(expr: string): string {
  const parts = (expr || '').trim().split(/\s+/);
  if (parts.length < 5) return expr;

  const [min, hour, dom, mon, dow] = parts as [string, string, string, string, string];

  // Common patterns
  if (min === '*' && hour === '*') return 'Every minute';
  if (min === '0' && hour === '*') return 'Every hour';
  if (min === '*/5') return 'Every 5 minutes';
  if (min === '*/10') return 'Every 10 minutes';
  if (min === '*/15') return 'Every 15 minutes';
  if (min === '*/30') return 'Every 30 minutes';

  const h = parseInt(hour);
  const m = parseInt(min);
  if (!isNaN(h) && !isNaN(m)) {
    const time = `${h % 12 || 12}:${m.toString().padStart(2, '0')} ${h >= 12 ? 'PM' : 'AM'}`;

    if (dom === '*' && mon === '*' && dow === '*') return `Every day at ${time}`;
    if (dom === '*' && mon === '*' && dow === '1-5') return `Weekdays at ${time}`;
    if (dom === '*' && mon === '*' && dow === '0') return `Every Sunday at ${time}`;
    if (dom === '*' && mon === '*' && dow === '1') return `Every Monday at ${time}`;
    if (dom === '1' && mon === '*') return `1st of every month at ${time}`;

    const dayNames = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];
    if (dow !== '*') {
      const days = dow.split(',').map((d) => dayNames[parseInt(d)] ?? d as string).join(', ');
      return `${days} at ${time}`;
    }

    return `At ${time}`;
  }

  return expr;
}

/** Time until next run */
export function timeUntil(dateStr: string): string {
  const diff = new Date(dateStr).getTime() - Date.now();
  if (diff < 0) return 'overdue';
  if (diff < 60000) return 'in <1 min';
  if (diff < 3600000) return `in ${Math.round(diff / 60000)} min`;
  if (diff < 86400000) return `in ${Math.round(diff / 3600000)}h`;
  return `in ${Math.round(diff / 86400000)}d`;
}
