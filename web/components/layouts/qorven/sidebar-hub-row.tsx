'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { Hash } from 'lucide-react';
import { cn } from '@/lib/utils';

// ── Gradient matches soul-card.tsx / sidebar-agent-row.tsx ─────────────────
const GRADIENTS = [
  'from-primary to-primary/80', 'from-emerald-500 to-teal-600',
  'from-orange-500 to-red-600', 'from-pink-500 to-rose-600',
  'from-cyan-500 to-blue-600', 'from-amber-500 to-yellow-600',
  'from-fuchsia-500 to-purple-600', 'from-lime-500 to-green-600',
];
function gradientFor(id: string): string {
  let hash = 0;
  for (let i = 0; i < id.length; i++) hash = id.charCodeAt(i) + ((hash << 5) - hash);
  return GRADIENTS[Math.abs(hash) % GRADIENTS.length]!;
}

export interface HubMember {
  id: string;
  display_name: string;
  avatar?: string;
}

export interface SidebarHubRowProps {
  id: string;
  name: string;
  displayName?: string;
  members?: HubMember[];
  messageCount?: number;
  isActive: boolean;
  onClick: () => void;
}

export function SidebarHubRow({
  id,
  name,
  displayName,
  members = [],
  messageCount,
  isActive,
  onClick,
}: SidebarHubRowProps) {
  const label = displayName || name;
  const shown = members.slice(0, 3); // show max 3 avatars stacked
  const overflow = members.length - 3;

  return (
    <button
      onClick={onClick}
      className={cn(
        'group/hub flex w-full items-center gap-2.5 rounded-lg px-2 py-1.5 text-left transition-colors',
        isActive
          ? 'bg-accent text-foreground'
          : 'text-muted-foreground hover:bg-muted/60 hover:text-foreground',
      )}
    >
      {/* Hub icon OR member stack */}
      {shown.length === 0 ? (
        <div className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
          <Hash className="h-3.5 w-3.5" />
        </div>
      ) : (
        // Stacked avatar pile
        <div className="relative flex h-7 shrink-0 items-center" style={{ width: Math.min(shown.length, 3) * 16 + 8 }}>
          {shown.map((m, i) => (
            <div
              key={m.id}
              className={cn(
                'absolute flex h-6 w-6 items-center justify-center rounded-full ring-2 ring-muted bg-gradient-to-br text-[9px] font-bold text-white overflow-hidden',
                gradientFor(m.id),
              )}
              style={{ left: i * 14, zIndex: i + 1 }}
            >
              {m.avatar ? (
                <img src={m.avatar} alt={m.display_name} className="h-full w-full object-cover" />
              ) : (
                (m.display_name?.[0] ?? '?').toUpperCase()
              )}
            </div>
          ))}
          {overflow > 0 && (
            <div
              className="absolute flex h-6 w-6 items-center justify-center rounded-full ring-2 ring-muted bg-muted text-[9px] font-medium text-muted-foreground"
              style={{ left: 3 * 14, zIndex: shown.length + 1 }}
            >
              +{overflow}
            </div>
          )}
        </div>
      )}

      {/* Hub name */}
      <span className="flex-1 truncate text-[13px] font-medium leading-tight">{label}</span>

      {/* Message count badge */}
      {messageCount != null && messageCount > 0 && (
        <span className="shrink-0 rounded-full bg-primary/15 px-1.5 py-0.5 text-[10px] font-medium text-primary">
          {messageCount > 99 ? '99+' : messageCount}
        </span>
      )}
    </button>
  );
}
