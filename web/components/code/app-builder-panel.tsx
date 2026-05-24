'use client';
// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useCallback, useEffect, useState } from 'react';
import {
  CheckCircle2,
  ChevronRight,
  Loader2,
  Package,
  Play,
  RefreshCw,
  Terminal,
  Wrench,
  XCircle,
} from 'lucide-react';
import { cn } from '@/lib/utils';
import { getToken, BASE as API_BASE } from '@/lib/api-core';

interface AppInfo {
  id: string;
  slug: string;
  display_name: string;
  version: string;
  description: string;
  enabled: boolean;
  installed_at: string;
  install_path: string;
}

interface ToolResult {
  name: string;
  output: string;
  error?: string;
  elapsed_ms?: number;
}

interface AppBuilderPanelProps {
  projectId: string;
  projectPath: string;
  className?: string;
}

async function apiFetch(path: string, options?: RequestInit) {
  const res = await fetch(`${API_BASE}${path}`, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${getToken()}`,
      ...options?.headers,
    },
  });
  return res;
}

export function AppBuilderPanel({ projectId, projectPath, className }: AppBuilderPanelProps) {
  const [app, setApp] = useState<AppInfo | null>(null);
  const [loading, setLoading] = useState(true);
  const [installing, setInstalling] = useState(false);
  const [reloading, setReloading] = useState(false);
  const [error, setError] = useState('');
  const [toolResults, setToolResults] = useState<ToolResult[]>([]);
  const [runningTool, setRunningTool] = useState('');
  const [migrationStatus, setMigrationStatus] = useState<'unknown' | 'ok' | 'error'>('unknown');

  const loadApp = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      const res = await apiFetch('/apps/');
      if (!res.ok) { setLoading(false); return; }
      const data = await res.json() as { apps: AppInfo[] };
      // Match by install_path prefix
      const found = data.apps?.find(a => a.install_path && projectPath && a.install_path.startsWith(projectPath));
      setApp(found ?? null);
    } catch {
      setError('Failed to load app status');
    } finally {
      setLoading(false);
    }
  }, [projectPath]);

  useEffect(() => { loadApp(); }, [loadApp]);

  const install = async () => {
    setInstalling(true);
    setError('');
    try {
      const res = await apiFetch('/apps/', {
        method: 'POST',
        body: JSON.stringify({ path: projectPath }),
      });
      if (!res.ok) {
        const d = await res.json().catch(() => ({}));
        setError((d as any).error || `Install failed (${res.status})`);
        return;
      }
      const created = await res.json() as AppInfo;
      setApp(created);
      setMigrationStatus('ok');
    } catch (e: any) {
      setError(e?.message || 'Install failed');
    } finally {
      setInstalling(false);
    }
  };

  const reload = async () => {
    if (!app) return;
    setReloading(true);
    setError('');
    try {
      const res = await apiFetch(`/apps/${app.id}/reload`, { method: 'POST' });
      if (!res.ok) {
        const d = await res.json().catch(() => ({}));
        setError((d as any).error || `Reload failed (${res.status})`);
        return;
      }
      const updated = await res.json() as AppInfo;
      setApp(updated);
    } catch (e: any) {
      setError(e?.message || 'Reload failed');
    } finally {
      setReloading(false);
    }
  };

  const runTool = async (toolName: string, args: Record<string, any> = {}) => {
    if (!app) return;
    setRunningTool(toolName);
    const start = Date.now();
    try {
      const res = await apiFetch(`/apps/${app.slug}/tools/${toolName}`, {
        method: 'POST',
        body: JSON.stringify({ args }),
      });
      const d = await res.json().catch(() => ({ output: 'No output' }));
      const elapsed_ms = Date.now() - start;
      setToolResults(prev => [
        { name: toolName, output: typeof d === 'string' ? d : JSON.stringify(d, null, 2), elapsed_ms },
        ...prev.slice(0, 9),
      ]);
    } catch (e: any) {
      setToolResults(prev => [
        { name: toolName, output: '', error: e?.message || 'Tool call failed', elapsed_ms: Date.now() - start },
        ...prev.slice(0, 9),
      ]);
    } finally {
      setRunningTool('');
    }
  };

  if (loading) {
    return (
      <div className={cn('flex h-full items-center justify-center', className)}>
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
      </div>
    );
  }

  return (
    <div className={cn('flex h-full flex-col overflow-y-auto', className)}>
      <div className="space-y-4 p-4">

        {/* Header */}
        <div className="flex items-center gap-3">
          <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-primary/10">
            <Package className="h-5 w-5 text-primary" />
          </div>
          <div className="flex-1 min-w-0">
            <p className="text-sm font-semibold">{app?.display_name ?? 'Qorven App'}</p>
            <p className="text-xs text-muted-foreground truncate">{projectPath}</p>
          </div>
          {app && (
            <span className={cn(
              'flex items-center gap-1 rounded-full px-2 py-0.5 text-2xs font-medium',
              app.enabled
                ? 'bg-emerald-500/10 text-emerald-500'
                : 'bg-muted text-muted-foreground',
            )}>
              {app.enabled
                ? <><CheckCircle2 className="h-2.5 w-2.5" />Installed</>
                : <><XCircle className="h-2.5 w-2.5" />Disabled</>
              }
            </span>
          )}
        </div>

        {error && (
          <div className="rounded-lg border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive">
            {error}
          </div>
        )}

        {/* Install / Reload */}
        <div className="flex gap-2">
          {!app ? (
            <button
              type="button"
              onClick={install}
              disabled={installing}
              className="flex flex-1 items-center justify-center gap-2 rounded-lg bg-primary px-3 py-2 text-xs font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50 transition-colors"
            >
              {installing
                ? <Loader2 className="h-3.5 w-3.5 animate-spin" />
                : <Play className="h-3.5 w-3.5" />
              }
              {installing ? 'Installing…' : 'Install App'}
            </button>
          ) : (
            <>
              <button
                type="button"
                onClick={reload}
                disabled={reloading}
                className="flex flex-1 items-center justify-center gap-2 rounded-lg border border-border bg-background px-3 py-2 text-xs font-medium text-foreground hover:bg-muted/40 disabled:opacity-50 transition-colors"
              >
                {reloading
                  ? <Loader2 className="h-3.5 w-3.5 animate-spin" />
                  : <RefreshCw className="h-3.5 w-3.5" />
                }
                {reloading ? 'Reloading…' : 'Reload'}
              </button>
              <button
                type="button"
                onClick={loadApp}
                className="flex items-center justify-center gap-1.5 rounded-lg border border-border bg-background px-3 py-2 text-xs text-muted-foreground hover:bg-muted/40 transition-colors"
              >
                <RefreshCw className="h-3.5 w-3.5" />
              </button>
            </>
          )}
        </div>

        {/* App details */}
        {app && (
          <>
            <div className="rounded-lg border border-border bg-muted/20 divide-y divide-border">
              <Row label="Slug" value={app.slug} mono />
              <Row label="Version" value={app.version || '—'} />
              <Row label="Installed" value={new Date(app.installed_at).toLocaleString()} />
              <Row label="Migrations" value={
                migrationStatus === 'ok' ? 'Applied' :
                migrationStatus === 'error' ? 'Error' : 'Unknown'
              } valueClass={
                migrationStatus === 'ok' ? 'text-emerald-500' :
                migrationStatus === 'error' ? 'text-destructive' : ''
              } />
            </div>

            {app.description && (
              <p className="text-xs text-muted-foreground">{app.description}</p>
            )}
          </>
        )}

        {/* Tool test panel */}
        {app && (
          <div className="space-y-2">
            <p className="text-xs font-semibold text-muted-foreground uppercase tracking-wider">Test Tools</p>
            <ToolTestRow
              slug={app.slug}
              running={runningTool}
              onRun={runTool}
            />
            {toolResults.length > 0 && (
              <div className="space-y-2 pt-1">
                {toolResults.map((r, i) => (
                  <ToolResultCard key={i} result={r} />
                ))}
              </div>
            )}
          </div>
        )}

      </div>
    </div>
  );
}

function Row({ label, value, mono, valueClass }: {
  label: string; value: string; mono?: boolean; valueClass?: string;
}) {
  return (
    <div className="flex items-center justify-between px-3 py-2">
      <span className="text-xs text-muted-foreground">{label}</span>
      <span className={cn('text-xs text-right truncate max-w-[60%]', mono && 'font-mono', valueClass)}>
        {value}
      </span>
    </div>
  );
}

function ToolTestRow({ slug, running, onRun }: {
  slug: string;
  running: string;
  onRun: (name: string, args?: Record<string, any>) => void;
}) {
  const [toolName, setToolName] = useState('');
  const [argsText, setArgsText] = useState('{}');
  const [argsError, setArgsError] = useState('');

  const handleRun = () => {
    if (!toolName.trim()) return;
    try {
      const args = JSON.parse(argsText);
      setArgsError('');
      onRun(toolName.trim(), args);
    } catch {
      setArgsError('Args must be valid JSON');
    }
  };

  return (
    <div className="rounded-lg border border-border bg-background space-y-2 p-3">
      <div className="flex gap-2">
        <input
          type="text"
          value={toolName}
          onChange={e => setToolName(e.target.value)}
          placeholder="tool name"
          className="flex-1 rounded-md border border-border bg-muted/30 px-2 py-1.5 font-mono text-xs focus:outline-none focus:ring-1 focus:ring-primary"
        />
        <button
          type="button"
          onClick={handleRun}
          disabled={!toolName.trim() || !!running}
          className="flex items-center gap-1.5 rounded-md bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50 transition-colors"
        >
          {running === toolName
            ? <Loader2 className="h-3 w-3 animate-spin" />
            : <ChevronRight className="h-3 w-3" />
          }
          Run
        </button>
      </div>
      <textarea
        value={argsText}
        onChange={e => { setArgsText(e.target.value); setArgsError(''); }}
        rows={3}
        placeholder='{"key": "value"}'
        className="w-full resize-none rounded-md border border-border bg-muted/30 px-2 py-1.5 font-mono text-xs focus:outline-none focus:ring-1 focus:ring-primary"
      />
      {argsError && (
        <p className="text-2xs text-destructive">{argsError}</p>
      )}
    </div>
  );
}

function ToolResultCard({ result }: { result: ToolResult }) {
  const [expanded, setExpanded] = useState(false);
  const hasOutput = !!result.output || !!result.error;
  const preview = (result.error || result.output).slice(0, 80);

  return (
    <div className={cn(
      'rounded-lg border bg-muted/20 text-xs overflow-hidden',
      result.error ? 'border-destructive/30' : 'border-border',
    )}>
      <button
        type="button"
        onClick={() => setExpanded(v => !v)}
        className="flex w-full items-center gap-2 px-3 py-2 text-left hover:bg-muted/30 transition-colors"
      >
        {result.error
          ? <XCircle className="h-3 w-3 shrink-0 text-destructive" />
          : <Terminal className="h-3 w-3 shrink-0 text-emerald-500" />
        }
        <span className="flex-1 font-mono truncate">{result.name}</span>
        {result.elapsed_ms !== undefined && (
          <span className="text-2xs text-muted-foreground">{result.elapsed_ms}ms</span>
        )}
        <Wrench className="h-3 w-3 text-muted-foreground/40" />
      </button>
      {expanded && hasOutput && (
        <pre className={cn(
          'border-t border-border px-3 py-2 overflow-x-auto text-2xs whitespace-pre-wrap',
          result.error ? 'text-destructive' : 'text-foreground',
        )}>
          {result.error || result.output}
        </pre>
      )}
    </div>
  );
}
