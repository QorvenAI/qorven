'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useState, useEffect } from 'react';
import { Users, Plus, X, Loader2, ChevronDown, ChevronUp } from 'lucide-react';
import { CanvasHeader } from '@/components/layouts/canvas-header';
import { EmptyState, emptyStates } from '@/components/empty-state';
import { ErrorBoundary } from '@/components/error-boundary';
import { cn } from '@/lib/utils';
import { toast } from 'sonner';
import { teamsApi } from '@/lib/api';

type Team = { id: string; name: string; member_count?: number; active_task_count?: number; status?: string };
type Member = { id: string; name: string; role?: string };

export default function TeamsPage() {
  const [teams, setTeams] = useState<Team[]>([]);
  const [loading, setLoading] = useState(true);
  const [showForm, setShowForm] = useState(false);
  const [name, setName] = useState('');
  const [saving, setSaving] = useState(false);
  const [selectedTeam, setSelectedTeam] = useState<string | null>(null);
  const [members, setMembers] = useState<Record<string, Member[]>>({});
  const [membersLoading, setMembersLoading] = useState<string | null>(null);

  const load = () => {
    setLoading(true);
    teamsApi.list()
      .then(d => { setTeams(Array.isArray(d) ? d : []); setLoading(false); })
      .catch(() => setLoading(false));
  };

  useEffect(load, []);

  const createTeam = async () => {
    if (!name.trim()) return;
    setSaving(true);
    try {
      await teamsApi.create({ name });
      toast.success('Team created');
      setName('');
      setShowForm(false);
      load();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Failed to create team');
    } finally {
      setSaving(false);
    }
  };

  const loadMembers = async (id: string) => {
    if (selectedTeam === id) { setSelectedTeam(null); return; }
    setSelectedTeam(id);
    if (members[id]) return;
    setMembersLoading(id);
    try {
      const d = await teamsApi.members(id);
      setMembers(prev => ({ ...prev, [id]: Array.isArray(d) ? d : [] }));
    } catch {
      setMembers(prev => ({ ...prev, [id]: [] }));
    } finally {
      setMembersLoading(null);
    }
  };

  return (
    <ErrorBoundary fallbackTitle="Failed to load teams">
      <div className="space-y-6">
        <CanvasHeader
          title="Teams"
          description={`${teams.length} team${teams.length !== 1 ? 's' : ''}`}
          actions={
            <button onClick={() => setShowForm(true)}
              className="inline-flex items-center gap-2 rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 cursor-pointer">
              <Plus className="h-4 w-4" /> Create Team
            </button>
          }
        />

        {showForm && (
          <div className="rounded-xl border border-border bg-card p-4 flex gap-3 items-center">
            <input value={name} onChange={e => setName(e.target.value)} placeholder="Team name"
              autoFocus onKeyDown={e => e.key === 'Enter' && createTeam()}
              className="qr-input flex-1" />
            <button onClick={createTeam} disabled={!name.trim() || saving}
              className="rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50 cursor-pointer">
              {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : 'Save'}
            </button>
            <button onClick={() => { setShowForm(false); setName(''); }}
              className="text-muted-foreground hover:text-foreground cursor-pointer">
              <X className="h-4 w-4" />
            </button>
          </div>
        )}

        {loading ? (
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {Array.from({ length: 3 }).map((_, i) => (
              <div key={i} className="rounded-xl border border-border bg-card p-5 space-y-3">
                <div className="h-5 w-28 animate-pulse rounded bg-muted" />
                <div className="h-4 w-20 animate-pulse rounded bg-muted" />
              </div>
            ))}
          </div>
        ) : teams.length === 0 ? (
          <EmptyState
            icon={emptyStates.teams?.icon ?? Users}
            title="No teams yet"
            description="Create a team to organise agents into groups with shared goals."
            actionLabel="Create Team"
            onAction={() => setShowForm(true)}
          />
        ) : (
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {teams.map(t => (
              <div key={t.id}
                className="rounded-xl border border-border bg-card p-5 cursor-pointer hover:border-primary/30 transition-colors"
                onClick={() => loadMembers(t.id)}>
                <div className="flex items-center justify-between mb-3">
                  <h3 className="font-semibold">{t.name}</h3>
                  <div className="flex items-center gap-1.5">
                    <span className={cn('text-xs px-2 py-0.5 rounded-full',
                      t.status === 'active' ? 'bg-emerald-500/10 text-emerald-500' : 'bg-muted text-muted-foreground')}>
                      {t.status ?? 'active'}
                    </span>
                    {selectedTeam === t.id
                      ? <ChevronUp className="h-3.5 w-3.5 text-muted-foreground" />
                      : <ChevronDown className="h-3.5 w-3.5 text-muted-foreground" />}
                  </div>
                </div>
                <div className="flex gap-4 text-sm text-muted-foreground">
                  <span>{t.member_count ?? 0} members</span>
                  <span>{t.active_task_count ?? 0} active tasks</span>
                </div>

                {selectedTeam === t.id && (
                  <div className="mt-4 pt-4 border-t border-border" onClick={e => e.stopPropagation()}>
                    {membersLoading === t.id ? (
                      <div className="flex items-center gap-2 text-sm text-muted-foreground">
                        <Loader2 className="h-3.5 w-3.5 animate-spin" /> Loading members…
                      </div>
                    ) : (members[t.id] ?? []).length === 0 ? (
                      <p className="text-sm text-muted-foreground">No members</p>
                    ) : (
                      <ul className="space-y-1.5">
                        {(members[t.id] ?? []).map(m => (
                          <li key={m.id} className="flex items-center justify-between text-sm">
                            <span>{m.name}</span>
                            <span className="text-xs text-muted-foreground">{m.role ?? 'member'}</span>
                          </li>
                        ))}
                      </ul>
                    )}
                  </div>
                )}
              </div>
            ))}
          </div>
        )}
      </div>
    </ErrorBoundary>
  );
}
