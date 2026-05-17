'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

/**
 * /goals — simple goal/OKR tracker (T2.11a).
 *
 * Backend model per goal: title + description + optional target_value
 * + unit (KPI-style). We show current_value / target_value as a
 * progress bar when a target is set, and fall back to a plain card
 * otherwise. Create form is inline at the top.
 */

import { useCallback, useEffect, useState } from 'react';
import { Target, Plus, Loader2, AlertCircle, CheckCircle2, TrendingUp } from 'lucide-react';
import { cn } from '@/lib/utils';
import { goals, type Goal } from '@/lib/api';
import { useStore } from '@/store';

export default function GoalsPage() {
  const [list, setList] = useState<Goal[]>([]);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState<string | null>(null);
  const [creating, setCreating] = useState(false);

  const refresh = useCallback(async () => {
    setLoading(true);
    setErr(null);
    try {
      const res = await goals.list();
      setList(res.goals ?? []);
    } catch (e) {
      setErr(e instanceof Error ? e.message : 'Failed to load goals');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { refresh(); }, [refresh]);

  return (
    <div className="mx-auto max-w-3xl space-y-5 p-4 lg:p-6">
      <header className="flex items-center gap-3">
        <Target className="h-6 w-6 text-primary" />
        <h1 className="text-lg font-semibold">Goals</h1>
        <button
          onClick={() => setCreating((v) => !v)}
          className="ml-auto inline-flex items-center gap-1.5 rounded-md bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90"
        >
          <Plus className="h-3.5 w-3.5" />
          {creating ? 'Cancel' : 'New goal'}
        </button>
      </header>

      {creating && <CreateForm onCreated={() => { setCreating(false); refresh(); }} />}

      {err && (
        <div className="flex items-center gap-2 rounded-lg border border-destructive/40 bg-destructive/5 p-3 text-xs text-destructive">
          <AlertCircle className="h-4 w-4" />
          <span>{err}</span>
        </div>
      )}

      {loading ? (
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <Loader2 className="h-3.5 w-3.5 animate-spin" /> Loading…
        </div>
      ) : list.length === 0 ? (
        <div className="flex flex-col items-center gap-2 rounded-xl border border-dashed border-border/60 bg-card/40 px-6 py-10 text-center">
          <Target className="h-6 w-6 text-muted-foreground/60" />
          <p className="text-sm">No goals yet.</p>
          <p className="text-2xs text-muted-foreground">
            Track KPIs and outcomes — agents can read these as context.
          </p>
        </div>
      ) : (
        <ul className="space-y-2">
          {list.map((g) => <GoalCard key={g.id} goal={g} />)}
        </ul>
      )}
    </div>
  );
}

function GoalCard({ goal }: { goal: Goal }) {
  const target = goal.target_value ?? 0;
  const current = goal.current_value ?? 0;
  const pct = target > 0 ? Math.min(100, (current / target) * 100) : 0;
  const done = target > 0 && current >= target;
  return (
    <li className="rounded-xl border border-border bg-card p-4">
      <div className="flex items-start gap-3">
        {done ? (
          <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-emerald-500" />
        ) : (
          <TrendingUp className="mt-0.5 h-4 w-4 shrink-0 text-primary" />
        )}
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <h3 className="truncate text-sm font-medium">{goal.title}</h3>
            {goal.agent_name && (
              <span className="shrink-0 rounded-sm bg-muted px-1.5 py-0.5 font-mono text-xs text-muted-foreground">
                {goal.agent_name}
              </span>
            )}
            {goal.status && (
              <span className="shrink-0 rounded-sm border border-border px-1.5 py-0.5 font-mono text-xs text-muted-foreground">
                {goal.status}
              </span>
            )}
          </div>
          {goal.description && (
            <p className="mt-1 text-xs leading-relaxed text-muted-foreground">{goal.description}</p>
          )}
          {target > 0 && (
            <div className="mt-3 space-y-1">
              <div className="flex items-center justify-between text-2xs font-mono">
                <span className={done ? 'text-emerald-500' : 'text-primary'}>
                  {current.toLocaleString()} / {target.toLocaleString()} {goal.unit ?? ''}
                </span>
                <span className="text-muted-foreground">{pct.toFixed(0)}%</span>
              </div>
              <div className="h-1.5 overflow-hidden rounded-full bg-muted">
                <div
                  className={cn('h-full rounded-full', done ? 'bg-emerald-500' : 'bg-primary')}
                  style={{ width: `${pct}%` }}
                />
              </div>
            </div>
          )}
          {goal.due_at && (
            <p className="mt-2 text-2xs text-muted-foreground">
              Due {new Date(goal.due_at).toLocaleDateString()}
            </p>
          )}
        </div>
      </div>
    </li>
  );
}

function CreateForm({ onCreated }: { onCreated: () => void }) {
  const souls = useStore((s) => s.souls);
  const [title, setTitle] = useState('');
  const [description, setDescription] = useState('');
  const [agentId, setAgentId] = useState('');
  const [target, setTarget] = useState('');
  const [unit, setUnit] = useState('');
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!title.trim()) return;
    setBusy(true);
    setErr(null);
    try {
      await goals.create({
        title: title.trim(),
        description: description.trim() || undefined,
        agent_id: agentId || undefined,
        target_value: target ? Number(target) : undefined,
        unit: unit.trim() || undefined,
      });
      setTitle(''); setDescription(''); setAgentId(''); setTarget(''); setUnit('');
      onCreated();
    } catch (e) {
      setErr(e instanceof Error ? e.message : 'Create failed');
    } finally {
      setBusy(false);
    }
  };

  return (
    <form onSubmit={submit} className="space-y-2 rounded-xl border border-border bg-card p-4 text-xs">
      <input
        value={title}
        onChange={(e) => setTitle(e.target.value)}
        placeholder="Title (required)"
        className="qr-input"
        required
      />
      <textarea
        value={description}
        onChange={(e) => setDescription(e.target.value)}
        rows={2}
        placeholder="Description (optional)"
        className="qr-textarea resize-none"
      />
      <div className="grid grid-cols-3 gap-2">
        <select
          value={agentId}
          onChange={(e) => setAgentId(e.target.value)}
          className="qr-input text-xs"
        >
          <option value="">Assign to…</option>
          {souls.map((s) => <option key={s.id} value={s.id}>{s.display_name || s.agent_key}</option>)}
        </select>
        <input
          value={target}
          onChange={(e) => setTarget(e.target.value)}
          placeholder="Target (optional)"
          type="number"
          className="qr-input text-xs"
        />
        <input
          value={unit}
          onChange={(e) => setUnit(e.target.value)}
          placeholder="Unit"
          className="qr-input text-xs"
        />
      </div>
      <button
        type="submit"
        disabled={busy || !title.trim()}
        className="inline-flex items-center gap-1.5 rounded-md bg-primary px-3 py-1.5 font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
      >
        {busy ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Plus className="h-3.5 w-3.5" />}
        Create goal
      </button>
      {err && <p className="text-2xs text-destructive">{err}</p>}
    </form>
  );
}
