'use client';
// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useState } from 'react';
import { cn } from '@/lib/utils';
import { calendarApi } from '@/lib/api-workspace';
import { useStore } from '@/store';

const RECURRENCE_PRESETS: { label: string; expr: string }[] = [
  { label: 'Every day at 9am', expr: '0 9 * * *' },
  { label: 'Every Monday at 9am', expr: '0 9 * * 1' },
  { label: 'Every hour', expr: '0 * * * *' },
  { label: 'First of month at 9am', expr: '0 9 1 * *' },
];

export function ScheduleDialog({ onClose, onCreated, defaultAgentId }: { onClose: () => void; onCreated: () => void; defaultAgentId?: string | null }) {
  const souls = useStore((s) => s.souls);
  const firstSoulId = souls.find(() => true)?.id ?? '';
  const [agentId, setAgentId] = useState<string>(defaultAgentId != null ? defaultAgentId : firstSoulId);
  const [instruction, setInstruction] = useState('');
  const [mode, setMode] = useState<'once' | 'repeat'>('once');
  const [when, setWhen] = useState('');
  const [expr, setExpr] = useState<string>(RECURRENCE_PRESETS[0]?.expr ?? '0 9 * * *');
  const [saving, setSaving] = useState(false);

  const submit = async () => {
    if (!agentId || !instruction) return;
    setSaving(true);
    try {
      await calendarApi.schedule({
        agent_id: agentId,
        instruction,
        mode,
        when: mode === 'once' && when ? new Date(when).toISOString() : undefined,
        cron_expression: mode === 'repeat' ? expr : undefined,
      });
      onCreated();
      onClose();
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="fixed inset-0 z-[120] flex items-center justify-center bg-black/40" onClick={onClose}>
      <div role="dialog" className="w-[440px] rounded-xl border border-border bg-card p-5 shadow-lg" onClick={(e) => e.stopPropagation()}>
        <h2 className="mb-4 text-base font-semibold">Schedule a task</h2>

        <label className="mb-1 block text-xs font-medium text-muted-foreground">Agent</label>
        <select value={agentId} onChange={(e) => setAgentId(e.target.value)} className="qr-select mb-3 w-full">
          {souls.map((s) => <option key={s.id} value={s.id}>{s.display_name}</option>)}
        </select>

        <label className="mb-1 block text-xs font-medium text-muted-foreground">Instruction</label>
        <textarea value={instruction} onChange={(e) => setInstruction(e.target.value)} placeholder="e.g. Email the weekly pipeline summary to sales@" className="qr-textarea mb-3 w-full" rows={3} />

        <div className="mb-3 inline-flex overflow-hidden rounded-md border border-border">
          <button onClick={() => setMode('once')} className={cn('px-3 py-1.5 text-xs font-medium', mode === 'once' ? 'bg-primary text-primary-foreground' : 'text-muted-foreground hover:bg-accent')}>Once</button>
          <button onClick={() => setMode('repeat')} className={cn('px-3 py-1.5 text-xs font-medium', mode === 'repeat' ? 'bg-primary text-primary-foreground' : 'text-muted-foreground hover:bg-accent')}>Repeat</button>
        </div>

        {mode === 'once' ? (
          <input type="datetime-local" value={when} onChange={(e) => setWhen(e.target.value)} className="qr-input mb-4 w-full" />
        ) : (
          <select value={expr} onChange={(e) => setExpr(e.target.value)} className="qr-select mb-4 w-full">
            {RECURRENCE_PRESETS.map((p) => <option key={p.expr} value={p.expr}>{p.label}</option>)}
          </select>
        )}

        <div className="flex justify-end gap-2">
          <button onClick={onClose} className="qr-btn qr-btn-ghost">Cancel</button>
          <button onClick={submit} disabled={!agentId || !instruction || saving} className="qr-btn qr-btn-primary">{saving ? 'Scheduling…' : 'Schedule'}</button>
        </div>
      </div>
    </div>
  );
}
