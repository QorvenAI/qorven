'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { cn } from '@/lib/utils';
import { BarChart3 } from 'lucide-react';

export function SidebarGroupTitle({ children }: { children: React.ReactNode }) {
  return <div className="px-3 pt-4 pb-1"><span className="text-2xs font-medium text-muted-foreground/60 uppercase tracking-wider">{children}</span></div>;
}

export function SidebarMenuItem({ icon: Icon, label, badge, badgeColor, active, onClick }: {
  icon: typeof BarChart3; label: string | React.ReactNode; badge?: string; badgeColor?: string; active?: boolean; onClick?: () => void;
}) {
  return (
    <button onClick={onClick}
      className={cn('flex w-full items-center gap-2.5 h-8.5 px-2.5 rounded-md text-2sm transition-colors',
        active
          ? 'bg-accent text-foreground font-medium'
          : 'font-normal text-muted-foreground hover:bg-muted hover:text-foreground')}>
      <Icon className={cn('h-4 w-4 shrink-0', active ? 'opacity-80' : 'opacity-50')} />
      <span className="truncate flex-1 text-left">{label}</span>
      {badge && <span className={cn('text-2xs font-medium rounded px-1.5 py-0.5 ml-auto', badgeColor)}>{badge}</span>}
    </button>
  );
}

export function SidebarSeparator() {
  return <div className="border-t border-border my-2.5 mx-2.5" />;
}

export function SidebarDivider() {
  return <div className="my-3 mx-2.5 h-px bg-border/60" />;
}
