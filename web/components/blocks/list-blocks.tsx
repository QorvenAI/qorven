'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { cn } from '@/lib/utils';

export function ListBlock({ title, items }: { title?: string; items: { id: string; title: string; subtitle?: string; avatar?: string; badge?: string; badgeColor?: string }[] }) {
  return (
    <div className="rounded-lg border border-border">
      {title && <div className="px-4 py-3 border-b border-border bg-card"><h3 className="text-sm font-medium">{title}</h3></div>}
      <div className="divide-y divide-border">
        {items.map(item => (
          <div key={item.id} className="flex items-center gap-3 px-4 py-3 hover:bg-muted/20">
            {item.avatar && <div className="h-8 w-8 rounded-full bg-muted flex items-center justify-center text-xs font-medium">{item.avatar}</div>}
            <div className="flex-1 min-w-0">
              <p className="text-sm font-medium truncate">{item.title}</p>
              {item.subtitle && <p className="text-xs text-muted-foreground truncate">{item.subtitle}</p>}
            </div>
            {item.badge && <span className={cn('rounded-full px-2 py-0.5 text-2xs font-medium', item.badgeColor || 'bg-muted text-muted-foreground')}>{item.badge}</span>}
          </div>
        ))}
      </div>
    </div>
  );
}

export function FeedBlock({ title, items }: { title?: string; items: { id: string; actor: string; action: string; target?: string; time: string; avatar?: string }[] }) {
  return (
    <div className="rounded-lg border border-border">
      {title && <div className="px-4 py-3 border-b border-border bg-card"><h3 className="text-sm font-medium">{title}</h3></div>}
      <div className="divide-y divide-border">
        {items.map(item => (
          <div key={item.id} className="flex gap-3 px-4 py-3">
            <div className="h-7 w-7 rounded-full bg-primary/20 flex items-center justify-center text-2xs font-semibold text-primary shrink-0">{item.avatar || item.actor[0]}</div>
            <div className="flex-1 min-w-0">
              <p className="text-sm"><span className="font-medium">{item.actor}</span> {item.action}{item.target && <> <span className="font-medium">{item.target}</span></>}</p>
              <p className="text-2sm text-muted-foreground mt-0.5">{item.time}</p>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

export function TimelineBlock({ title, events }: { title?: string; events: { date: string; title: string; description?: string; color?: string }[] }) {
  return (
    <div className="rounded-lg border border-border p-4">
      {title && <h3 className="text-sm font-medium mb-4">{title}</h3>}
      <div className="space-y-4">
        {events.map((e, i) => (
          <div key={i} className="flex gap-3">
            <div className="flex flex-col items-center">
              <div className="h-2.5 w-2.5 rounded-full shrink-0 mt-1.5" style={{ background: e.color || '#a3e635' }} />
              {i < events.length - 1 && <div className="w-px flex-1 bg-border mt-1" />}
            </div>
            <div className="pb-4">
              <p className="text-xs text-muted-foreground">{e.date}</p>
              <p className="text-sm font-medium">{e.title}</p>
              {e.description && <p className="text-xs text-muted-foreground mt-0.5">{e.description}</p>}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
