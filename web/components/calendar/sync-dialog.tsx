'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState } from 'react';
import { calendarApi } from '@/lib/api-workspace';
import { request } from '@/lib/api-core';
import { useStore } from '@/store';

const PROVIDERS = [
  { id: 'google-calendar', name: 'Google Calendar' },
  { id: 'zoho-calendar', name: 'Zoho Calendar' },
];

const SCOPES = [
  { id: 'company', name: 'Company' },
  { id: 'department', name: 'Department' },
  { id: 'private', name: 'Private' },
];

export function SyncDialog({ scope: initialScope, onClose, onSaved }: {
  scope?: string;
  onClose: () => void;
  onSaved: () => void;
}) {
  // Coerce any non-standard scope value to 'company'
  const resolvedScope = (initialScope === 'private' || initialScope === 'department') ? initialScope : 'company';
  const souls = useStore((s) => s.souls);
  const [scope, setScope] = useState(resolvedScope);
  const [provider, setProvider] = useState(PROVIDERS[0]!.id);
  const [remoteCalendarID, setRemoteCalendarID] = useState('');
  const [departments, setDepartments] = useState<{ id: string; name: string }[]>([]);
  const [deptId, setDeptId] = useState('');
  const [ownerAgent, setOwnerAgent] = useState('');
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  // Close on Escape
  useEffect(() => {
    const handler = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose(); };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [onClose]);

  useEffect(() => {
    request<{ departments: { id: string; name: string }[] }>('/budgets/departments')
      .then((d) => setDepartments(d.departments ?? [])).catch(() => setDepartments([]));
  }, []);

  const save = async () => {
    // Department/private syncs match nothing without their target, so require it.
    if (scope === 'department' && !deptId) { setError('Select a department'); return; }
    if (scope === 'private' && !ownerAgent) { setError('Select an agent'); return; }
    setError('');
    setSaving(true);
    try {
      await calendarApi.createSync({
        scope,
        provider,
        remote_calendar_id: remoteCalendarID || undefined,
        scope_id: scope === 'department' ? deptId : undefined,
        owner_agent_id: scope === 'private' ? ownerAgent : undefined,
      });
      onSaved();
      onClose();
    } catch (e: any) {
      setError(e?.message ?? 'Failed to create sync');
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="fixed inset-0 z-[120] flex items-center justify-center bg-black/40" onClick={onClose}>
      <div role="dialog" className="w-[440px] rounded-xl border border-border bg-card p-5 shadow-lg" onClick={(e) => e.stopPropagation()}>
        <h2 className="mb-1 text-base font-semibold">Sync to calendar</h2>
        <p className="mb-4 text-xs text-muted-foreground">
          Push Qorven events to an external calendar provider. The provider must be connected in{' '}
          <span className="font-medium">Settings → Connections</span> first.
        </p>

        <label className="mb-1 block text-xs font-medium text-muted-foreground">Scope</label>
        <select value={scope} onChange={(e) => setScope(e.target.value)} className="qr-select mb-3 w-full">
          {SCOPES.map((s) => <option key={s.id} value={s.id}>{s.name}</option>)}
        </select>

        {scope === 'department' && (
          <select value={deptId} onChange={(e) => setDeptId(e.target.value)} className="qr-select mb-3 w-full">
            <option value="">Select department…</option>
            {departments.map((d) => <option key={d.id} value={d.id}>{d.name}</option>)}
          </select>
        )}
        {scope === 'private' && (
          <select value={ownerAgent} onChange={(e) => setOwnerAgent(e.target.value)} className="qr-select mb-3 w-full">
            <option value="">Select agent…</option>
            {souls.map((s) => <option key={s.id} value={s.id}>{s.display_name}</option>)}
          </select>
        )}

        <label className="mb-1 block text-xs font-medium text-muted-foreground">Provider</label>
        <select value={provider} onChange={(e) => setProvider(e.target.value)} className="qr-select mb-3 w-full">
          {PROVIDERS.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
        </select>

        <label className="mb-1 block text-xs font-medium text-muted-foreground">Remote calendar ID (optional)</label>
        <input
          value={remoteCalendarID}
          onChange={(e) => setRemoteCalendarID(e.target.value)}
          placeholder="Leave blank to use primary calendar"
          className="qr-input mb-4 w-full"
        />

        {error && <p className="mb-3 text-xs text-destructive">{error}</p>}

        <div className="flex justify-end gap-2">
          <button onClick={onClose} className="qr-btn qr-btn-ghost">Cancel</button>
          <button onClick={save} disabled={saving} className="qr-btn qr-btn-primary">
            {saving ? 'Saving…' : 'Create sync'}
          </button>
        </div>
      </div>
    </div>
  );
}
