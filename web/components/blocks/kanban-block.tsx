'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { cn } from '@/lib/utils';

interface KanbanProps {
  title?: string;
  columns: { id: string; title: string; color?: string }[];
  items: { id: string; columnId: string; title: string; description?: string; tags?: string[]; avatar?: string }[];
}

export function KanbanBoard({ title, columns, items }: KanbanProps) {
  return (
    <div className="rounded-lg border border-border p-4">
      {title && <h3 className="text-sm font-medium mb-3">{title}</h3>}
      <div className="flex gap-3 overflow-x-auto pb-2">
        {columns.map(col => {
          const colItems = items.filter(i => i.columnId === col.id);
          return (
            <div key={col.id} className="min-w-[240px] flex-1">
              <div className="flex items-center gap-2 mb-2">
                <div className="h-2 w-2 rounded-full" style={{ background: col.color || '#6b7280' }} />
                <span className="text-xs font-medium">{col.title}</span>
                <span className="text-2xs text-muted-foreground ml-auto">{colItems.length}</span>
              </div>
              <div className="space-y-2">
                {colItems.map(item => (
                  <div key={item.id} className="rounded-lg border border-border bg-card p-3 hover:border-primary/30 cursor-pointer transition-colors">
                    <p className="text-sm font-medium">{item.title}</p>
                    {item.description && <p className="text-xs text-muted-foreground mt-1 line-clamp-2">{item.description}</p>}
                    {item.tags && item.tags.length > 0 && (
                      <div className="flex gap-1 mt-2">{item.tags.map(t => <span key={t} className="rounded bg-muted px-1.5 py-0.5 text-2xs">{t}</span>)}</div>
                    )}
                  </div>
                ))}
                {colItems.length === 0 && <div className="rounded-lg border border-dashed border-border p-4 text-center text-xs text-muted-foreground">Empty</div>}
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}
