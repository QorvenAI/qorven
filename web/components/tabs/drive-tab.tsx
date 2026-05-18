'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState } from 'react';
import { cn } from '@/lib/utils';
import { UploadZone } from '@/components/upload-zone';
import { Upload, FolderPlus, FileText, Folder, Download, MoreHorizontal, LayoutGrid, List } from 'lucide-react';

interface Props { agentId: string; scope?: 'agent' | 'global' }

export function DriveTab({ agentId, scope = 'agent' }: Props) {
  const [files, setFiles] = useState<any[]>([]);
  const [view, setView] = useState<'list' | 'grid'>('list');
  const [quota, setQuota] = useState<{ used_bytes: number; total_bytes: number; percent: number } | null>(null);
  const getToken = () => typeof window !== 'undefined' ? (localStorage.getItem('qorven_token') || '') : '';

  useEffect(() => {
    fetch(`/api/v1/drive/files?agent_id=${agentId}`, { headers: { Authorization: `Bearer ${getToken()}` } })
      .then((r) => r.json()).then((d) => setFiles(Array.isArray(d) ? d : [])).catch(() => {});
    fetch(`/api/v1/drive/quota?agent_id=${agentId}`, { headers: { Authorization: `Bearer ${getToken()}` } })
      .then((r) => r.json()).then(setQuota).catch(() => {});
  }, [agentId, getToken()]);

  const formatSize = (bytes: number) => {
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  };

  return (
    <div className="max-w-3xl mx-auto">
      {/* Action bar */}
      <div className="flex items-center gap-2 mb-4">
        <button className="flex items-center gap-1.5 rounded-lg bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90">
          <Upload className="h-3.5 w-3.5" /> Upload
        </button>
        <button className="flex items-center gap-1.5 rounded-lg border border-border px-3 py-1.5 text-xs hover:bg-accent">
          <FolderPlus className="h-3.5 w-3.5" /> New Folder
        </button>
        <div className="flex-1" />
        <button onClick={() => setView('grid')} className={cn('h-7 w-7 flex items-center justify-center rounded', view === 'grid' ? 'bg-accent' : 'text-muted-foreground')}>
          <LayoutGrid className="h-3.5 w-3.5" />
        </button>
        <button onClick={() => setView('list')} className={cn('h-7 w-7 flex items-center justify-center rounded', view === 'list' ? 'bg-accent' : 'text-muted-foreground')}>
          <List className="h-3.5 w-3.5" />
        </button>
      </div>

      {/* Breadcrumb */}
      <div className="text-2xs text-muted-foreground mb-3">📂 workspace/</div>

      {/* File list */}
      {files.length === 0 ? (
        <div className="py-12 text-center">
          <p className="text-sm text-muted-foreground">No files yet</p>
          <p className="text-2xs text-muted-foreground mt-1">Upload files or let your Qor create them</p>
        </div>
      ) : (
        <div className="rounded-xl border border-border divide-y divide-border">
          {files.map((f) => (
            <div key={f.id} className="flex items-center gap-3 px-4 py-2.5 hover:bg-accent/30 group">
              {f.is_folder
                ? <Folder className="h-4 w-4 text-amber-400 shrink-0" />
                : <FileText className="h-4 w-4 text-blue-400 shrink-0" />}
              <span className="flex-1 text-sm truncate">{f.name}</span>
              {!f.is_folder && <span className="text-2xs text-muted-foreground shrink-0">{formatSize(f.size_bytes)}</span>}
              <span className="text-2xs text-muted-foreground shrink-0 w-20 text-right">{f.updated_at ? new Date(f.updated_at).toLocaleDateString() : ''}</span>
              <div className="flex gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                {!f.is_folder && (
                  <button className="h-6 w-6 flex items-center justify-center rounded text-muted-foreground hover:text-foreground">
                    <Download className="h-3 w-3" />
                  </button>
                )}
                <button className="h-6 w-6 flex items-center justify-center rounded text-muted-foreground hover:text-foreground">
                  <MoreHorizontal className="h-3 w-3" />
                </button>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Quota bar */}
      {quota && (
        <div className="mt-4 flex items-center gap-3">
          <div className="flex-1 h-2 rounded-full bg-muted overflow-hidden">
            <div className={cn('h-full rounded-full', quota.percent > 80 ? 'bg-destructive' : 'bg-primary')}
              style={{ width: `${Math.min(quota.percent, 100)}%` }} />
          </div>
          <span className="text-2xs text-muted-foreground shrink-0">
            {formatSize(quota.used_bytes)} / {formatSize(quota.total_bytes)}
          </span>
        </div>
      )}

      {/* Sandbox artifacts */}
      <SandboxArtifacts agentId={agentId} />
    </div>
  );
}

function SandboxArtifacts({ agentId }: { agentId: string }) {
  const [artifacts, setArtifacts] = useState<any[]>([]);
  const [open, setOpen] = useState(false);
  const getToken = () => typeof window !== 'undefined' ? (localStorage.getItem('qorven_token') || '') : '';

  useEffect(() => {
    fetch(`/api/v1/sandbox/artifacts?agent_id=${agentId}`, { headers: { Authorization: `Bearer ${getToken()}` } })
      .then((r) => r.json()).then((d) => setArtifacts(Array.isArray(d) ? d : [])).catch(() => {});
  }, [agentId, getToken()]);

  if (!artifacts.length) return null;

  return (
    <div className="mt-6">
      <button onClick={() => setOpen(!open)} className="flex items-center gap-2 text-xs font-medium text-muted-foreground hover:text-foreground">
        ⚡ Sandbox Artifacts ({artifacts.length})
      </button>
      {open && (
        <div className="mt-2 rounded-xl border border-border divide-y divide-border">
          {artifacts.map((a, i) => (
            <div key={i} className="flex items-center gap-3 px-4 py-2 text-xs">
              <span className="text-primary/70">⚡</span>
              <span className="flex-1 truncate">{a.name || a.path}</span>
              <span className="text-2xs text-muted-foreground">{a.size ? `${(a.size / 1024).toFixed(1)} KB` : ''}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
