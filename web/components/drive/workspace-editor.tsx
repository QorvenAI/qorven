'use client';
// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useRef, useState } from 'react';
import { History } from 'lucide-react';
import { cn } from '@/lib/utils';
import { driveApi, type WorkspaceFileMeta, type ContextFileVersion } from '@/lib/api-workspace';

function formatTs(iso: string): string {
  try {
    const d = new Date(iso);
    return d.toLocaleString(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
  } catch {
    return iso;
  }
}

export function WorkspaceEditor({ agentId }: { agentId: string }) {
  const [files, setFiles] = useState<WorkspaceFileMeta[]>([]);
  const [active, setActive] = useState<string | null>(null);
  const [content, setContent] = useState('');
  const [dirty, setDirty] = useState(false);
  const [saving, setSaving] = useState(false);

  const [versions, setVersions] = useState<ContextFileVersion[]>([]);
  const [historyOpen, setHistoryOpen] = useState(false);
  const [historyLoading, setHistoryLoading] = useState(false);
  const [restoring, setRestoring] = useState<string | null>(null);
  const panelRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    driveApi.workspaceFiles(agentId).then((r) => {
      setFiles(r.files ?? []);
      if (r.files?.length) setActive(r.files[0]!.name);
    }).catch(() => setFiles([]));
  }, [agentId]);

  useEffect(() => {
    if (!active) return;
    driveApi.workspaceGet(agentId, active).then((r) => { setContent(r.content); setDirty(false); }).catch(() => setContent(''));
    // Close history panel when switching files
    setHistoryOpen(false);
    setVersions([]);
  }, [agentId, active]);

  // Close panel on outside click
  useEffect(() => {
    if (!historyOpen) return;
    const handler = (e: MouseEvent) => {
      if (panelRef.current && !panelRef.current.contains(e.target as Node)) {
        setHistoryOpen(false);
      }
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, [historyOpen]);

  const save = async () => {
    if (!active) return;
    setSaving(true);
    try { await driveApi.workspacePut(agentId, active, content); setDirty(false); } finally { setSaving(false); }
  };

  const toggleHistory = async () => {
    if (historyOpen) {
      setHistoryOpen(false);
      return;
    }
    if (!active) return;
    setHistoryOpen(true);
    setHistoryLoading(true);
    try {
      const r = await driveApi.workspaceVersions(agentId, active);
      setVersions(r.versions ?? []);
    } catch {
      setVersions([]);
    } finally {
      setHistoryLoading(false);
    }
  };

  const restore = async (v: ContextFileVersion) => {
    if (!active) return;
    setRestoring(v.id);
    try {
      await driveApi.workspaceRestore(agentId, v.id);
      const r = await driveApi.workspaceGet(agentId, active);
      setContent(r.content);
      setDirty(false);
      setHistoryOpen(false);
    } catch {
      // ignore — leave panel open so user can retry
    } finally {
      setRestoring(null);
    }
  };

  return (
    <div className="flex h-full">
      <div className="w-48 shrink-0 border-r border-border p-2">
        <div className="mb-2 text-2xs font-semibold uppercase text-muted-foreground">Workspace</div>
        {files.map((f) => (
          <button key={f.name} onClick={() => setActive(f.name)}
            className={cn('flex w-full items-center justify-between rounded-md px-2 py-1.5 text-xs', active === f.name ? 'bg-accent font-medium' : 'hover:bg-accent text-muted-foreground')}>
            <span>{f.name}</span>
            {f.missing && <span className="text-2xs text-muted-foreground">new</span>}
          </button>
        ))}
        {files.length === 0 && <p className="px-2 py-3 text-2xs text-muted-foreground">No workspace files.</p>}
      </div>
      <div className="flex flex-1 flex-col">
        <div className="flex items-center justify-between border-b border-border px-3 py-2">
          <span className="text-xs font-medium">{active ?? 'Select a file'}</span>
          <div className="flex items-center gap-2">
            {active && (
              <div className="relative" ref={panelRef}>
                <button
                  onClick={toggleHistory}
                  className={cn('qr-btn qr-btn-ghost qr-btn-sm flex items-center gap-1', historyOpen && 'bg-accent')}
                  title="Version history"
                >
                  <History className="h-3.5 w-3.5" />
                  <span>History</span>
                </button>
                {historyOpen && (
                  <div className="absolute right-0 top-full z-50 mt-1 w-64 rounded-md border border-border bg-popover shadow-md">
                    <div className="border-b border-border px-3 py-2 text-xs font-semibold text-foreground">
                      Prior versions
                    </div>
                    <div className="max-h-64 overflow-y-auto">
                      {historyLoading && (
                        <p className="px-3 py-3 text-xs text-muted-foreground">Loading…</p>
                      )}
                      {!historyLoading && versions.length === 0 && (
                        <p className="px-3 py-3 text-xs text-muted-foreground">No previous versions.</p>
                      )}
                      {!historyLoading && versions.map((v) => (
                        <div key={v.id} className="flex items-center justify-between px-3 py-2 hover:bg-accent">
                          <span className="text-xs text-foreground">{formatTs(v.created_at)}</span>
                          <button
                            onClick={() => restore(v)}
                            disabled={restoring === v.id}
                            className="qr-btn qr-btn-ghost qr-btn-sm text-xs"
                          >
                            {restoring === v.id ? 'Restoring…' : 'Restore'}
                          </button>
                        </div>
                      ))}
                    </div>
                  </div>
                )}
              </div>
            )}
            <button onClick={save} disabled={!dirty || saving} className="qr-btn qr-btn-primary qr-btn-sm">{saving ? 'Saving…' : 'Save'}</button>
          </div>
        </div>
        <textarea
          value={content}
          onChange={(e) => { setContent(e.target.value); setDirty(true); }}
          className="qr-textarea flex-1 resize-none rounded-none border-0 font-mono text-xs"
          placeholder="# This file is loaded into the agent's system prompt…"
        />
      </div>
    </div>
  );
}
