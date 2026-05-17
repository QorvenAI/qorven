'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

/**
 * /sandbox — isolated code execution (T2.3).
 *
 * The backend sandbox binds every run to an agent_id — executions
 * happen inside that agent's workspace + under its tool profile. We
 * default to Prime, fall back to the first available agent.
 *
 * Layout: history rail on the left, editor + output stacked in the
 * middle, artifacts rail on the right.
 */

import { useEffect, useMemo, useRef, useState } from 'react';
import {
  Play, Loader2, Copy, Check, Terminal, Clock, FileDown,
  AlertCircle, RotateCw, ChevronRight,
} from 'lucide-react';
import { cn } from '@/lib/utils';
import { sandbox, type SandboxRun, type SandboxArtifact } from '@/lib/api';
import { useStore } from '@/store';

// Kept conservative — each snippet prints a greeting; nothing that
// needs network or writes to disk, so a curious user poking buttons
// never triggers a destructive gate on first try.
const LANG_DEFAULTS: Record<string, string> = {
  python: 'print("Hello from Qorven sandbox!")',
  javascript: 'console.log("Hello from Qorven sandbox!");',
  typescript: 'console.log("Hello from Qorven sandbox!");',
  bash: 'echo "Hello from Qorven sandbox!"',
  go: [
    'package main',
    '',
    'import "fmt"',
    '',
    'func main() {',
    '\tfmt.Println("Hello from Qorven sandbox!")',
    '}',
  ].join('\n'),
};

const LANG_LABEL: Record<string, string> = {
  python: 'Python',
  javascript: 'JavaScript',
  typescript: 'TypeScript',
  bash: 'Bash',
  go: 'Go',
};

type OutputKind = 'stdout' | 'stderr' | 'idle';

