'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useState, useEffect, useCallback } from 'react';
import { Plus, Loader2, Target, CheckCircle2 } from 'lucide-react';
import { workGoals as goalsApi } from '@/lib/api';
import { GoalOutline } from '@/components/code/goal-outline';
import type { WorkGoalTreeNode } from '@/types';

export function GoalsTab() {
  const [goals, setGoals] = useState<WorkGoalTreeNode[]>([]);
  const [loading, setLoading] = useState(true);
  const [showCreate, setShowCreate] = useState(false);
  const [newTitle, setNewTitle] = useState('');
  const [newDesc, setNewDesc] = useState('');
  const [creating, setCreating] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const data = await goalsApi.tree();
      setGoals(data);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { load(); }, [load]);

  const totalGoals = goals.length + goals.reduce((n, g) => n + g.children.length, 0);
  const doneGoals = goals.filter(g => g.status === 'done').length
    + goals.reduce((n, g) => n + g.children.filter(c => c.status === 'done').length, 0);

  const createGoal = async () => {
    if (!newTitle.trim()) return;
    setCreating(true);
    try {
      await goalsApi.create({ title: newTitle.trim(), description: newDesc.trim() || undefined });
      setNewTitle('');
      setNewDesc('');
      setShowCreate(false);
      await load();
    } finally {
      setCreating(false);
    }
  };

  const toggleGoal = async (id: string, status: 'open' | 'done') => {
    try {
      await goalsApi.update(id, { status });
      await load();
    } catch { /* keep current state; server rejected */ }
  };

  return (
    <div className="flex flex-col h-full">
      {/* Mission banner */}
      <div className="shrink-0 border-b border-border bg-gradient-to-r from-primary/5 to-transparent px-6 py-5">
        <div className="flex items-center gap-3 mb-1">
          <Target className="h-5 w-5 text-primary" />
          <h2 className="text-lg font-semibold">Mission Goals</h2>
        </div>
        <p className="text-sm text-muted-foreground max-w-xl">
          Define what your organisation is building and why. Goals give agents context for autonomous ticket work.
        </p>
        {totalGoals > 0 && (
          <div className="flex items-center gap-3 mt-3">
            <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
              <CheckCircle2 className="h-3.5 w-3.5 text-emerald-500" />
              {doneGoals} / {totalGoals} complete
            </div>
            <div className="h-1.5 w-32 rounded-full bg-muted overflow-hidden">
              <div className="h-full rounded-full bg-primary transition-all"
                style={{ width: `${totalGoals > 0 ? Math.round(doneGoals / totalGoals * 100) : 0}%` }} />
            </div>
          </div>
        )}
      </div>

      {/* Toolbar */}
      <div className="flex shrink-0 items-center gap-2 border-b border-border px-4 py-2.5">
        <span className="flex-1 text-xs text-muted-foreground">{totalGoals} goal{totalGoals !== 1 ? 's' : ''}</span>
        <button
          onClick={() => setShowCreate(!showCreate)}
          className="flex items-center gap-1.5 rounded-lg bg-primary px-3 py-1.5 text-xs font-semibold text-primary-foreground hover:bg-primary/90 transition-colors"
        >
          <Plus className="h-3.5 w-3.5" />
          Add goal
        </button>
      </div>

      {/* Create form */}
      {showCreate && (
        <div className="shrink-0 border-b border-border bg-muted/20 px-4 py-3 space-y-2">
          <input
            autoFocus
            value={newTitle}
            onChange={e => setNewTitle(e.target.value)}
            onKeyDown={e => { if (e.key === 'Enter' && !e.shiftKey) createGoal(); if (e.key === 'Escape') setShowCreate(false); }}
            placeholder="Goal title…"
            className="qr-input" />
          <textarea
            value={newDesc}
            onChange={e => setNewDesc(e.target.value)}
            placeholder="Description (optional)…"
            rows={2}
            className="qr-textarea resize-none" />
          <div className="flex items-center gap-2">
            <button onClick={createGoal} disabled={!newTitle.trim() || creating}
              className="flex items-center gap-1.5 rounded-lg bg-primary px-3 py-1.5 text-xs font-semibold text-primary-foreground hover:bg-primary/90 disabled:opacity-50 transition-colors">
              {creating ? <Loader2 className="h-3 w-3 animate-spin" /> : <Plus className="h-3 w-3" />}
              Create
            </button>
            <button onClick={() => setShowCreate(false)} className="text-xs text-muted-foreground hover:text-foreground">
              Cancel
            </button>
          </div>
        </div>
      )}

      {/* Outline */}
      <div className="flex-1 overflow-y-auto px-4 py-3">
        {loading ? (
          <div className="flex items-center justify-center py-20">
            <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
          </div>
        ) : (
          <GoalOutline goals={goals} onToggle={toggleGoal} />
        )}
      </div>
    </div>
  );
}
