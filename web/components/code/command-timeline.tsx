'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useState } from 'react';
import { ChevronRight, AlertCircle, CheckCircle2, Loader2 } from 'lucide-react';
import { cn } from '@/lib/utils';
import type { BuildEntry } from './code-types';

interface TimelineRow {
  index: number;        // original position in buildLog
  tool: string;
  label: string;
  status: 'done' | 'running' | 'error' | 'pending';
  ts?: number;
  durationMs?: number;
  children: BuildEntry[]; // log entries between this tool_start and the next
}

function buildTimeline(entries: BuildEntry[], running: boolean): TimelineRow[] {
  const rows: TimelineRow[] = [];
  let current: TimelineRow | null = null;

  for (let i = 0; i < entries.length; i++) {
    const e = entries[i]!;
    if (e.type === 'tool_start') {
      if (current) {
        // Close previous row — compute duration from next tool_start ts
        if (current.ts && e.ts) current.durationMs = e.ts - current.ts;
        rows.push(current);
      }
      current = {
        index: i,
        tool: e.tool || '',
        label: e.content || e.tool || '',
        status: 'done',
        ts: e.ts,
        children: [],
      };
    } else if (current) {
      if (e.type === 'error') current.status = 'error';
      current.children.push(e);
    }
  }

  if (current) {
    current.status = running ? 'running' : current.status;
    rows.push(current);
  }

  return rows;
}

function StatusDot({ status }: { status: TimelineRow['status'] }) {
  if (status === 'running') return (
    <span className="flex h-2 w-2 shrink-0 items-center justify-center">
      <Loader2 className="h-2.5 w-2.5 animate-spin text-amber-400" />
    </span>
  );
  if (status === 'error') return (
    <span className="h-2 w-2 shrink-0 rounded-full bg-destructive" />
  );
  if (status === 'done') return (
    <span className="h-2 w-2 shrink-0 rounded-full bg-emerald-500" />
  );
  return <span className="h-2 w-2 shrink-0 rounded-full bg-muted-foreground/30" />;
}

function formatDuration(ms?: number): string {
  if (!ms || ms < 0) return '';
  if (ms < 1000) return `${ms}ms`;
  return `${(ms / 1000).toFixed(1)}s`;
}

function TimelineRowItem({ row, onFileClick }: {
  row: TimelineRow;
  onFileClick?: (path: string) => void;
}) {
  const [expanded, setExpanded] = useState(false);
  const hasDetail = row.children.length > 0;
  const fileChips = row.children.filter(e => e.type === 'file_chip' || (e.type === 'file_created' && e.path));
  const errors = row.children.filter(e => e.type === 'error');

  return (
    <div className="group">
      <button
        onClick={() => hasDetail && setExpanded(v => !v)}
        className={cn(
          'flex w-full items-center gap-2.5 rounded-md px-2 py-1.5 text-left text-xs transition-colors',
          hasDetail ? 'hover:bg-accent/40 cursor-pointer' : 'cursor-default',
          row.status === 'error' && 'bg-destructive/5',
          row.status === 'running' && 'bg-primary/5',
        )}
      >
        {/* Connector line */}
        <div className="flex w-2 shrink-0 flex-col items-center">
          <StatusDot status={row.status} />
        </div>

        {/* Tool name */}
        <span className={cn(
          'shrink-0 font-mono font-medium',
          row.status === 'error' ? 'text-destructive' :
          row.status === 'running' ? 'text-primary' : 'text-foreground',
        )}>
          {row.tool || '—'}
        </span>

        {/* Label / snippet */}
        <span className="flex-1 truncate text-muted-foreground">{row.label}</span>

        {/* Stats */}
        <div className="ml-auto flex shrink-0 items-center gap-2">
          {fileChips.length > 0 && (
            <span className="text-2xs text-emerald-500 font-mono">+{fileChips.length} files</span>
          )}
          {errors.length > 0 && (
            <AlertCircle className="h-3 w-3 text-destructive" />
          )}
          {row.durationMs !== undefined && (
            <span className="text-2xs text-muted-foreground/60 font-mono tabular-nums">
              {formatDuration(row.durationMs)}
            </span>
          )}
          {hasDetail && (
            <ChevronRight className={cn('h-3 w-3 text-muted-foreground/40 transition-transform', expanded && 'rotate-90')} />
          )}
        </div>
      </button>

      {expanded && row.children.length > 0 && (
        <div className="ml-[18px] mt-0.5 mb-1 space-y-0.5 border-l border-border/60 pl-3">
          {row.children.map((c, i) => {
            if (c.type === 'file_chip' || (c.type === 'file_created' && c.path)) {
              const p = c.path || c.content;
              return (
                <button key={i} onClick={() => onFileClick?.(p)}
                  className="flex w-full items-center gap-1.5 rounded px-1 py-0.5 text-xs text-emerald-500 hover:bg-emerald-500/5 transition-colors text-left">
                  <CheckCircle2 className="h-2.5 w-2.5 shrink-0" />
                  <span className="truncate font-mono">{p.split('/').pop()}</span>
                  {(c.linesAdded || 0) > 0 && <span className="ml-auto text-2xs font-mono">+{c.linesAdded}</span>}
                </button>
              );
            }
            if (c.type === 'error') return (
              <div key={i} className="flex items-center gap-1.5 px-1 py-0.5 text-xs text-destructive">
                <AlertCircle className="h-2.5 w-2.5 shrink-0" />
                <span className="truncate">{c.content}</span>
              </div>
            );
            if (c.type === 'text' && c.content.trim()) return (
              <div key={i} className="px-1 py-0.5 text-xs text-muted-foreground/70 leading-relaxed font-mono truncate">
                {c.content.trim().split('\n')[0]}
              </div>
            );
            return null;
          })}
        </div>
      )}
    </div>
  );
}

export function CommandTimeline({ entries, running, onFileClick }: {
  entries: BuildEntry[];
  running: boolean;
  onFileClick?: (path: string) => void;
}) {
  const rows = buildTimeline(entries, running);

  if (rows.length === 0) {
    return (
      <div className="flex flex-1 flex-col items-center justify-center gap-2 p-6 text-center">
        <p className="text-xs text-muted-foreground">No commands yet</p>
      </div>
    );
  }

  return (
    <div className="flex-1 overflow-y-auto px-2 py-2 space-y-0.5">
      {rows.map(row => (
        <TimelineRowItem key={row.index} row={row} onFileClick={onFileClick} />
      ))}
    </div>
  );
}
