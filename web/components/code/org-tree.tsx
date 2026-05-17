'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useState } from 'react';
import { ChevronRight, ChevronDown, User } from 'lucide-react';
import { cn } from '@/lib/utils';
import type { Soul } from '@/types';

interface OrgNode {
  soul: Soul;
  children: OrgNode[];
}

function buildTree(souls: Soul[]): OrgNode[] {
  const map = new Map<string, OrgNode>();
  souls.forEach(s => map.set(s.id, { soul: s, children: [] }));
  const roots: OrgNode[] = [];
  souls.forEach(s => {
    const node = map.get(s.id)!;
    if (s.manager_id && map.has(s.manager_id)) {
      map.get(s.manager_id)!.children.push(node);
    } else {
      roots.push(node);
    }
  });
  return roots;
}

function OrgNodeRow({ node, depth }: { node: OrgNode; depth: number }) {
  const [open, setOpen] = useState(depth < 2);
  const s = node.soul;
  const hasChildren = node.children.length > 0;

  return (
    <>
      <div
        className={cn('flex items-center gap-2 rounded-lg py-1.5 pr-3 transition-colors hover:bg-accent/50 cursor-default select-none')}
        style={{ paddingLeft: `${depth * 20 + 8}px` }}
      >
        {hasChildren ? (
          <button onClick={() => setOpen(!open)} className="flex items-center p-0.5 text-muted-foreground/60 hover:text-foreground">
            {open ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
          </button>
        ) : (
          <span className="w-5 shrink-0" />
        )}
        {s.avatar ? (
          <img src={s.avatar} alt="" className="h-7 w-7 rounded-full shrink-0 object-cover" />
        ) : (
          <div className="flex h-7 w-7 items-center justify-center rounded-full bg-primary/10 shrink-0">
            <User className="h-3.5 w-3.5 text-primary" />
          </div>
        )}
        <div className="min-w-0">
          <p className="text-sm font-medium leading-tight truncate">{s.display_name}</p>
          <p className="text-xs text-muted-foreground truncate">{s.title || s.role}</p>
        </div>
        <span className={cn('ml-auto shrink-0 rounded-full px-1.5 py-0.5 text-xs font-medium',
          s.status === 'active' ? 'bg-emerald-500/10 text-emerald-600' : 'bg-muted text-muted-foreground')}>
          {s.status}
        </span>
      </div>
      {open && node.children.map(c => <OrgNodeRow key={c.soul.id} node={c} depth={depth + 1} />)}
    </>
  );
}

export function OrgTree({ souls }: { souls: Soul[] }) {
  const roots = buildTree(souls);
  if (roots.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-16 gap-2 text-center">
        <User className="h-10 w-10 text-muted-foreground/20" />
        <p className="text-sm text-muted-foreground">No souls configured yet</p>
      </div>
    );
  }
  return (
    <div className="space-y-0.5 py-1">
      {roots.map(n => <OrgNodeRow key={n.soul.id} node={n} depth={0} />)}
    </div>
  );
}
