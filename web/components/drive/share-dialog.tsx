'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState } from 'react';
import { cn } from '@/lib/utils';
import { driveApi, type DriveScope } from '@/lib/api-workspace';
import { request } from '@/lib/api-core';
import { useStore } from '@/store';

export function ShareDialog({ fileId, current, onClose, onSaved }: {
  fileId: string;
  current?: DriveScope;
  onClose: () => void;
  onSaved: () => void;
}) {
  const souls = useStore((s) => s.souls);
  const [scope, setScope] = useState<DriveScope>(current ?? 'private');
  const [departments, setDepartments] = useState<{ id: string; name: string }[]>([]);
  const [deptId, setDeptId] = useState('');
  const [customAgent, setCustomAgent] = useState('');
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    request<{ departments: { id: string; name: string }[] }>('/budgets/departments')
      .then((d) => setDepartments(d.departments ?? [])).catch(() => setDepartments([]));
  }, []);

  const save = async () => {
    setSaving(true);
    try {
      await driveApi.setScope(fileId, scope, scope === 'department' ? deptId : null);
      if (scope === 'custom' && customAgent) {
        await driveApi.share(fileId, 'agent', customAgent, 'viewer');
      }
      onSaved();
      onClose();
    } finally {
      setSaving(false);
    }
  };

  const opt = (s: DriveScope, label: string, desc: string) => (
    <button
      key={s}
      onClick={() => setScope(s)}
      className={cn(
        'flex flex-col items-start rounded-md border px-3 py-2 text-left',
        scope === s ? 'border-primary bg-primary/5' : 'border-border hover:bg-accent',
      )}
    >
      <span className="text-xs font-medium">{label}</span>
      <span className="text-2xs text-muted-foreground">{desc}</span>
    </button>
  );

  return (
    <div
      className="fixed inset-0 z-[120] flex items-center justify-center bg-black/40"
      onClick={onClose}
    >
      <div
        role="dialog"
        className="w-[440px] rounded-xl border border-border bg-card p-5 shadow-lg"
        onClick={(e) => e.stopPropagation()}
      >
        <h2 className="mb-4 text-base font-semibold">Share</h2>

        <div className="mb-3 grid grid-cols-2 gap-2">
          {opt('private', 'Private', 'Only the owning agent')}
          {opt('company', 'Company', 'Everyone in the company')}
          {opt('department', 'Department', 'A department\'s agents')}
          {opt('custom', 'Custom', 'Specific agents')}
        </div>

        {scope === 'department' && (
          <select
            value={deptId}
            onChange={(e) => setDeptId(e.target.value)}
            className="qr-select mb-3 w-full"
          >
            <option value="">Select department...</option>
            {departments.map((d) => (
              <option key={d.id} value={d.id}>{d.name}</option>
            ))}
          </select>
        )}

        {scope === 'custom' && (
          <select
            value={customAgent}
            onChange={(e) => setCustomAgent(e.target.value)}
            className="qr-select mb-3 w-full"
          >
            <option value="">Grant an agent...</option>
            {souls.map((s) => (
              <option key={s.id} value={s.id}>{s.display_name}</option>
            ))}
          </select>
        )}

        <div className="flex justify-end gap-2">
          <button onClick={onClose} className="qr-btn qr-btn-ghost">
            Cancel
          </button>
          <button
            onClick={save}
            disabled={saving || (scope === 'department' && !deptId)}
            className="qr-btn qr-btn-primary"
          >
            {saving ? 'Saving...' : 'Save'}
          </button>
        </div>
      </div>
    </div>
  );
}
