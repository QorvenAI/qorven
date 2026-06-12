'use client';
// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState } from 'react';
import { cn } from '@/lib/utils';
import { driveApi, type WorkspaceFileMeta } from '@/lib/api-workspace';

export function WorkspaceEditor({ agentId }: { agentId: string }) {
  const [files, setFiles] = useState<WorkspaceFileMeta[]>([]);
  const [active, setActive] = useState<string | null>(null);
  const [content, setContent] = useState('');
  const [dirty, setDirty] = useState(false);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    driveApi.workspaceFiles(agentId).then((r) => {
      setFiles(r.files ?? []);
      if (r.files?.length) setActive(r.files[0]!.name);
    }).catch(() => setFiles([]));
  }, [agentId]);

  useEffect(() => {
    if (!active) return;
    driveApi.workspaceGet(agentId, active).then((r) => { setContent(r.content); setDirty(false); }).catch(() => setContent(''));
  }, [agentId, active]);

  const save = async () => {
    if (!active) return;
    setSaving(true);
    try { await driveApi.workspacePut(agentId, active, content); setDirty(false); } finally { setSaving(false); }
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
          <button onClick={save} disabled={!dirty || saving} className="qr-btn qr-btn-primary qr-btn-sm">{saving ? 'Saving…' : 'Save'}</button>
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