export default function SandboxPage() {
  const souls = useStore((s) => s.souls);
  const prime = souls.find((s) => s.agent_key === 'prime' || s.role === 'supervisor');
  const [agentId, setAgentId] = useState<string>('');
  const [lang, setLang] = useState<string>('python');
  const [code, setCode] = useState<string>(LANG_DEFAULTS.python ?? '');
  const [running, setRunning] = useState(false);
  const [output, setOutput] = useState('');
  const [outputKind, setOutputKind] = useState<OutputKind>('idle');
  const [lastRun, setLastRun] = useState<SandboxRun | null>(null);
  const [error, setError] = useState<string | null>(null);

  const [runs, setRuns] = useState<SandboxRun[]>([]);
  const [artifacts, setArtifacts] = useState<SandboxArtifact[]>([]);
  const [copied, setCopied] = useState(false);

  const editorRef = useRef<HTMLTextAreaElement>(null);

  // Default agent = Prime when souls load.
  useEffect(() => {
    if (!agentId && prime) setAgentId(prime.id);
    else if (!agentId && souls.length > 0) setAgentId(souls[0]!.id);
  }, [agentId, prime, souls]);

  const refreshSidebars = (id: string) => {
    if (!id) return;
    // Backend encodes empty slices as JSON `null` (Go default), so we
    // must coerce here — callers downstream spread/map on these arrays.
    sandbox.runs(id)
      .then((d) => setRuns(Array.isArray(d) ? d : []))
      .catch(() => setRuns([]));
    sandbox.artifacts(id)
      .then((d) => setArtifacts(Array.isArray(d) ? d : []))
      .catch(() => setArtifacts([]));
  };

  useEffect(() => {
    if (agentId) refreshSidebars(agentId);
  }, [agentId]);

  const run = async () => {
    if (!agentId || !code.trim()) return;
    setRunning(true);
    setError(null);
    setOutput('');
    setOutputKind('idle');
    setLastRun(null);

    try {
      // The backend accepts both `command` (direct bash) and
      // `code`+`language` (writes to a temp file + runs the matching
      // interpreter). We always go through code+language so the
      // editor is the single source of truth.
      const res = await sandbox.run({
        agent_id: agentId,
        command: '',
        code,
        language: lang,
      });
      setLastRun(res);
      setOutput(res.output || '(no output)');
      setOutputKind(res.exit_code === 0 ? 'stdout' : 'stderr');
      refreshSidebars(agentId);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Sandbox run failed');
    } finally {
      setRunning(false);
    }
  };

  const changeLang = (next: string) => {
    setLang(next);
    // Only overwrite the editor when it's still a canned default;
    // never clobber the user's own work.
    if (code.trim() === '' || Object.values(LANG_DEFAULTS).includes(code)) {
      setCode(LANG_DEFAULTS[next] ?? '');
    }
  };

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(output);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      // Clipboard blocked (non-secure context, permissions) — no toast
      // infra so we silently fail. Users can still select + copy.
    }
  };

  const loadHistoryRun = (r: SandboxRun) => {
    setLang(r.language ?? 'python');
    setCode(r.code ?? r.command ?? '');
    setOutput(r.output ?? '');
    setOutputKind(r.exit_code === 0 ? 'stdout' : 'stderr');
    setLastRun(r);
    setError(null);
    editorRef.current?.focus();
  };

  return (
    <div className="flex h-full min-h-0 flex-col gap-3 full-bleed p-4 lg:p-6">
      {/* Header */}
      <header className="flex flex-wrap items-center gap-3">
        <Terminal className="h-6 w-6 text-primary" />
        <h1 className="text-lg font-semibold">Sandbox</h1>
        <p className="text-xs text-muted-foreground">
          Run code inside an agent&apos;s workspace — isolated, tool-profile-gated.
        </p>
        <div className="ml-auto flex items-center gap-2">
          <label className="text-2xs text-muted-foreground">Agent</label>
          <select
            value={agentId}
            onChange={(e) => setAgentId(e.target.value)}
            className="qr-input text-xs h-7 py-0"
          >
            <option value="" disabled>Choose…</option>
            {souls.map((s) => (
              <option key={s.id} value={s.id}>
                {s.display_name || s.agent_key}
              </option>
            ))}
          </select>
        </div>
      </header>

      {/* 3-column layout */}
      <div className="grid min-h-0 flex-1 grid-cols-1 gap-3 lg:grid-cols-[240px_1fr_240px]">
        <HistoryRail
          runs={runs}
          activeId={lastRun?.id}
          onPick={loadHistoryRun}
          onRefresh={() => refreshSidebars(agentId)}
        />

        <div className="flex min-h-0 flex-col gap-2">
          <div className="flex items-center gap-1.5 rounded-md border border-border bg-card px-2 py-1.5">
            <div className="flex items-center gap-0.5">
              {Object.keys(LANG_DEFAULTS).map((l) => (
                <button
                  key={l}
                  onClick={() => changeLang(l)}
                  className={cn(
                    'rounded-md px-2 py-1 text-2xs font-medium transition-colors',
                    lang === l
                      ? 'bg-primary/15 text-primary'
                      : 'text-muted-foreground hover:bg-accent hover:text-foreground',
                  )}
                >
                  {LANG_LABEL[l] ?? l}
                </button>
              ))}
            </div>
            <div className="mx-1 h-4 w-px bg-border" />
            <button
              onClick={run}
              disabled={running || !agentId || !code.trim()}
              className={cn(
                'ml-auto inline-flex items-center gap-1.5 rounded-md px-3 py-1 text-xs font-medium transition-colors',
                'bg-primary text-primary-foreground hover:bg-primary/90',
                'disabled:cursor-not-allowed disabled:opacity-50',
              )}
            >
              {running ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Play className="h-3.5 w-3.5" />}
              {running ? 'Running' : 'Run'}
            </button>
          </div>

          <textarea
            ref={editorRef}
            value={code}
            onChange={(e) => setCode(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
                e.preventDefault();
                run();
              }
            }}
            spellCheck={false}
            className={cn(
              'min-h-[180px] flex-1 rounded-md border border-border bg-background p-3',
              'font-mono text-xs leading-relaxed outline-none placeholder:text-muted-foreground/50 focus:border-primary',
            )}
            placeholder={`Write ${LANG_LABEL[lang] ?? lang} here…`}
          />

          <OutputPanel
            running={running}
            error={error}
            output={output}
            kind={outputKind}
            lastRun={lastRun}
            onCopy={copy}
            copied={copied}
          />
        </div>

        <ArtifactsRail artifacts={artifacts} />
      </div>
    </div>
  );
}

function HistoryRail({
  runs,
  activeId,
  onPick,
  onRefresh,
}: {
  runs: SandboxRun[];
  activeId?: string;
  onPick: (r: SandboxRun) => void;
  onRefresh: () => void;
}) {
  return (
    <aside className="flex min-h-0 flex-col rounded-xl border border-border bg-card/40">
      <header className="flex items-center gap-2 border-b border-border/60 px-3 py-2">
        <Clock className="h-3.5 w-3.5 text-muted-foreground" />
        <h2 className="text-xs font-semibold tracking-wider">HISTORY</h2>
        <button
          onClick={onRefresh}
          title="Refresh"
          className="ml-auto flex h-5 w-5 items-center justify-center rounded-sm text-muted-foreground hover:bg-accent hover:text-foreground"
        >
          <RotateCw className="h-3 w-3" />
        </button>
      </header>
      <div className="flex-1 overflow-y-auto">
        {runs.length === 0 ? (
          <p className="p-3 text-2xs text-muted-foreground">
            No runs yet for this agent.
          </p>
        ) : (
          <ul className="divide-y divide-border/60">
            {runs.map((r) => (
              <li key={r.id}>
                <button
                  onClick={() => onPick(r)}
                  className={cn(
                    'flex w-full flex-col gap-1 px-3 py-2 text-left text-2xs transition-colors hover:bg-accent/40',
                    activeId === r.id && 'bg-primary/5',
                  )}
                >
                  <div className="flex items-center gap-1.5">
                    <span
                      className={cn(
                        'inline-block h-1.5 w-1.5 rounded-full',
                        r.exit_code === 0 ? 'bg-emerald-500' : 'bg-destructive',
                      )}
                    />
                    <span className="font-mono font-medium">{r.language ?? 'shell'}</span>
                    <span className="ml-auto text-muted-foreground">
                      {Math.round((r.duration_ms ?? 0) / 1000 * 10) / 10}s
                    </span>
                  </div>
                  <div className="truncate font-mono text-muted-foreground">
                    {((r.code ?? r.command ?? '').split('\n')[0] ?? '').slice(0, 40) || '(empty)'}
                  </div>
                  <div className="text-muted-foreground/60">
                    {new Date(r.created_at).toLocaleTimeString()}
                  </div>
                </button>
              </li>
            ))}
          </ul>
        )}
      </div>
    </aside>
  );
}

