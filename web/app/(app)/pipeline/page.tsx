'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

/**
 * /pipeline — propose → validate → apply workflow (T2.5).
 *
 * Left rail: list of recent proposals (pending + applied mixed,
 * pending highlighted). Main: selected proposal — description, risk,
 * file diff, compile/test validation results, and the action buttons
 * wired to /pipeline/validate/{id} + /pipeline/apply/{id}.
 *
 * This is NOT a PR editor — we don't let the user *write* proposals
 * from here; agents create them via the tool. The page is the review
 * surface: inspect, validate, apply, or let them sit.
 */

import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  GitBranch, Play, CheckCircle2, XCircle, Loader2, RefreshCw,
  FileCode, AlertTriangle, AlertCircle, Clock, ShieldAlert,
  FileDiff, FilePlus, FileX,
} from 'lucide-react';
import { CanvasHeader } from '@/components/layouts/canvas-header';
import { cn } from '@/lib/utils';
import {
  pipeline,
  type CodeChange,
  type CodeChangeStatus,
  type CodeFileChange,
  type CodeFileAction,
} from '@/lib/api';

const RISK_STYLE: Record<string, string> = {
  low:    'text-emerald-500 border-emerald-500/30 bg-emerald-500/10',
  medium: 'text-amber-400 border-amber-400/30 bg-amber-400/10',
  high:   'text-destructive border-destructive/30 bg-destructive/10',
};

const STATUS_STYLE: Record<string, { dot: string; label: string }> = {
  proposed:   { dot: 'bg-amber-400',   label: 'proposed' },
  validating: { dot: 'bg-primary animate-pulse', label: 'validating' },
  validated:  { dot: 'bg-primary',     label: 'validated' },
  applying:   { dot: 'bg-primary animate-pulse', label: 'applying' },
  applied:    { dot: 'bg-emerald-500', label: 'applied' },
  rejected:   { dot: 'bg-destructive', label: 'rejected' },
  failed:     { dot: 'bg-destructive', label: 'failed' },
};

const FILE_ACTION_ICON: Record<CodeFileAction, typeof FileCode> = {
  create: FilePlus,
  modify: FileDiff,
  delete: FileX,
};
const FILE_ACTION_TONE: Record<CodeFileAction, string> = {
  create: 'text-emerald-500',
  modify: 'text-amber-400',
  delete: 'text-destructive',
};

