'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useEffect, useState, useCallback, useRef } from 'react';
import { useStore } from '@/store';
import { cn } from '@/lib/utils';
import { soulGradient } from '@/components/soul-card';
import {
  FolderOpen, File, Image, FileCode, FileSpreadsheet,
  Upload, MoreHorizontal, AlertTriangle,
  Loader2, CheckCircle2, AlertCircle, Clock,
  Sparkles, X, Search,
} from 'lucide-react';
import { EmptyState, emptyStates } from '@/components/empty-state';
import { request, BASE, getToken } from '@/lib/api-core';

const mimeIcon = (mime: string) => {
  if (mime?.startsWith('image/')) return Image;
  if (mime?.includes('code') || mime?.includes('javascript') || mime?.includes('json') || mime?.includes('python')) return FileCode;
  if (mime?.includes('spreadsheet') || mime?.includes('csv')) return FileSpreadsheet;
  return File;
};

const formatSize = (bytes: number) => {
  if (!bytes) return '—';
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1048576) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / 1048576).toFixed(1)} MB`;
};

type EnrichmentStatus = 'pending' | 'processing' | 'done' | 'failed';

interface DriveFile {
  id: string;
  name: string;
  is_folder: boolean;
  mime_type?: string;
  size_bytes?: number;
  updated_at?: string;
  agent_id?: string;
  enrichment_status?: EnrichmentStatus;
  summary?: string;
  keywords?: string[];
  entities_extracted?: string[];
}

function EnrichmentBadge({ status }: { status?: EnrichmentStatus }) {
  if (!status || status === 'pending') {
    return (
      <span className="inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium bg-muted text-muted-foreground">
        <Clock className="h-3 w-3" />
        pending
      </span>
    );
  }
  if (status === 'processing') {
    return (
      <span className="inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium bg-blue-100 text-blue-600 dark:bg-blue-950 dark:text-blue-400 animate-pulse">
        <Loader2 className="h-3 w-3 animate-spin" />
        processing
      </span>
    );
  }
  if (status === 'done') {
    return (
      <span className="inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium bg-emerald-100 text-emerald-700 dark:bg-emerald-950 dark:text-emerald-400">
        <CheckCircle2 className="h-3 w-3" />
        enriched
      </span>
    );
  }
  // failed
  return (
    <span className="inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium bg-red-100 text-red-600 dark:bg-red-950 dark:text-red-400">
      <AlertCircle className="h-3 w-3" />
      failed
    </span>
  );
}

function FileDetailModal({ file, onClose, onEnrich }: {
  file: DriveFile;
  onClose: () => void;
  onEnrich: (id: string) => void;
}) {
  const Icon = mimeIcon(file.mime_type ?? '');
  const keywords: string[] = Array.isArray(file.keywords) ? file.keywords : [];
  const entities: string[] = Array.isArray(file.entities_extracted) ? file.entities_extracted : [];
  const canEnrich = !file.is_folder && (file.enrichment_status === 'pending' || file.enrichment_status === 'failed' || !file.enrichment_status);

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm" onClick={onClose}>
      <div
        className="relative w-full max-w-lg rounded-xl bg-card border border-border shadow-2xl p-6 mx-4"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div className="flex items-start justify-between gap-3 mb-5">
          <div className="flex items-center gap-3 min-w-0">
            <div className="shrink-0 h-10 w-10 rounded-lg bg-muted flex items-center justify-center">
              <Icon className="h-5 w-5 text-muted-foreground" />
            </div>
            <div className="min-w-0">
              <h2 className="text-sm font-semibold truncate">{file.name}</h2>
              <div className="flex items-center gap-2 mt-1">
                <EnrichmentBadge status={file.enrichment_status} />
                {file.size_bytes ? (
                  <span className="text-xs text-muted-foreground">{formatSize(file.size_bytes)}</span>
                ) : null}
                {file.updated_at ? (
                  <span className="text-xs text-muted-foreground">
                    {new Date(file.updated_at).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })}
                  </span>
                ) : null}
              </div>
            </div>
          </div>
          <button onClick={onClose} className="shrink-0 h-7 w-7 rounded-md hover:bg-muted flex items-center justify-center text-muted-foreground">
            <X className="h-4 w-4" />
          </button>
        </div>

        {/* Summary */}
        {file.summary ? (
          <div className="mb-4">
            <p className="text-xs font-medium text-muted-foreground uppercase tracking-wider mb-1.5">Summary</p>
            <p className="text-sm text-foreground/80 leading-relaxed">{file.summary}</p>
          </div>
        ) : (
          file.enrichment_status === 'done' ? null : (
            <div className="mb-4 rounded-lg bg-muted border border-border p-3 text-sm text-muted-foreground italic">
              No summary yet — click &quot;Enrich&quot; to analyse this document.
            </div>
          )
        )}

        {/* Keywords */}
        {keywords.length > 0 && (
          <div className="mb-4">
            <p className="text-xs font-medium text-muted-foreground uppercase tracking-wider mb-2">Keywords</p>
            <div className="flex flex-wrap gap-1.5">
              {keywords.map((kw) => (
                <span key={kw} className="rounded-full px-2.5 py-0.5 text-xs bg-indigo-950 text-indigo-300 border border-indigo-800">
                  {kw}
                </span>
              ))}
            </div>
          </div>
        )}

        {/* Entities */}
        {entities.length > 0 && (
          <div className="mb-5">
            <p className="text-xs font-medium text-muted-foreground uppercase tracking-wider mb-2">Entities</p>
            <div className="flex flex-wrap gap-1.5">
              {entities.map((ent) => (
                <span key={ent} className="rounded-full px-2.5 py-0.5 text-xs bg-amber-950 text-amber-300 border border-amber-800">
                  {ent}
                </span>
              ))}
            </div>
          </div>
        )}

        {/* Actions */}
        {canEnrich && (
          <button
            onClick={() => { onEnrich(file.id); onClose(); }}
            className="w-full flex items-center justify-center gap-2 h-9 rounded-lg bg-indigo-600 hover:bg-indigo-500 text-white text-sm font-medium transition-colors"
          >
            <Sparkles className="h-4 w-4" />
            Enrich Document
          </button>
        )}
      </div>
    </div>
  );
}

export default function DrivePage() {
  const driveSoulFilter = useStore((s) => s.driveSoulFilter);
  const souls = useStore((s) => s.souls);
  const [files, setFiles] = useState<DriveFile[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [uploading, setUploading] = useState(false);
  const [parentId, setParentId] = useState<string | null>(null);
  const [path, setPath] = useState<{ id: string | null; name: string }[]>([{ id: null, name: 'Root' }]);
  const [selectedFile, setSelectedFile] = useState<DriveFile | null>(null);
  const [search, setSearch] = useState('');
  const fileInputRef = useRef<HTMLInputElement>(null);

  const fetchFiles = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const params = new URLSearchParams();
      if (driveSoulFilter) params.set('agent_id', driveSoulFilter);
      if (parentId) params.set('parent_id', parentId);
      const data = await request<DriveFile[] | { files?: DriveFile[] }>(`/drive/files?${params}`);
      setFiles(Array.isArray(data) ? data : ((data as { files?: DriveFile[] }).files ?? []));
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load files');
    } finally {
      setLoading(false);
    }
  }, [driveSoulFilter, parentId]);

  useEffect(() => {
    fetchFiles();
  }, [fetchFiles]);

  const openFolder = (id: string, name: string) => {
    setParentId(id);
    setPath((prev) => [...prev, { id, name }]);
  };

  const navigateTo = (idx: number) => {
    setParentId(path[idx]!.id);
    setPath((prev) => prev.slice(0, idx + 1));
  };

  const handleEnrich = async (id: string) => {
    setFiles((prev) => prev.map((f) => f.id === id ? { ...f, enrichment_status: 'processing' } : f));
    try {
      await request(`/drive/files/${id}/enrich`, { method: 'POST', body: '{}' });
      setTimeout(fetchFiles, 3000);
      setTimeout(fetchFiles, 8000);
      setTimeout(fetchFiles, 15000);
    } catch {
      setFiles((prev) => prev.map((f) => f.id === id ? { ...f, enrichment_status: 'failed' } : f));
    }
  };

  const handleUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    e.target.value = '';
    setUploading(true);
    try {
      const form = new FormData();
      form.append('file', file);
      if (parentId) form.append('parent_id', parentId);
      const res = await fetch(`${BASE}/drive/upload`, {
        method: 'POST',
        headers: { Authorization: `Bearer ${getToken()}` },
        body: form,
      });
      if (!res.ok) throw new Error(`Upload failed: ${res.status}`);
      await fetchFiles();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Upload failed');
    } finally {
      setUploading(false);
    }
  };

  const soulMap = Object.fromEntries(souls.map((s) => [s.id, s]));

  // Filter by search query (name, summary, or keywords)
  const filteredFiles = files.filter((f) => {
    if (!search.trim()) return true;
    const q = search.toLowerCase();
    if (f.name.toLowerCase().includes(q)) return true;
    if (f.summary?.toLowerCase().includes(q)) return true;
    if (Array.isArray(f.keywords) && f.keywords.some((k) => k.toLowerCase().includes(q))) return true;
    if (Array.isArray(f.entities_extracted) && f.entities_extracted.some((e) => e.toLowerCase().includes(q))) return true;
    return false;
  });

  return (
    <div className="full-bleed h-[calc(100vh-var(--header-height)-2.5rem)]">
      <div className="flex flex-col h-full">
        {/* Toolbar */}
        <div className="flex items-center justify-between px-5 py-3 border-b border-border shrink-0 gap-3">
          {/* Breadcrumb */}
          <nav className="flex items-center gap-1 text-sm shrink-0">
            {path.map((p, i) => (
              <span key={i} className="flex items-center gap-1">
                {i > 0 && <span className="text-muted-foreground">/</span>}
                <button onClick={() => navigateTo(i)}
                  className={cn('hover:text-foreground transition-colors', i === path.length - 1 ? 'text-foreground font-medium' : 'text-muted-foreground')}>
                  {p.name}
                </button>
              </span>
            ))}
          </nav>

          {/* Search */}
          <div className="relative flex-1 max-w-xs">
            <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground pointer-events-none" />
            <input
              type="text"
              placeholder="Search by name, summary, keywords…"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="w-full h-8 rounded-md border border-border bg-background pl-8 pr-3 text-sm placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-ring"
            />
          </div>

          <input ref={fileInputRef} type="file" className="hidden" onChange={handleUpload} />
          <button
            onClick={() => fileInputRef.current?.click()}
            disabled={uploading}
            className="shrink-0 flex items-center gap-1.5 h-9 rounded-md bg-primary text-primary-foreground px-4 text-sm font-medium hover:bg-primary/90 disabled:opacity-60"
          >
            {uploading ? <Loader2 className="h-4 w-4 animate-spin" /> : <Upload className="h-4 w-4" />}
            {uploading ? 'Uploading…' : 'Upload'}
          </button>
        </div>

        {/* Error banner */}
        {error && (
          <div className="flex items-center gap-2 px-5 py-2.5 bg-destructive/10 border-b border-destructive/20 text-sm text-destructive shrink-0">
            <AlertTriangle className="h-4 w-4 shrink-0" />
            <span className="flex-1">{error}</span>
            <button onClick={() => setError(null)} className="shrink-0 text-muted-foreground hover:text-foreground">
              <X className="h-4 w-4" />
            </button>
          </div>
        )}

        {/* File list */}
        <div className="flex-1 overflow-y-auto">
          {loading ? (
            <div className="flex items-center justify-center h-full">
              <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
            </div>
          ) : filteredFiles.length === 0 ? (
            <div className="flex items-center justify-center h-full">
              <EmptyState
                {...emptyStates.drive}
                title={search ? 'No files match your search' : 'No files yet'}
                description={search ? 'Try a different query or clear the search.' : 'Upload files or create folders to get started.'}
              />
            </div>
          ) : (
            <table className="w-full">
              <thead>
                <tr className="border-b border-border text-sm text-muted-foreground">
                  <th className="text-left font-medium px-5 py-2.5">Name</th>
                  <th className="text-left font-medium px-3 py-2.5 w-28">Enrichment</th>
                  <th className="text-left font-medium px-3 py-2.5 w-32">Qor</th>
                  <th className="text-left font-medium px-3 py-2.5 w-24">Size</th>
                  <th className="text-left font-medium px-3 py-2.5 w-36">Modified</th>
                  <th className="w-10"></th>
                </tr>
              </thead>
              <tbody>
                {filteredFiles.map((f) => {
                  const Icon = f.is_folder ? FolderOpen : mimeIcon(f.mime_type ?? '');
                  const soul = f.agent_id ? soulMap[f.agent_id] : null;
                  return (
                    <tr
                      key={f.id}
                      className="border-b border-border hover:bg-accent/30 transition-colors cursor-pointer"
                      onClick={() => f.is_folder ? openFolder(f.id, f.name) : setSelectedFile(f)}
                    >
                      <td className="px-5 py-2.5">
                        <div className="flex items-center gap-3">
                          <Icon className={cn('h-5 w-5 shrink-0', f.is_folder ? 'text-amber-400' : 'text-muted-foreground')} />
                          <div className="min-w-0">
                            <span className="text-sm font-medium truncate block">{f.name}</span>
                            {f.summary && (
                              <span className="text-xs text-muted-foreground truncate block max-w-xs">{f.summary}</span>
                            )}
                          </div>
                        </div>
                      </td>
                      <td className="px-3 py-2.5">
                        {!f.is_folder && (
                          <div className="flex items-center gap-1.5">
                            <EnrichmentBadge status={f.enrichment_status} />
                            {(f.enrichment_status === 'pending' || f.enrichment_status === 'failed' || !f.enrichment_status) && (
                              <button
                                onClick={(e) => { e.stopPropagation(); handleEnrich(f.id); }}
                                title="Enrich document"
                                className="h-5 w-5 rounded flex items-center justify-center text-muted-foreground hover:text-indigo-400 transition-colors"
                              >
                                <Sparkles className="h-3.5 w-3.5" />
                              </button>
                            )}
                          </div>
                        )}
                      </td>
                      <td className="px-3 py-2.5">
                        {soul && (
                          <div className="flex items-center gap-1.5">
                            <div className={cn('flex h-5 w-5 items-center justify-center rounded-full bg-gradient-to-br text-2xs font-semibold text-white', soulGradient(soul.display_name))}>
                              {soul.display_name.charAt(0)}
                            </div>
                            <span className="text-xs text-muted-foreground truncate">{soul.display_name}</span>
                          </div>
                        )}
                      </td>
                      <td className="px-3 py-2.5 text-sm text-muted-foreground">{f.is_folder ? '—' : formatSize(f.size_bytes ?? 0)}</td>
                      <td className="px-3 py-2.5 text-sm text-muted-foreground">
                        {f.updated_at ? new Date(f.updated_at).toLocaleDateString('en-US', { month: 'short', day: 'numeric' }) : ''}
                      </td>
                      <td className="px-2">
                        <button className="h-7 w-7 flex items-center justify-center rounded-md hover:bg-accent text-muted-foreground"
                          onClick={(e) => e.stopPropagation()}>
                          <MoreHorizontal className="h-4 w-4" />
                        </button>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          )}
        </div>
      </div>

      {/* File detail modal */}
      {selectedFile && (
        <FileDetailModal
          file={selectedFile}
          onClose={() => setSelectedFile(null)}
          onEnrich={handleEnrich}
        />
      )}
    </div>
  );
}
