'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useEffect, useRef, useState } from 'react';
import { AlertCircle, CheckCircle2, GitBranch, Loader2, Play, StopCircle, Zap } from 'lucide-react';
import { cn } from '@/lib/utils';
import type { BuildEntry } from './code-types';
import { FileChangeChip } from './file-change-chip';
import { PrCard } from './pr-card';
import { CommandTimeline } from './command-timeline';

type BuildView = 'log' | 'timeline';

export function BuildLog({ entries, running, onStop, onFileClick, summary, onOpenSession }: {
  entries: BuildEntry[];
  running: boolean;
  onStop: () => void;
  onFileClick?: (path: string) => void;
  onOpenSession?: () => void;
  summary?: { files: number; agents: number; prUrl?: string; previewUrl?: string; elapsed?: string };
}) {
  const bottomRef = useRef<HTMLDivElement>(null);
  const [view, setView] = useState<BuildView>('log');
  useEffect(() => {
    if (view === 'log') bottomRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [entries, view]);

  return (
    <div className="flex h-full flex-col">
      <div className="flex h-[35px] shrink-0 items-center gap-2 border-b border-border px-3">
        <div className="flex items-center gap-1.5 flex-1">
          {running
            ? <Loader2 className="h-3.5 w-3.5 animate-spin text-primary" />
            : <CheckCircle2 className="h-3.5 w-3.5 text-emerald-500" />}
          <span className="text-xs font-medium">{running ? 'Building…' : 'Build complete'}</span>
        </div>

        {/* View toggle */}
        <div className="flex items-center rounded-md border border-border overflow-hidden text-xs shrink-0">
          {(['log', 'timeline'] as BuildView[]).map(v => (
            <button key={v} onClick={() => setView(v)}
              className={cn('px-2.5 py-0.5 capitalize transition-colors',
                view === v ? 'bg-primary text-primary-foreground' : 'text-muted-foreground hover:text-foreground hover:bg-accent')}>
              {v}
            </button>
          ))}
        </div>

        {running && (
          <button onClick={onStop} className="flex items-center gap-1 text-xs text-destructive hover:underline shrink-0">
            <StopCircle className="h-3 w-3" /> Stop
          </button>
        )}
      </div>

      {view === 'timeline' ? (
        <CommandTimeline entries={entries} running={running} onFileClick={onFileClick} />
      ) : (
      <div className="flex-1 overflow-y-auto p-3 space-y-1 font-mono text-xs">
        {entries.map((e, i) => {
          if (e.type === 'text') return (
            <div key={i} className="text-foreground/80 whitespace-pre-wrap leading-relaxed">{e.content}</div>
          );
          if (e.type === 'tool_start') return (
            <div key={i} className="flex items-center gap-1.5 text-primary/80">
              <Zap className="h-3 w-3 shrink-0" />
              <span className="font-semibold">{e.tool}</span>
              <span className="text-muted-foreground truncate">{e.content}</span>
            </div>
          );
          if (e.type === 'file_created' && e.path) return (
            <button key={i}
              onClick={() => e.path && onFileClick?.(e.path)}
              className="flex w-full items-center gap-1.5 text-emerald-400 hover:text-emerald-300 hover:bg-emerald-500/5 rounded px-1 -mx-1 transition-colors text-left group"
            >
              <CheckCircle2 className="h-3 w-3 shrink-0" />
              <span className="truncate">{e.path}</span>
              <span className="ml-auto opacity-0 group-hover:opacity-100 text-xs text-emerald-400/60 shrink-0">open →</span>
            </button>
          );
          if (e.type === 'file_created') return (
            <div key={i} className="flex items-center gap-1.5 text-emerald-400">
              <CheckCircle2 className="h-3 w-3 shrink-0" />
              <span className="text-muted-foreground">{e.content}</span>
            </div>
          );
          if (e.type === 'file_chip') return (
            <div key={i} className="px-1">
              <FileChangeChip
                path={e.path || e.content}
                linesAdded={e.linesAdded}
                linesRemoved={e.linesRemoved}
                totalLines={e.totalLines}
                onClick={onFileClick}
              />
            </div>
          );
          if (e.type === 'pr_card') return (
            <div key={i} className="px-1">
              <PrCard
                prUrl={e.prUrl}
                prTitle={e.prTitle}
                prNumber={e.prNumber}
                prRepo={e.prRepo}
                onViewPr={onOpenSession}
              />
            </div>
          );
          if (e.type === 'error') return (
            <div key={i} className="flex items-center gap-1.5 text-destructive">
              <AlertCircle className="h-3 w-3 shrink-0" />
              <span>{e.content}</span>
            </div>
          );
          if (e.type === 'done') return null;
          return null;
        })}
        <div ref={bottomRef} />
      </div>
      )}

      {!running && summary && summary.files > 0 && (
        <div className="shrink-0 border-t border-border bg-card p-3 space-y-2">
          <div className="flex items-center gap-2">
            <CheckCircle2 className="h-4 w-4 text-emerald-500 shrink-0" />
            <span className="text-xs font-semibold">Build complete</span>
            {summary.elapsed && <span className="ml-auto text-xs text-muted-foreground">{summary.elapsed}</span>}
          </div>
          <div className="grid grid-cols-2 gap-1.5">
            <div className="rounded-md bg-muted/50 px-2 py-1 text-center">
              <p className="text-base font-bold">{summary.files}</p>
              <p className="text-xs text-muted-foreground">files</p>
            </div>
            <div className="rounded-md bg-muted/50 px-2 py-1 text-center">
              <p className="text-base font-bold">{summary.agents}</p>
              <p className="text-xs text-muted-foreground">agents</p>
            </div>
          </div>
          <div className="flex flex-col gap-1">
            {summary.prUrl && (
              <a href={summary.prUrl} target="_blank" rel="noopener noreferrer"
                className="flex items-center gap-1.5 rounded-md border border-border bg-card px-2.5 py-1.5 text-xs text-primary hover:bg-accent transition-colors">
                <GitBranch className="h-3.5 w-3.5 shrink-0" />
                View Pull Request
              </a>
            )}
            {summary.previewUrl && (
              <button onClick={() => window.open(summary.previewUrl, '_blank')}
                className="flex items-center gap-1.5 rounded-md border border-emerald-500/30 bg-emerald-500/5 px-2.5 py-1.5 text-xs text-emerald-600 hover:bg-emerald-500/10 transition-colors">
                <Play className="h-3.5 w-3.5 shrink-0" />
                Open Preview
              </button>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
