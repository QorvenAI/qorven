'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState } from 'react';
import { ArrowRight } from 'lucide-react';
import { cn } from '@/lib/utils';
import { agents } from '@/lib/api-agents';
import { request } from '@/lib/api-core';
import type { Soul } from '@/types';

interface OfficerSetupCardProps {
  role: 'cto' | 'cmo';
  roleLabel: string;          // "CTO", "CMO"
  pageName: string;           // "code", "social"
  defaultName: string;        // "Prime Coder", "Prime Marketer"
  blurb: string;              // one-line description
  onCreate: (opts: { name: string; model: string; providerId: string }) => Promise<void>;
}

export function OfficerSetupCard({ role, roleLabel, pageName, defaultName, blurb, onCreate }: OfficerSetupCardProps) {
  const [name, setName]             = useState(defaultName);
  const [models, setModels]         = useState<string[]>([]);
  const [model, setModel]           = useState('');
  const [providerId, setProviderId] = useState('');
  const [loading, setLoading]       = useState(true);
  const [creating, setCreating]     = useState(false);
  const [error, setError]           = useState('');

  useEffect(() => {
    (async () => {
      try {
        const coo = (await agents.byRole('coo')) ?? (await agents.byKey('chief'));
        const pid = coo?.provider_id ?? '';
        setProviderId(pid);
        if (pid) {
          const res = await request<{ models: string[] }>(`/providers/${pid}/models`).catch(() => ({ models: [] as string[] }));
          const list = res.models ?? [];
          setModels(list);
          if (coo?.model && list.includes(coo.model)) setModel(coo.model);
          else if (list[0]) setModel(list[0]);
        }
      } catch {
        setError('Could not load models — you can still create with the default.');
      } finally {
        setLoading(false);
      }
    })();
  }, []);

  const create = async () => {
    if (creating) return;
    setCreating(true);
    setError('');
    try {
      await onCreate({ name: name.trim() || defaultName, model, providerId });
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Could not create. Try again.');
      setCreating(false);
    }
  };

  return (
    <div className="flex h-full items-center justify-center p-8">
      <div className="w-full max-w-md rounded-2xl border border-border bg-card p-6 space-y-5">
        <div className="flex items-start gap-3">
          <div className="h-9 w-9 shrink-0 rounded-full bg-gradient-to-br from-violet-500 to-fuchsia-600 flex items-center justify-center">
            <span className="text-xs font-bold text-white">{roleLabel}</span>
          </div>
          <div>
            <h2 className="text-lg font-semibold text-foreground">Meet your {roleLabel}</h2>
            <p className="text-sm text-muted-foreground mt-0.5">{blurb}</p>
          </div>
        </div>

        <div className="space-y-1.5">
          <label className="text-xs font-medium text-muted-foreground">Name</label>
          <input
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder={defaultName}
            className="h-10 w-full rounded-xl border border-border bg-background px-3 text-sm outline-none focus:border-primary"
          />
        </div>

        <div className="space-y-1.5">
          <label className="text-xs font-medium text-muted-foreground">Model</label>
          {loading ? (
            <div className="h-10 rounded-xl border border-border bg-background px-3 flex items-center text-sm text-muted-foreground">Loading models…</div>
          ) : models.length > 0 ? (
            <div className="max-h-40 overflow-y-auto flex flex-col gap-1 pr-1">
              {models.map((m) => (
                <button
                  key={m}
                  onClick={() => setModel(m)}
                  className={cn(
                    'flex items-center justify-between rounded-lg border px-3 py-2 text-left text-xs font-mono transition-colors cursor-pointer',
                    m === model ? 'border-primary/50 bg-primary/5 text-foreground' : 'border-border bg-background text-foreground hover:bg-accent',
                  )}>
                  <span className="truncate">{m}</span>
                  {m === model && <span className="shrink-0 ml-2 text-[10px] uppercase text-primary font-semibold">Selected</span>}
                </button>
              ))}
            </div>
          ) : (
            <div className="h-10 rounded-xl border border-border bg-background px-3 flex items-center text-sm text-muted-foreground">Using COO&apos;s default model</div>
          )}
        </div>

        {error && <p className="text-xs text-destructive">{error}</p>}

        <button
          onClick={create}
          disabled={creating}
          className="w-full inline-flex items-center justify-center gap-2 rounded-xl bg-primary px-5 py-2.5 text-sm font-semibold text-primary-foreground hover:bg-primary/90 disabled:opacity-50 transition-opacity cursor-pointer">
          {creating ? 'Creating…' : <>Create {roleLabel} <ArrowRight className="h-4 w-4" /></>}
        </button>
      </div>
    </div>
  );
}
