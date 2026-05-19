'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

/**
 * /research — multi-stage web research (T2.8).
 *
 * POST /v1/research/start kicks off a job; we then poll GET /v1/research/{id}
 * every 1.5s until it hits a terminal status. The job exposes an
 * in-memory progress log (decompose → search → analyze → synthesize
 * → extract) that we render live.
 *
 * Mode maps to backend quick/balanced/quality — same semantic tiers
 * as Council depth. Sources are shown with title + url, and the
 * synthesis lands below in Markdown-friendly plain text.
 */

import { useEffect, useRef, useState } from 'react';
import {
  Search, Send, Loader2, Zap, Scale, Layers, ExternalLink, AlertCircle,
  CheckCircle2, XCircle, BookOpen, RotateCw,
} from 'lucide-react';
import { CanvasHeader } from '@/components/layouts/canvas-header';
import { cn } from '@/lib/utils';
import { research, type ResearchJob, type ResearchMode } from '@/lib/api';

const MODES: { key: ResearchMode; label: string; subtitle: string; icon: typeof Zap }[] = [
  { key: 'quick',    label: 'Quick',    subtitle: '1 pass · fast',             icon: Zap },
  { key: 'balanced', label: 'Balanced', subtitle: '2-3 passes · summarized',   icon: Scale },
  { key: 'quality',  label: 'Quality',  subtitle: 'Deep · extraction + synth', icon: Layers },
];

const STEP_LABEL: Record<string, string> = {
  decompose:  'Decomposing query',
  search:     'Searching the web',
  analyze:    'Analyzing results',
  synthesize: 'Synthesizing answer',
  extract:    'Extracting citations',
};

export default function ResearchPage() {
  const [query, setQuery] = useState('');
  const [mode, setMode] = useState<ResearchMode>('balanced');
  const [job, setJob] = useState<ResearchJob | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [starting, setStarting] = useState(false);
  const pollRef = useRef<number | null>(null);

  // Polling loop — starts when we get a job id, stops on terminal state
  // or component unmount. 1.5s cadence is enough for the research
  // backend which tends to advance a step every 2-4 seconds.
  useEffect(() => {
    if (!job || job.status !== 'running') return;
    const tick = async () => {
      try {
        const next = await research.get(job.id);
        setJob(next);
        if (next.status === 'running') {
          pollRef.current = window.setTimeout(tick, 1500);
        }
      } catch (e) {
        setErr(e instanceof Error ? e.message : 'Polling failed');
      }
    };
    pollRef.current = window.setTimeout(tick, 1500);
    return () => {
      if (pollRef.current) clearTimeout(pollRef.current);
    };
  }, [job]);

  const start = async () => {
    if (!query.trim() || starting) return;
    setErr(null);
    setJob(null);
    setStarting(true);
    try {
      const { id } = await research.start({ query: query.trim(), mode });
      setJob({ id, query: query.trim(), mode, status: 'running' });
    } catch (e) {
      setErr(e instanceof Error ? e.message : 'Research failed to start');
    } finally {
      setStarting(false);
    }
  };

  const running = job?.status === 'running';
  const completed = job?.status === 'completed';
  const failed = job?.status === 'failed';

  return (
    <div className="mx-auto max-w-4xl space-y-5 p-4 lg:p-6">
      <CanvasHeader title="Research" description="Decompose a question into sub-queries, search the web in parallel, extract + cite, synthesize." />

      {/* Mode picker */}
      <div className="grid grid-cols-3 gap-2">
        {MODES.map(({ key, label, subtitle, icon: Icon }) => {
          const active = mode === key;
          return (
            <button
              key={key}
              onClick={() => !running && setMode(key)}
              disabled={running}
              className={cn(
                'flex items-start gap-2.5 rounded-xl border p-3 text-left transition-colors',
                active
                  ? 'border-primary bg-primary/10'
                  : 'border-border bg-card hover:border-border/80 hover:bg-accent/40',
                running && 'cursor-not-allowed opacity-60',
              )}
            >
              <Icon className={cn('mt-0.5 h-4 w-4 shrink-0', active ? 'text-primary' : 'text-muted-foreground')} />
              <div className="min-w-0 flex-1">
                <div className="text-sm font-medium">{label}</div>
                <div className="mt-0.5 text-2xs text-muted-foreground">{subtitle}</div>
              </div>
            </button>
          );
        })}
      </div>

      {/* Compose */}
      <div className="flex flex-col gap-2 rounded-xl border border-border bg-card p-3 focus-within:border-primary/60">
        <textarea
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
              e.preventDefault();
              start();
            }
          }}
          rows={2}
          disabled={running || starting}
          placeholder="What do you want researched?"
          className="w-full resize-none bg-transparent text-sm outline-none placeholder:text-muted-foreground/60 disabled:opacity-60"
        />
        <div className="flex items-center justify-between gap-2 border-t border-border/60 pt-2">
          <span className="text-2xs text-muted-foreground">
            <kbd className="rounded bg-muted px-1 py-0.5 font-mono text-xs">⌘ Enter</kbd> to start
          </span>
          <button
            onClick={start}
            disabled={running || starting || !query.trim()}
            className="inline-flex items-center gap-1.5 rounded-md bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
          >
            {starting || running ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Send className="h-3.5 w-3.5" />}
            {running ? 'Researching' : 'Start'}
          </button>
        </div>
      </div>

      {err && (
        <div className="flex items-start gap-2 rounded-lg border border-destructive/40 bg-destructive/5 p-3 text-xs text-destructive">
          <AlertCircle className="mt-0.5 h-4 w-4 shrink-0" />
          <span>{err}</span>
        </div>
      )}

      {/* Progress */}
      {job && (
        <ProgressTrail job={job} />
      )}

      {/* Failed */}
      {failed && job?.error && (
        <div className="flex items-start gap-2 rounded-lg border border-destructive/40 bg-destructive/5 p-3 text-xs text-destructive">
          <XCircle className="mt-0.5 h-4 w-4 shrink-0" />
          <div>
            <p className="font-medium">Research failed</p>
            <p className="mt-0.5 whitespace-pre-wrap text-destructive/80">{job.error}</p>
          </div>
        </div>
      )}

      {/* Report */}
      {completed && job?.report && <ReportCard job={job} />}
    </div>
  );
}

