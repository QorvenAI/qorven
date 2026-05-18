'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { User } from 'lucide-react';
import { cn } from '@/lib/utils';
import type { Soul } from '@/types';

function SoulCard({ soul }: { soul: Soul }) {
  return (
    <div className="flex flex-col items-center gap-2 rounded-xl border border-border bg-card p-4 text-center hover:border-primary/30 hover:shadow-sm transition-all">
      {soul.avatar ? (
        <img src={soul.avatar} alt="" className="h-12 w-12 rounded-full object-cover" />
      ) : (
        <div className="flex h-12 w-12 items-center justify-center rounded-full bg-primary/10">
          <User className="h-6 w-6 text-primary" />
        </div>
      )}
      <div className="min-w-0 w-full">
        <p className="text-sm font-semibold truncate">{soul.display_name}</p>
        <p className="text-xs text-muted-foreground truncate">{soul.title || soul.role}</p>
      </div>
      <span className={cn('rounded-full px-2 py-0.5 text-xs font-medium',
        soul.status === 'active' ? 'bg-emerald-500/10 text-emerald-600' : 'bg-muted text-muted-foreground')}>
        {soul.status}
      </span>
    </div>
  );
}

export function OrgGrid({ souls }: { souls: Soul[] }) {
  const byRole = souls.reduce<Record<string, Soul[]>>((acc, s) => {
    const key = s.role || 'Other';
    (acc[key] ||= []).push(s);
    return acc;
  }, {});

  if (souls.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-16 gap-2 text-center">
        <User className="h-10 w-10 text-muted-foreground/20" />
        <p className="text-sm text-muted-foreground">No souls configured yet</p>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {Object.entries(byRole).map(([role, list]) => (
        <div key={role}>
          <h3 className="mb-3 text-xs font-semibold uppercase tracking-wider text-muted-foreground">{role}</h3>
          <div className="grid gap-3 grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5">
            {list.map(s => <SoulCard key={s.id} soul={s} />)}
          </div>
        </div>
      ))}
    </div>
  );
}
