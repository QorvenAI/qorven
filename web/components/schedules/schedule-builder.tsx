'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useState, useEffect } from 'react';
import { cronToHuman } from '@/components/cron/cron-utils';
import { cn } from '@/lib/utils';

type Mode = 'hourly' | 'daily' | 'weekly' | 'once' | 'advanced';

export interface ScheduleValue {
  cron_expression: string;
  one_shot: boolean;
}

const WEEKDAYS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];
const MODES: { id: Mode; label: string }[] = [
  { id: 'hourly', label: 'Hourly' },
  { id: 'daily', label: 'Daily' },
  { id: 'weekly', label: 'Weekly' },
  { id: 'once', label: 'One-time' },
  { id: 'advanced', label: 'Advanced' },
];

function detectMode(expr: string): Mode {
  if (!expr) return 'daily';
  const parts = expr.trim().split(/\s+/);
  if (parts.length !== 5) return 'advanced';
  const [min, hour, dom, mon, dow] = parts as [string, string, string, string, string];
  if (min === '0' && hour === '*' && dom === '*' && mon === '*' && dow === '*') return 'hourly';
  const isNum = (s: string) => /^\d+$/.test(s);
  if (isNum(min) && isNum(hour) && dom === '*' && mon === '*' && dow === '*') return 'daily';
  if (isNum(min) && isNum(hour) && dom === '*' && mon === '*' && isNum(dow)) return 'weekly';
  return 'advanced';
}

function exprToTime(expr: string): string {
  const parts = expr.trim().split(/\s+/);
  if (parts.length < 2) return '09:00';
  const min = parseInt(parts[0]!, 10);
  const hour = parseInt(parts[1]!, 10);
  if (isNaN(hour) || isNaN(min)) return '09:00';
  return `${String(hour).padStart(2, '0')}:${String(min).padStart(2, '0')}`;
}

function exprToWeekday(expr: string): number {
  const parts = expr.trim().split(/\s+/);
  if (parts.length < 5) return 1;
  const dow = parseInt(parts[4]!, 10);
  return isNaN(dow) ? 1 : dow;
}

export function ScheduleBuilder({
  value,
  onChange,
}: {
  value: ScheduleValue;
  onChange: (v: ScheduleValue) => void;
}) {
  const initial = value.cron_expression || '0 9 * * *';
  const [mode, setMode] = useState<Mode>(() => detectMode(initial));
  const [time, setTime] = useState(() => exprToTime(initial));
  const [weekday, setWeekday] = useState(() => exprToWeekday(initial));
  const [datetime, setDatetime] = useState('');
  const [raw, setRaw] = useState(initial);

  useEffect(() => {
    let expr = raw;
    let once = false;
    const [hhStr, mmStr] = time.split(':');
    const hh = parseInt(hhStr ?? '9', 10);
    const mm = parseInt(mmStr ?? '0', 10);

    if (mode === 'hourly') {
      expr = '0 * * * *';
    } else if (mode === 'daily') {
      expr = `${mm} ${hh} * * *`;
    } else if (mode === 'weekly') {
      expr = `${mm} ${hh} * * ${weekday}`;
    } else if (mode === 'once') {
      if (datetime) {
        const d = new Date(datetime);
        once = true;
        expr = `${d.getMinutes()} ${d.getHours()} ${d.getDate()} ${d.getMonth() + 1} *`;
      }
    } else {
      expr = raw;
    }

    onChange({ cron_expression: expr, one_shot: once });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [mode, time, weekday, datetime, raw]);

  const preview = (() => {
    try {
      return cronToHuman(value.cron_expression);
    } catch {
      return value.cron_expression;
    }
  })();

  return (
    <div className="space-y-3">
      {/* Mode pills */}
      <div className="flex gap-2 flex-wrap">
        {MODES.map((m) => (
          <button
            key={m.id}
            type="button"
            onClick={() => setMode(m.id)}
            className={cn(
              'qr-btn qr-btn-sm',
              mode === m.id ? 'qr-btn-primary' : 'qr-btn-outline',
            )}
          >
            {m.label}
          </button>
        ))}
      </div>

      {/* Daily — time picker */}
      {mode === 'daily' && (
        <div className="space-y-1">
          <label className="text-xs text-muted-foreground">Time</label>
          <input
            type="time"
            value={time}
            onChange={(e) => setTime(e.target.value)}
            className="qr-input"
          />
        </div>
      )}

      {/* Weekly — weekday + time */}
      {mode === 'weekly' && (
        <div className="space-y-3">
          <div className="space-y-1">
            <label className="text-xs text-muted-foreground">Day of week</label>
            <div className="flex flex-wrap gap-1.5">
              {WEEKDAYS.map((d, i) => (
                <button
                  key={d}
                  type="button"
                  onClick={() => setWeekday(i)}
                  className={cn(
                    'qr-btn qr-btn-xs',
                    weekday === i ? 'qr-btn-primary' : 'qr-btn-outline',
                  )}
                >
                  {d}
                </button>
              ))}
            </div>
          </div>
          <div className="space-y-1">
            <label className="text-xs text-muted-foreground">Time</label>
            <input
              type="time"
              value={time}
              onChange={(e) => setTime(e.target.value)}
              className="qr-input"
            />
          </div>
        </div>
      )}

      {/* One-time */}
      {mode === 'once' && (
        <div className="space-y-1">
          <label className="text-xs text-muted-foreground">Date and time</label>
          <input
            type="datetime-local"
            value={datetime}
            onChange={(e) => setDatetime(e.target.value)}
            className="qr-input"
          />
        </div>
      )}

      {/* Advanced — raw expression */}
      {mode === 'advanced' && (
        <div className="space-y-1">
          <label className="text-xs text-muted-foreground">Cron expression</label>
          <input
            type="text"
            value={raw}
            onChange={(e) => setRaw(e.target.value)}
            placeholder="0 9 * * *"
            className="qr-input font-mono"
          />
        </div>
      )}

      {/* Human preview */}
      {value.cron_expression && (
        <div className="rounded-lg border border-border bg-muted/30 px-3 py-2 space-y-0.5">
          <p className="text-xs text-muted-foreground">
            Runs:{' '}
            <span className="font-medium text-foreground">{preview}</span>
          </p>
          <p className="text-2xs font-mono text-muted-foreground/70">{value.cron_expression}</p>
        </div>
      )}
    </div>
  );
}
