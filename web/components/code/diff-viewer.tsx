'use client';
// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useRef, useState } from 'react';
import { cn } from '@/lib/utils';
import { detectLang } from './code-utils';

interface DiffLine {
  type: 'add' | 'del' | 'eq';
  content: string;
  oldLine?: number;
  newLine?: number;
}

// Minimal LCS-based line-level diff
function computeDiff(original: string, modified: string): DiffLine[] {
  const aLines = original.split('\n');
  const bLines = modified.split('\n');
  const n = aLines.length;
  const m = bLines.length;

  // LCS DP table (length only — reconstruct via back-trace)
  const dp: number[][] = Array.from({ length: n + 1 }, () => new Array(m + 1).fill(0));
  for (let i = 1; i <= n; i++) {
    for (let j = 1; j <= m; j++) {
      dp[i]![j] = aLines[i - 1] === bLines[j - 1]
        ? dp[i - 1]![j - 1]! + 1
        : Math.max(dp[i - 1]![j]!, dp[i]![j - 1]!);
    }
  }

  // Back-trace to build diff
  const result: DiffLine[] = [];
  let i = n, j = m;
  let oldCount = 0, newCount = 0;
  const ops: Array<{ type: 'add' | 'del' | 'eq'; a?: number; b?: number }> = [];

  while (i > 0 || j > 0) {
    if (i > 0 && j > 0 && aLines[i - 1] === bLines[j - 1]) {
      ops.push({ type: 'eq', a: i, b: j });
      i--; j--;
    } else if (j > 0 && (i === 0 || dp[i]![j - 1]! >= dp[i - 1]![j]!)) {
      ops.push({ type: 'add', b: j });
      j--;
    } else {
      ops.push({ type: 'del', a: i });
      i--;
    }
  }
  ops.reverse();

  let ol = 1, nl = 1;
  for (const op of ops) {
    if (op.type === 'eq') {
      result.push({ type: 'eq', content: aLines[op.a! - 1]!, oldLine: ol++, newLine: nl++ });
    } else if (op.type === 'del') {
      result.push({ type: 'del', content: aLines[op.a! - 1]!, oldLine: ol++ });
    } else {
      result.push({ type: 'add', content: bLines[op.b! - 1]!, newLine: nl++ });
    }
  }
  return result;
}

function countStats(lines: DiffLine[]) {
  let added = 0, removed = 0;
  for (const l of lines) {
    if (l.type === 'add') added++;
    if (l.type === 'del') removed++;
  }
  return { added, removed };
}

interface DiffViewerProps {
  path: string;
  original: string;
  modified: string;
  className?: string;
}

export function DiffViewer({ path, original, modified, className }: DiffViewerProps) {
  const lines = computeDiff(original, modified);
  const { added, removed } = countStats(lines);
  const name = path.split('/').pop() || path;

  return (
    <div className={cn('flex flex-col h-full overflow-hidden', className)}>
      {/* Header */}
      <div className="flex shrink-0 items-center gap-3 border-b border-border bg-muted/20 px-4 py-2">
        <span className="font-mono text-xs text-foreground font-medium truncate flex-1">{name}</span>
        <span className="text-xs text-muted-foreground font-mono truncate max-w-[200px]">{path}</span>
        <div className="flex items-center gap-1.5">
          {added > 0 && (
            <span className="rounded border border-emerald-500/30 bg-emerald-500/10 px-1.5 py-0.5 font-mono text-2xs text-emerald-400">+{added}</span>
          )}
          {removed > 0 && (
            <span className="rounded border border-destructive/30 bg-destructive/10 px-1.5 py-0.5 font-mono text-2xs text-destructive">−{removed}</span>
          )}
        </div>
      </div>

      {/* Diff lines */}
      <div className="flex-1 overflow-auto">
        <table className="w-full border-collapse font-mono text-xs" style={{ minWidth: '100%' }}>
          <tbody>
            {lines.map((line, idx) => (
              <tr
                key={idx}
                className={cn(
                  'group leading-5',
                  line.type === 'add' && 'bg-emerald-500/8',
                  line.type === 'del' && 'bg-destructive/8',
                )}
              >
                {/* Old line number */}
                <td className="w-10 select-none pr-2 text-right text-2xs text-muted-foreground/40 border-r border-border/50 pl-2">
                  {line.oldLine ?? ''}
                </td>
                {/* New line number */}
                <td className="w-10 select-none pr-2 text-right text-2xs text-muted-foreground/40 border-r border-border/50">
                  {line.newLine ?? ''}
                </td>
                {/* Gutter indicator */}
                <td className={cn(
                  'w-5 select-none text-center text-2xs border-r border-border/50',
                  line.type === 'add' && 'text-emerald-400 border-l-2 border-l-emerald-500',
                  line.type === 'del' && 'text-destructive border-l-2 border-l-destructive',
                  line.type === 'eq' && 'text-transparent',
                )}>
                  {line.type === 'add' ? '+' : line.type === 'del' ? '−' : ' '}
                </td>
                {/* Content */}
                <td className={cn(
                  'px-3 py-px whitespace-pre overflow-hidden',
                  line.type === 'add' && 'text-emerald-300',
                  line.type === 'del' && 'text-red-300 line-through opacity-70',
                  line.type === 'eq' && 'text-foreground/60',
                )}>
                  {line.content}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {lines.length === 0 && (
          <div className="flex h-32 items-center justify-center text-xs text-muted-foreground">
            No changes
          </div>
        )}
      </div>
    </div>
  );
}