export default function PipelinePage() {
  const [all, setAll] = useState<CodeChange[]>([]);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState<string | null>(null);
  const [selectedId, setSelectedId] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    setLoading(true);
    setErr(null);
    try {
      // Fetch the union: recent changes + pending. Backend returns them
      // in separate buckets; we merge + de-dup by id so "pending" isn't
      // double-listed.
      const [changes, pending] = await Promise.all([
        pipeline.changes().catch(() => ({ changes: [] as CodeChange[] })),
        pipeline.pending().catch(() => ({ pending: [] as CodeChange[] })),
      ]);
      const seen = new Map<string, CodeChange>();
      for (const c of pending.pending) seen.set(c.id, c);
      for (const c of changes.changes) if (!seen.has(c.id)) seen.set(c.id, c);
      const merged = Array.from(seen.values()).sort((a, b) =>
        (b.created_at ?? '').localeCompare(a.created_at ?? ''),
      );
      setAll(merged);
      if (selectedId == null && merged.length > 0) {
        setSelectedId(merged[0]!.id);
      }
    } catch (e) {
      setErr(e instanceof Error ? e.message : 'Failed to load pipeline');
    } finally {
      setLoading(false);
    }
  }, [selectedId]);

  useEffect(() => { refresh(); }, []); // eslint-disable-line

  const selected = useMemo(
    () => all.find((c) => c.id === selectedId) ?? null,
    [all, selectedId],
  );

  const updateOne = (next: CodeChange) => {
    setAll((prev) => prev.map((c) => (c.id === next.id ? next : c)));
  };

  return (
    <div className="flex h-full min-h-0 flex-col gap-3 full-bleed p-4 lg:p-6">
      <CanvasHeader
        title="Pipeline"
        description="Agent-proposed code changes — validate, then apply."
        actions={
          <button
            onClick={refresh}
            disabled={loading}
            className="inline-flex items-center gap-1.5 rounded-md border border-border px-2.5 py-1.5 text-xs text-muted-foreground hover:bg-accent disabled:opacity-60"
          >
            {loading ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <RefreshCw className="h-3.5 w-3.5" />}
            Refresh
          </button>
        }
      />

      {err && (
        <div className="flex items-center gap-2 rounded-lg border border-destructive/40 bg-destructive/5 p-3 text-xs text-destructive">
          <AlertTriangle className="h-4 w-4" />
          <span>{err}</span>
        </div>
      )}

      <div className="grid min-h-0 flex-1 grid-cols-1 gap-3 lg:grid-cols-[280px_1fr]">
        {/* Rail */}
        <aside className="flex min-h-0 flex-col overflow-hidden rounded-xl border border-border bg-card/40">
          <header className="flex items-center gap-2 border-b border-border/60 px-3 py-2">
            <Clock className="h-3.5 w-3.5 text-muted-foreground" />
            <h2 className="text-xs font-semibold tracking-wider">PROPOSALS</h2>
            <span className="ml-auto font-mono text-2xs text-muted-foreground">{all.length}</span>
          </header>
          <div className="flex-1 overflow-y-auto">
            {all.length === 0 ? (
              <p className="p-3 text-2xs text-muted-foreground">
                No proposals. Agents create these via the pipeline tool.
              </p>
            ) : (
              <ul className="divide-y divide-border/60">
                {all.map((c) => (
                  <li key={c.id}>
                    <button
                      onClick={() => setSelectedId(c.id)}
                      className={cn(
                        'flex w-full flex-col gap-1 px-3 py-2 text-left text-2xs transition-colors hover:bg-accent/40',
                        selectedId === c.id && 'bg-primary/5',
                      )}
                    >
                      <div className="flex items-center gap-2">
                        <StatusDot status={c.status} />
                        <span className={cn('rounded-sm border px-1.5 text-xs font-mono uppercase', RISK_STYLE[c.risk] ?? RISK_STYLE.medium)}>
                          {c.risk}
                        </span>
                        <span className="ml-auto text-muted-foreground">
                          {c.files.length} file{c.files.length !== 1 ? 's' : ''}
                        </span>
                      </div>
                      <p className="line-clamp-2 font-medium text-foreground">
                        {c.description || '(no description)'}
                      </p>
                      <div className="text-muted-foreground/60">
                        {c.created_at ? new Date(c.created_at).toLocaleString() : ''}
                      </div>
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </div>
        </aside>

        {/* Detail */}
        <section className="flex min-h-0 flex-col overflow-hidden rounded-xl border border-border bg-card/40">
          {!selected ? (
            <div className="flex flex-1 items-center justify-center p-6 text-center">
              <p className="text-sm text-muted-foreground">Select a proposal to review.</p>
            </div>
          ) : (
            <ProposalDetail change={selected} onUpdate={updateOne} />
          )}
        </section>
      </div>
    </div>
  );
}

function StatusDot({ status }: { status: CodeChangeStatus }) {
  const s = STATUS_STYLE[status] ?? { dot: 'bg-muted', label: status };
  return (
    <span className="inline-flex items-center gap-1.5">
      <span className={cn('h-2 w-2 rounded-full', s.dot)} />
      <span className="font-mono lowercase text-muted-foreground">{s.label}</span>
    </span>
  );
}

function ProposalDetail({ change, onUpdate }: { change: CodeChange; onUpdate: (c: CodeChange) => void }) {
  const [busy, setBusy] = useState<'validate' | 'apply' | null>(null);
  const [err, setErr] = useState<string | null>(null);

  const doValidate = async () => {
    setBusy('validate');
    setErr(null);
    try {
      const next = await pipeline.validate(change.id);
      onUpdate(next);
    } catch (e) {
      setErr(e instanceof Error ? e.message : 'Validation failed');
    } finally {
      setBusy(null);
    }
  };
  const doApply = async () => {
    setBusy('apply');
    setErr(null);
    try {
      const next = await pipeline.apply(change.id);
      onUpdate(next);
    } catch (e) {
      setErr(e instanceof Error ? e.message : 'Apply failed');
    } finally {
      setBusy(null);
    }
  };

  const canValidate = change.status === 'proposed' || change.status === 'validated';
  const canApply = change.status === 'validated' && change.compile_ok !== false;

  return (
    <>
      {/* Header */}
      <header className="space-y-2 border-b border-border/60 p-4">
        <div className="flex items-center gap-2">
          <StatusDot status={change.status} />
          <span
            className={cn(
              'rounded-md border px-1.5 py-0.5 text-xs font-mono uppercase',
              RISK_STYLE[change.risk] ?? RISK_STYLE.medium,
            )}
          >
            {change.risk} risk
          </span>
          <span className="ml-auto flex items-center gap-2 font-mono text-2xs text-muted-foreground">
            {change.proposed_by && <span>by {change.proposed_by}</span>}
            <span>·</span>
            <span>{change.created_at ? new Date(change.created_at).toLocaleString() : ''}</span>
          </span>
        </div>
        <h3 className="text-base font-semibold leading-snug">{change.description || '(no description)'}</h3>

        {/* Validation summary */}
        <ValidationStrip change={change} />

        {/* Actions */}
        <div className="flex items-center gap-2 pt-1">
          <button
            onClick={doValidate}
            disabled={!canValidate || !!busy}
            className="inline-flex items-center gap-1.5 rounded-md border border-border bg-card px-3 py-1 text-xs font-medium hover:bg-accent disabled:cursor-not-allowed disabled:opacity-50"
          >
            {busy === 'validate' ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <ShieldAlert className="h-3.5 w-3.5" />}
            Validate
          </button>
          <button
            onClick={doApply}
            disabled={!canApply || !!busy}
            className="inline-flex items-center gap-1.5 rounded-md bg-primary px-3 py-1 text-xs font-medium text-primary-foreground hover:bg-primary/90 disabled:cursor-not-allowed disabled:opacity-50"
          >
            {busy === 'apply' ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Play className="h-3.5 w-3.5" />}
            Apply
          </button>
          {!canApply && change.status === 'validated' && change.compile_ok === false && (
            <span className="text-2xs text-destructive">Compile failed — fix first</span>
          )}
          {change.status === 'applied' && change.applied_at && (
            <span className="ml-auto text-2xs text-emerald-500">
              Applied {new Date(change.applied_at).toLocaleString()}
            </span>
          )}
        </div>

        {err && <p className="text-2xs text-destructive">{err}</p>}
      </header>

      {/* File diffs */}
      <div className="flex-1 overflow-y-auto p-4">
        {change.files.length === 0 ? (
          <p className="text-xs text-muted-foreground">No file changes.</p>
        ) : (
          <ul className="space-y-3">
            {change.files.map((f, i) => (
              <FileDiffBlock key={`${f.path}-${i}`} file={f} />
            ))}
          </ul>
        )}
      </div>
    </>
  );
}

function ValidationStrip({ change }: { change: CodeChange }) {
  if (change.compile_ok == null && change.test_ok == null) return null;
  return (
    <div className="flex flex-wrap items-center gap-2 text-2xs">
      {change.compile_ok != null && (
        <span
          className={cn(
            'inline-flex items-center gap-1 rounded-md border px-2 py-0.5 font-mono',
            change.compile_ok
              ? 'border-emerald-500/30 bg-emerald-500/10 text-emerald-500'
              : 'border-destructive/30 bg-destructive/10 text-destructive',
          )}
        >
          {change.compile_ok ? <CheckCircle2 className="h-3 w-3" /> : <XCircle className="h-3 w-3" />}
          compile {change.compile_ok ? 'ok' : 'fail'}
        </span>
      )}
      {change.test_ok != null && (
        <span
          className={cn(
            'inline-flex items-center gap-1 rounded-md border px-2 py-0.5 font-mono',
            change.test_ok
              ? 'border-emerald-500/30 bg-emerald-500/10 text-emerald-500'
              : 'border-destructive/30 bg-destructive/10 text-destructive',
          )}
        >
          {change.test_ok ? <CheckCircle2 className="h-3 w-3" /> : <XCircle className="h-3 w-3" />}
          tests {change.test_ok ? 'pass' : 'fail'}
          {change.tests_passed != null && <span>· {change.tests_passed}✓</span>}
          {change.tests_failed ? <span className="text-destructive">{change.tests_failed}✗</span> : null}
        </span>
      )}
      {(change.compile_error || change.test_error) && (
        <details className="w-full">
          <summary className="cursor-pointer text-muted-foreground hover:text-foreground">error details</summary>
          <pre className="mt-1 whitespace-pre-wrap rounded-md border border-border/60 bg-background/50 p-2 font-mono text-xs text-destructive/90">
            {change.compile_error}
            {change.compile_error && change.test_error ? '\n\n' : ''}
            {change.test_error}
          </pre>
        </details>
      )}
    </div>
  );
}

function FileDiffBlock({ file }: { file: CodeFileChange }) {
  const Icon = FILE_ACTION_ICON[file.action] ?? FileCode;
  const tone = FILE_ACTION_TONE[file.action] ?? 'text-muted-foreground';
  return (
    <li className="rounded-lg border border-border bg-card">
      <header className="flex items-center gap-2 border-b border-border/60 px-3 py-1.5 text-2xs">
        <Icon className={cn('h-3.5 w-3.5', tone)} />
        <span className="font-mono font-medium">{file.path}</span>
        <span className={cn('ml-auto rounded-sm px-1.5 py-0.5 font-mono uppercase text-xs', tone, 'bg-muted/40')}>
          {file.action}
        </span>
      </header>
      {file.action === 'delete' ? (
        <div className="px-3 py-2 text-2xs text-muted-foreground">File will be deleted.</div>
      ) : file.action === 'create' ? (
        <pre className="max-h-72 overflow-auto whitespace-pre-wrap break-words px-3 py-2 font-mono text-2xs text-foreground/85">
          {file.new_content}
        </pre>
      ) : (
        <SideBySideDiff oldContent={file.old_content ?? ''} newContent={file.new_content} />
      )}
    </li>
  );
}

function SideBySideDiff({ oldContent, newContent }: { oldContent: string; newContent: string }) {
  // Lightweight diff: compute a per-line LCS-ish alignment. Good enough
  // for review; not Myers. Keeps the bundle lean — no external diff
  // lib imported for a feature only this page uses.
  const oldLines = oldContent.split('\n');
  const newLines = newContent.split('\n');
  const rows = alignLines(oldLines, newLines);

  return (
    <div className="grid max-h-96 grid-cols-2 divide-x divide-border/60 overflow-auto">
      <pre className="px-3 py-2 font-mono text-xs leading-relaxed">
        {rows.map((r, i) => (
          <div key={i} className={cn(
            r.oldIdx < 0 ? 'bg-transparent'
              : r.removed ? 'bg-destructive/15 text-destructive'
              : 'text-foreground/80',
          )}>
            {r.oldIdx >= 0 ? (
              <><span className="mr-2 inline-block w-8 text-right text-muted-foreground">{r.oldIdx + 1}</span>{r.oldLine}</>
            ) : (
              <>&nbsp;</>
            )}
          </div>
        ))}
      </pre>
      <pre className="px-3 py-2 font-mono text-xs leading-relaxed">
        {rows.map((r, i) => (
          <div key={i} className={cn(
            r.newIdx < 0 ? 'bg-transparent'
              : r.added ? 'bg-emerald-500/15 text-emerald-500'
              : 'text-foreground/80',
          )}>
            {r.newIdx >= 0 ? (
              <><span className="mr-2 inline-block w-8 text-right text-muted-foreground">{r.newIdx + 1}</span>{r.newLine}</>
            ) : (
              <>&nbsp;</>
            )}
          </div>
        ))}
      </pre>
    </div>
  );
}

type DiffRow = {
  oldIdx: number;
  newIdx: number;
  oldLine: string;
  newLine: string;
  added: boolean;
  removed: boolean;
};

/** Trivial line-alignment: walk both sequences, match on equality,
 *  emit "remove" / "add" rows otherwise. Not optimal but fine for
 *  typical agent-generated diffs under a few hundred lines. */
function alignLines(oldLines: string[], newLines: string[]): DiffRow[] {
  const rows: DiffRow[] = [];
  let i = 0, j = 0;
  while (i < oldLines.length || j < newLines.length) {
    if (i < oldLines.length && j < newLines.length && oldLines[i] === newLines[j]) {
      rows.push({ oldIdx: i, newIdx: j, oldLine: oldLines[i]!, newLine: newLines[j]!, added: false, removed: false });
      i++; j++;
      continue;
    }
    // Look ahead up to 10 lines for a resync.
    const match = findResync(oldLines, newLines, i, j, 10);
    if (match) {
      while (i < match.i) { rows.push({ oldIdx: i, newIdx: -1, oldLine: oldLines[i]!, newLine: '', added: false, removed: true }); i++; }
      while (j < match.j) { rows.push({ oldIdx: -1, newIdx: j, oldLine: '', newLine: newLines[j]!, added: true, removed: false }); j++; }
      continue;
    }
    if (i < oldLines.length) { rows.push({ oldIdx: i, newIdx: -1, oldLine: oldLines[i]!, newLine: '', added: false, removed: true }); i++; }
    if (j < newLines.length) { rows.push({ oldIdx: -1, newIdx: j, oldLine: '', newLine: newLines[j]!, added: true, removed: false }); j++; }
  }
  return rows;
}

function findResync(a: string[], b: string[], ia: number, ib: number, window: number): { i: number; j: number } | null {
  for (let da = 0; da < window && ia + da < a.length; da++) {
    for (let db = 0; db < window && ib + db < b.length; db++) {
      if (a[ia + da] === b[ib + db]) return { i: ia + da, j: ib + db };
    }
  }
  return null;
}
