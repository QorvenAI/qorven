'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useRef, useState } from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { workflows } from '@/lib/api';
import { ErrorBoundary } from '@/components/error-boundary';
import { descriptions } from '@/lib/branding';
import { AlertCircle, Plus, Play, GitBranch, Loader2, Trash2 } from 'lucide-react';
import { EmptyState, emptyStates } from '@/components/empty-state';
import { cn } from '@/lib/utils';

interface Workflow { id: string; name?: string; status?: string; trigger_type?: string; last_run?: string; enabled?: boolean; steps?: unknown[] }

export default function WorkflowsPage() {
  const router = useRouter();
  const [list, setList] = useState<Workflow[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [showCreate, setShowCreate] = useState(false);
  const [deleting, setDeleting] = useState<string | null>(null);

  const load = () => {
    setLoading(true);
    setError(null);
    workflows.list()
      .then((d) => { setList(d as Workflow[]); setLoading(false); })
      .catch((e) => { setError(e.message); setLoading(false); });
  };

  useEffect(load, []);

  const onCreated = (id: string) => {
    setShowCreate(false);
    router.push(`/workflows/${id}`);
  };

  const onDelete = async (id: string, name?: string) => {
    if (!confirm(`Delete "${name || 'this workflow'}"? This cannot be undone.`)) return;
    setDeleting(id);
    try {
      await workflows.delete(id);
      setList((prev) => prev.filter((w) => w.id !== id));
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Delete failed');
    } finally {
      setDeleting(null);
    }
  };

  return (
    <ErrorBoundary fallbackTitle="Failed to load workflows">
      <div className="space-y-6">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-lg font-semibold">Workflows</h1>
            <p className="text-sm text-muted-foreground">{descriptions.workflows}</p>
          </div>
          <button
            onClick={() => setShowCreate(true)}
            className="inline-flex items-center gap-2 rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90"
          >
            <Plus className="h-4 w-4" /> New Workflow
          </button>
        </div>

        {loading ? (
          <div className="space-y-2">
            {Array.from({ length: 4 }).map((_, i) => (
              <div key={i} className="flex items-center gap-4 rounded-xl border border-border bg-card px-4 py-3">
                <div className="h-4 w-4 animate-pulse rounded bg-muted" />
                <div className="flex-1 space-y-1">
                  <div className="h-4 w-40 animate-pulse rounded bg-muted" />
                  <div className="h-3 w-56 animate-pulse rounded bg-muted" />
                </div>
              </div>
            ))}
          </div>
        ) : error ? (
          <div className="flex flex-col items-center py-16 text-center">
            <AlertCircle className="h-8 w-8 text-destructive" />
            <p className="mt-2 text-sm text-destructive">{error}</p>
            <button onClick={load} className="mt-3 rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90">Retry</button>
          </div>
        ) : list.length === 0 ? (
          <EmptyState
            {...emptyStates.workflows}
            description="No workflows yet. Create one to coordinate multi-step agent flows with branching logic."
            actionLabel="New Workflow"
            onAction={() => setShowCreate(true)}
          />
        ) : (
          <div className="space-y-2">
            {list.map((wf) => (
              <div key={wf.id} className="group flex items-center gap-3 rounded-xl border border-border bg-card px-4 py-3 transition-colors hover:border-primary/30">
                <Link href={`/workflows/${wf.id}`} className="flex min-w-0 flex-1 items-center gap-3">
                  <GitBranch className="h-4 w-4 text-muted-foreground shrink-0" />
                  <div className="min-w-0 flex-1">
                    <p className="text-sm font-medium truncate">{wf.name ?? 'Untitled Workflow'}</p>
                    <p className="text-xs text-muted-foreground truncate">
                      <span className={cn('font-medium', wf.enabled ? 'text-emerald-500' : 'text-muted-foreground')}>
                        {wf.enabled ? 'enabled' : 'draft'}
                      </span>
                      {wf.trigger_type && <> · Trigger: {wf.trigger_type}</>}
                      {' · '}Last run: {wf.last_run ? new Date(wf.last_run).toLocaleDateString() : 'Never'}
                    </p>
                  </div>
                </Link>
                <button
                  onClick={() => workflows.run(wf.id)}
                  title="Run now"
                  className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg text-muted-foreground hover:bg-accent hover:text-primary"
                >
                  <Play className="h-4 w-4" />
                </button>
                <button
                  onClick={() => onDelete(wf.id, wf.name)}
                  disabled={deleting === wf.id}
                  title="Delete"
                  className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg text-muted-foreground opacity-0 transition-opacity group-hover:opacity-100 hover:bg-destructive/10 hover:text-destructive disabled:opacity-50"
                >
                  {deleting === wf.id ? <Loader2 className="h-4 w-4 animate-spin" /> : <Trash2 className="h-4 w-4" />}
                </button>
              </div>
            ))}
          </div>
        )}

        {showCreate && <CreateWorkflowDialog onClose={() => setShowCreate(false)} onCreated={onCreated} />}
      </div>
    </ErrorBoundary>
  );
}

function CreateWorkflowDialog({ onClose, onCreated }: { onClose: () => void; onCreated: (id: string) => void }) {
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => { inputRef.current?.focus(); }, []);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    const trimmed = name.trim();
    if (!trimmed) { setError('Name is required'); return; }
    setSubmitting(true);
    setError(null);
    try {
      const { id } = await workflows.create({ name: trimmed, description: description.trim() || undefined, steps: [] });
      onCreated(id);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Create failed');
      setSubmitting(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4" onClick={onClose}>
      <form
        onSubmit={submit}
        onClick={(e) => e.stopPropagation()}
        className="w-full max-w-md rounded-xl border border-border bg-card p-5 shadow-xl"
      >
        <h2 className="text-base font-semibold">New Workflow</h2>
        <p className="mt-1 text-xs text-muted-foreground">Give it a name — you&apos;ll add steps in the editor.</p>

        <label className="mt-4 block text-xs font-medium">Name</label>
        <input
          ref={inputRef}
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="Daily brief"
          className="qr-input mt-1"
        />

        <label className="mt-3 block text-xs font-medium">Description <span className="text-muted-foreground">(optional)</span></label>
        <textarea
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          rows={2}
          placeholder="What does this workflow do?"
          className="qr-textarea mt-1 resize-none"
        />

        {error && <p className="mt-3 text-xs text-destructive">{error}</p>}

        <div className="mt-5 flex justify-end gap-2">
          <button type="button" onClick={onClose} className="rounded-md border border-border px-3 py-1.5 text-xs hover:bg-accent">
            Cancel
          </button>
          <button
            type="submit"
            disabled={submitting}
            className="inline-flex items-center gap-1.5 rounded-md bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
          >
            {submitting ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Plus className="h-3.5 w-3.5" />}
            Create
          </button>
        </div>
      </form>
    </div>
  );
}