function ProgressTrail({ job }: { job: ResearchJob }) {
  const progress = job.progress ?? [];
  const running = job.status === 'running';
  return (
    <section className="rounded-xl border border-border bg-card/40 p-3">
      <header className="flex items-center gap-2 border-b border-border/60 pb-2">
        {running ? (
          <Loader2 className="h-3.5 w-3.5 animate-spin text-primary" />
        ) : job.status === 'completed' ? (
          <CheckCircle2 className="h-3.5 w-3.5 text-emerald-500" />
        ) : (
          <XCircle className="h-3.5 w-3.5 text-destructive" />
        )}
        <h2 className="text-xs font-semibold tracking-wider">
          {running ? 'IN PROGRESS' : job.status === 'completed' ? 'COMPLETED' : 'FAILED'}
        </h2>
        <span className="ml-auto font-mono text-2xs text-muted-foreground">
          {job.mode} · job {job.id.slice(0, 8)}
        </span>
      </header>
      {progress.length === 0 ? (
        <p className="pt-2 text-2xs text-muted-foreground">Waiting for first step…</p>
      ) : (
        <ul className="mt-2 space-y-1">
          {progress.map((p, i) => (
            <li key={i} className="flex items-start gap-2 text-2xs">
              <span className="mt-1 inline-block h-1.5 w-1.5 shrink-0 rounded-full bg-primary/80" />
              <div className="min-w-0 flex-1">
                <div className="font-mono font-medium">{STEP_LABEL[p.step] ?? p.step}</div>
                <div className="text-muted-foreground">
                  {p.detail}
                  {p.sources != null && <span className="ml-1 text-muted-foreground/70">· {p.sources} sources</span>}
                </div>
              </div>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

function ReportCard({ job }: { job: ResearchJob }) {
  const report = job.report!;
  return (
    <div className="space-y-4">
      <section className="rounded-xl border border-primary/30 bg-primary/5 p-5">
        <div className="flex items-center gap-2">
          <BookOpen className="h-4 w-4 text-primary" />
          <h2 className="text-sm font-semibold uppercase tracking-wider text-primary">Synthesis</h2>
        </div>
        <p className="mt-3 whitespace-pre-wrap text-sm leading-relaxed text-foreground/90">
          {report.answer}
        </p>
      </section>

      {report.sources.length > 0 && (
        <section className="rounded-xl border border-border bg-card/40">
          <header className="flex items-center gap-2 border-b border-border/60 px-4 py-2.5">
            <RotateCw className="h-3.5 w-3.5 text-muted-foreground" />
            <h2 className="text-xs font-semibold tracking-wider">SOURCES ({report.sources.length})</h2>
          </header>
          <ul className="divide-y divide-border/60">
            {report.sources.map((s, i) => (
              <li key={i} className="p-3 text-xs">
                <a
                  href={s.url}
                  target="_blank"
                  rel="noreferrer noopener"
                  className="flex items-start gap-2 text-primary hover:underline"
                >
                  <ExternalLink className="mt-0.5 h-3 w-3 shrink-0" />
                  <span className="flex-1 font-medium">{s.title || s.url}</span>
                </a>
                <div className="mt-1 truncate font-mono text-2xs text-muted-foreground">{s.url}</div>
                {s.text && (
                  <p className="mt-1 line-clamp-3 leading-relaxed text-muted-foreground">
                    {s.text}
                  </p>
                )}
              </li>
            ))}
          </ul>
        </section>
      )}
    </div>
  );
}
