'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { cn } from '@/lib/utils';
import { Mail, Phone, Building2, CheckCircle, Circle, Clock } from 'lucide-react';

export function PipelineBlock({ title, stages }: { title?: string; stages: { name: string; count: number; value?: string; color?: string }[] }) {
  const total = stages.reduce((s, st) => s + st.count, 0) || 1;
  return (
    <div className="rounded-lg border border-border p-4">
      {title && <h3 className="text-sm font-medium mb-3">{title}</h3>}
      <div className="flex gap-1 mb-3 h-2 rounded-full overflow-hidden bg-muted">
        {stages.map((s, i) => <div key={i} className="h-full transition-all" style={{ width: `${(s.count / total) * 100}%`, background: s.color || `hsl(${i * 60}, 70%, 50%)` }} />)}
      </div>
      <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
        {stages.map((s, i) => (
          <div key={i} className="text-center">
            <div className="h-1.5 w-1.5 rounded-full mx-auto mb-1" style={{ background: s.color || `hsl(${i * 60}, 70%, 50%)` }} />
            <p className="text-lg font-semibold tabular-nums">{s.count}</p>
            <p className="text-2sm text-muted-foreground">{s.name}</p>
            {s.value && <p className="text-2xs text-muted-foreground">{s.value}</p>}
          </div>
        ))}
      </div>
    </div>
  );
}

export function ContactsBlock({ title, contacts }: { title?: string; contacts: { id: string; name: string; company?: string; email?: string; avatar?: string; lastContact?: string; status?: string }[] }) {
  return (
    <div className="rounded-lg border border-border">
      {title && <div className="px-4 py-3 border-b border-border bg-card"><h3 className="text-sm font-medium">{title}</h3></div>}
      <div className="divide-y divide-border">
        {contacts.map(c => (
          <div key={c.id} className="flex items-center gap-3 px-4 py-3 hover:bg-muted/20">
            <div className="h-9 w-9 rounded-full bg-primary/20 flex items-center justify-center text-xs font-semibold text-primary">{c.avatar || c.name.split(' ').map(n => n[0]).join('')}</div>
            <div className="flex-1 min-w-0">
              <p className="text-sm font-medium">{c.name}</p>
              <div className="flex items-center gap-2 text-2sm text-muted-foreground">
                {c.company && <span className="flex items-center gap-0.5"><Building2 className="h-2.5 w-2.5" />{c.company}</span>}
                {c.email && <span className="flex items-center gap-0.5"><Mail className="h-2.5 w-2.5" />{c.email}</span>}
              </div>
            </div>
            {c.lastContact && <span className="text-2xs text-muted-foreground">{c.lastContact}</span>}
            {c.status && <span className={cn('rounded-full px-2 py-0.5 text-2xs', c.status === 'active' ? 'bg-emerald-500/20 text-emerald-400' : 'bg-muted text-muted-foreground')}>{c.status}</span>}
          </div>
        ))}
      </div>
    </div>
  );
}

export function ProgressBlock({ title, steps }: { title?: string; steps: { label: string; status: 'done' | 'active' | 'pending' }[] }) {
  return (
    <div className="rounded-lg border border-border p-4">
      {title && <h3 className="text-sm font-medium mb-3">{title}</h3>}
      <div className="flex items-center gap-2">
        {steps.map((s, i) => (
          <div key={i} className="flex items-center gap-2 flex-1">
            <div className="flex items-center gap-1.5">
              {s.status === 'done' ? <CheckCircle className="h-4 w-4 text-emerald-400" /> : s.status === 'active' ? <Clock className="h-4 w-4 text-primary animate-pulse" /> : <Circle className="h-4 w-4 text-muted-foreground" />}
              <span className={cn('text-xs', s.status === 'done' ? 'text-emerald-400' : s.status === 'active' ? 'text-foreground font-medium' : 'text-muted-foreground')}>{s.label}</span>
            </div>
            {i < steps.length - 1 && <div className={cn('flex-1 h-px', s.status === 'done' ? 'bg-emerald-400' : 'bg-border')} />}
          </div>
        ))}
      </div>
    </div>
  );
}
