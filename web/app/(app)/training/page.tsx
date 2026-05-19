'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useCallback, useEffect, useState } from 'react';
import { Download, Loader2, AlertCircle } from 'lucide-react';
import { CanvasHeader } from '@/components/layouts/canvas-header';
import { agents, training, BASE, getToken } from '@/lib/api';
import { EmptyState, emptyStates } from '@/components/empty-state';
import { ErrorBoundary } from '@/components/error-boundary';
import { TableRowSkeleton } from '@/components/skeletons';
import type { Soul } from '@/types';

type ExportFormat = 'jsonl' | 'preferences' | 'corrections';

export default function TrainingPage() {
  const [list, setList] = useState<Soul[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [exporting, setExporting] = useState<string | null>(null);
  const [format, setFormat] = useState<ExportFormat>('jsonl');

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await agents.list();
      setList(Array.isArray(data) ? data : []);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to load agents');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { load(); }, [load]);

  const exportAgent = async (agentId: string, agentName: string) => {
    setExporting(agentId);
    try {
      const token = getToken();
      const url = `${BASE}/training/export/${encodeURIComponent(agentId)}?format=${format}`;
      const res = await fetch(url, { headers: { Authorization: `Bearer ${token}` } });
      if (!res.ok) throw new Error(`Export failed: ${res.status}`);
      const blob = await res.blob();
      const objectUrl = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = objectUrl;
      a.download = `training-${agentName.toLowerCase().replace(/\s+/g, '-')}-${format}.jsonl`;
      a.click();
      URL.revokeObjectURL(objectUrl);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Export failed');
    } finally {
      setExporting(null);
    }
  };

  return (
    <ErrorBoundary fallbackTitle="Failed to load training page">
      <div className="space-y-6">
        <CanvasHeader title="Training Data" description="Export conversation histories and feedback to fine-tune or evaluate your Qors." />

        {/* Format picker */}
        <div className="flex items-center gap-3 rounded-xl border border-border bg-card px-4 py-3">
          <span className="text-xs font-medium text-muted-foreground">Export format:</span>
          {(['jsonl', 'preferences', 'corrections'] as ExportFormat[]).map((f) => (
            <button
              key={f}
              onClick={() => setFormat(f)}
              className={`rounded-md px-3 py-1 text-xs font-medium transition-colors ${
                format === f
                  ? 'bg-primary text-primary-foreground'
                  : 'text-muted-foreground hover:bg-accent hover:text-foreground'
              }`}
            >
              {f}
            </button>
          ))}
          <span className="ml-auto text-2xs text-muted-foreground">
            {format === 'jsonl' && 'Raw message pairs — compatible with most fine-tuning APIs.'}
            {format === 'preferences' && 'Ranked completions — for RLHF / DPO training.'}
            {format === 'corrections' && 'Edited outputs only — captures explicit corrections.'}
          </span>
        </div>

        {error && (
          <div className="flex items-center gap-2 rounded-lg border border-destructive/40 bg-destructive/5 p-3 text-xs text-destructive">
            <AlertCircle className="h-4 w-4 shrink-0" />
            <span>{error}</span>
          </div>
        )}

        {loading ? (
          <div className="space-y-1">
            {Array.from({ length: 4 }).map((_, i) => <TableRowSkeleton key={i} cols={3} />)}
          </div>
        ) : list.length === 0 ? (
          <EmptyState {...emptyStates.souls} description="No Qors found. Create one first before exporting training data." />
        ) : (
          <div className="rounded-xl border border-border overflow-hidden">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-border bg-muted/30 text-2xs uppercase tracking-wider text-muted-foreground">
                  <th className="px-4 py-2.5 text-left font-medium">Qor</th>
                  <th className="px-4 py-2.5 text-left font-medium hidden sm:table-cell">Model</th>
                  <th className="px-4 py-2.5 text-right font-medium">Export</th>
                </tr>
              </thead>
              <tbody>
                {list.map((a) => (
                  <tr key={a.id} className="border-b border-border/60 last:border-0 hover:bg-accent/30 transition-colors">
                    <td className="px-4 py-3">
                      <p className="font-medium">{a.display_name || a.agent_key || 'Unnamed'}</p>
                      <p className="mt-0.5 font-mono text-2xs text-muted-foreground">{a.id.slice(0, 8)}</p>
                    </td>
                    <td className="px-4 py-3 font-mono text-2xs text-muted-foreground hidden sm:table-cell">
                      {a.model || '—'}
                    </td>
                    <td className="px-4 py-3 text-right">
                      <button
                        onClick={() => exportAgent(a.id, a.display_name || a.agent_key || a.id)}
                        disabled={exporting === a.id}
                        className="inline-flex items-center gap-1.5 rounded-md border border-border px-3 py-1.5 text-xs font-medium hover:bg-accent disabled:opacity-50 transition-colors"
                      >
                        {exporting === a.id
                          ? <Loader2 className="h-3.5 w-3.5 animate-spin" />
                          : <Download className="h-3.5 w-3.5" />}
                        Export {format}
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </ErrorBoundary>
  );
}