function OutputPanel({
  running,
  error,
  output,
  kind,
  lastRun,
  onCopy,
  copied,
}: {
  running: boolean;
  error: string | null;
  output: string;
  kind: OutputKind;
  lastRun: SandboxRun | null;
  onCopy: () => void;
  copied: boolean;
}) {
  const hasContent = !!output || !!error;
  return (
    <section className="flex min-h-[160px] flex-col rounded-md border border-border bg-black/60">
      <header className="flex items-center gap-2 border-b border-border/60 px-3 py-1.5 text-2xs text-muted-foreground">
        <ChevronRight className="h-3 w-3" />
        <span className="font-mono uppercase tracking-wider">
          {kind === 'stderr' ? 'stderr' : kind === 'stdout' ? 'stdout' : 'output'}
        </span>
        {lastRun && (
          <span className="font-mono">
            · exit {lastRun.exit_code} · {(lastRun.duration_ms / 1000).toFixed(2)}s
          </span>
        )}
        <button
          onClick={onCopy}
          disabled={!hasContent || running}
          className="ml-auto flex items-center gap-1 rounded-sm px-1.5 py-0.5 text-xs hover:bg-accent disabled:opacity-40"
        >
          {copied ? <Check className="h-3 w-3 text-emerald-500" /> : <Copy className="h-3 w-3" />}
          {copied ? 'copied' : 'copy'}
        </button>
      </header>
      <pre
        className={cn(
          'flex-1 overflow-auto whitespace-pre-wrap break-words p-3 font-mono text-2xs leading-relaxed',
          kind === 'stderr' ? 'text-destructive/90' : 'text-foreground/85',
        )}
      >
        {running ? (
          <span className="flex items-center gap-2 text-muted-foreground">
            <Loader2 className="h-3 w-3 animate-spin" />
            Running…
          </span>
        ) : error ? (
          <span className="flex items-start gap-2 text-destructive">
            <AlertCircle className="mt-0.5 h-3 w-3 shrink-0" />
            <span>{error}</span>
          </span>
        ) : output ? (
          output
        ) : (
          <span className="text-muted-foreground/60">
            Click Run to execute. Output (stdout + stderr) appears here, capped at 50 KB server-side.
          </span>
        )}
      </pre>
    </section>
  );
}

function ArtifactsRail({ artifacts }: { artifacts: SandboxArtifact[] }) {
  const sorted = useMemo(
    () => [...artifacts].sort((a, b) => (b.modified ?? '').localeCompare(a.modified ?? '')),
    [artifacts],
  );
  return (
    <aside className="flex min-h-0 flex-col rounded-xl border border-border bg-card/40">
      <header className="flex items-center gap-2 border-b border-border/60 px-3 py-2">
        <FileDown className="h-3.5 w-3.5 text-muted-foreground" />
        <h2 className="text-xs font-semibold tracking-wider">ARTIFACTS</h2>
        <span className="ml-auto text-2xs text-muted-foreground">{sorted.length}</span>
      </header>
      <div className="flex-1 overflow-y-auto">
        {sorted.length === 0 ? (
          <p className="p-3 text-2xs text-muted-foreground">
            No files written. Artifacts appear when a run writes to the workspace.
          </p>
        ) : (
          <ul className="divide-y divide-border/60">
            {sorted.map((a) => (
              <li key={a.path} className="px-3 py-2 text-2xs">
                <div className="flex items-center justify-between gap-2">
                  <span className="truncate font-mono font-medium" title={a.name}>
                    {a.name}
                  </span>
                  <span className="shrink-0 text-muted-foreground">{formatBytes(a.size)}</span>
                </div>
                <div className="truncate font-mono text-muted-foreground/70" title={a.path}>
                  {a.path}
                </div>
                <div className="text-muted-foreground/60">
                  {a.modified ? new Date(a.modified).toLocaleString() : ''}
                </div>
              </li>
            ))}
          </ul>
        )}
      </div>
    </aside>
  );
}

function formatBytes(b: number) {
  if (b < 1024) return `${b} B`;
  if (b < 1024 * 1024) return `${(b / 1024).toFixed(1)} KB`;
  return `${(b / 1024 / 1024).toFixed(1)} MB`;
}
