'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useState, useMemo } from 'react';
import { useStore } from '@/store';
import { cn } from '@/lib/utils';
import { soulGradient } from '@/components/soul-card';
import { ActivityPanel } from '@/components/activity-pulse';
import { ProviderDetailPanel } from '@/components/settings/provider-detail-panel';
import { modelDisplayName } from '@/lib/model-names';
import { X } from 'lucide-react';
import type { Soul } from '@/types';

type PanelTab = 'profile' | 'activity' | 'info';

export function ContextPanel() {
  const open = useStore((s) => s.contextPanelOpen);
  const content = useStore((s) => s.contextPanelContent);
  const close = useStore((s) => s.closeContextPanel);
  const [tab, setTab] = useState<PanelTab>('profile');

  if (!open || !content) return null;

  if (content.type === 'soul') {
    return <SoulProfile soul={content.data as Soul} tab={tab} setTab={setTab} onClose={close} />;
  }

  if (content.type === 'activity') {
    return (
      <div className="context-panel fixed top-0 right-0 bottom-0 z-20 border-s border-border bg-background" style={{ width: 'var(--context-panel-width)' }}>
        <ActivityPanel />
      </div>
    );
  }

  if (content.type === 'provider') {
    return (
      <div className="context-panel fixed top-0 right-0 bottom-0 z-20 border-s border-border bg-background" style={{ width: 'var(--context-panel-width)' }}>
        <ProviderDetailPanel provider={content.data} onClose={close} onConnected={() => {}} />
      </div>
    );
  }

  // Fallback for other content types
  return (
    <div className="context-panel fixed top-0 right-0 bottom-0 z-20 border-s border-border bg-background" style={{ width: 'var(--context-panel-width)' }}>
      <div className="flex items-center justify-between border-b border-border px-4 h-[var(--header-height)]">
        <h3 className="text-sm font-medium">{content.type}</h3>
        <button onClick={close} className="flex h-7 w-7 items-center justify-center rounded-md text-muted-foreground hover:bg-accent"><X className="h-4 w-4" /></button>
      </div>
      <div className="overflow-y-auto p-4 text-sm">
        <pre className="whitespace-pre-wrap text-2sm text-muted-foreground">{JSON.stringify(content.data, null, 2)}</pre>
      </div>
    </div>
  );
}

function SoulProfile({ soul, tab, setTab, onClose }: { soul: Soul; tab: PanelTab; setTab: (t: PanelTab) => void; onClose: () => void }) {
  const soulState = useStore((s) => s.soulStates[soul.id]);
  const liveEvents = useStore((s) => s.liveEvents);
  const agentEvents = useMemo(() => liveEvents.filter((e) => e.agent_id === soul.id).slice(0, 15), [liveEvents, soul.id]);
  const activity = soulState?.activity ?? 'idle';

  const pct = soul.credit_budget_cents > 0 ? (soul.credit_used_cents / soul.credit_budget_cents) * 100 : 0;

  return (
    <div className="context-panel fixed top-0 right-0 bottom-0 z-20 border-s border-border bg-background" style={{ width: 'var(--context-panel-width)' }}>
      {/* Header */}
      <div className="flex items-center justify-between border-b border-border px-4 h-[var(--header-height)]">
        <span className="text-sm font-medium">Qor Profile</span>
        <button onClick={onClose} className="flex h-7 w-7 items-center justify-center rounded-md text-muted-foreground hover:bg-accent"><X className="h-4 w-4" /></button>
      </div>

      {/* Qor card */}
      <div className="flex flex-col items-center py-5 border-b border-border">
        <div className={cn('flex h-16 w-16 items-center justify-center rounded-full bg-gradient-to-br text-xl font-semibold text-white', soulGradient(soul.display_name))}>
          {soul.display_name.charAt(0)}
        </div>
        <p className="mt-2 text-sm font-semibold">{soul.display_name}</p>
        <p className="text-xs text-muted-foreground">{soul.title || soul.role} · {modelDisplayName(soul.model)}</p>
        <p className={cn('mt-1 text-xs', activity === 'thinking' ? 'text-amber-400' : 'text-emerald-400')}>
          {activity === 'thinking' ? '●●● Thinking…' : activity === 'running' ? '▶ Running' : '🟢 Online'}
        </p>
      </div>

      {/* Tabs */}
      <div className="flex border-b border-border">
        {(['profile', 'activity', 'info'] as PanelTab[]).map((t) => (
          <button key={t} onClick={() => setTab(t)}
            className={cn('flex-1 py-2 text-xs font-medium text-center transition-colors',
              tab === t ? 'text-foreground border-b-2 border-primary' : 'text-muted-foreground hover:text-foreground')}>
            {t.charAt(0).toUpperCase() + t.slice(1)}
          </button>
        ))}
      </div>

      {/* Tab content */}
      <div className="overflow-y-auto p-4 text-sm space-y-3">
        {tab === 'profile' && (
          <>
            <Row label="Role" value={soul.role || '—'} />
            <Row label="Title" value={soul.title || '—'} />
            <Row label="Memory" value={soul.memory_enabled ? 'Enabled' : 'Disabled'} />
            <Row label="Tools" value={soul.tool_profile || 'full'} />
            <Row label="Skills" value={`${soul.skills?.length ?? 0} installed`} />
            <a href={`/qors/${soul.id}`} className="block mt-4 text-xs text-primary hover:underline text-center">Open Full Config →</a>
          </>
        )}
        {tab === 'activity' && (
          <>
            <Row label="Tokens today" value={soulState?.tokensToday?.toLocaleString() ?? '0'} />
            <Row label="Last event" value={soulState?.lastEvent ?? 'None'} />
            {agentEvents.length > 0 && (
              <div className="mt-3 space-y-1">
                {agentEvents.map((e) => (
                  <div key={e.id} className="flex items-center gap-2 text-2xs">
                    <span className="text-muted-foreground w-14 shrink-0">{new Date(e.timestamp).toLocaleTimeString()}</span>
                    <span className="truncate">{e.detail || e.type}</span>
                  </div>
                ))}
              </div>
            )}
          </>
        )}
        {tab === 'info' && (
          <>
            <Row label="Model" value={modelDisplayName(soul.model)} />
            <Row label="Temperature" value={String(soul.temperature)} />
            <Row label="Context window" value={`${(soul.context_window / 1000).toFixed(0)}K`} />
            {soul.credit_budget_cents > 0 && (
              <div>
                <p className="text-2xs text-muted-foreground mb-1">Budget</p>
                <div className="h-2 rounded-full bg-muted overflow-hidden">
                  <div className={cn('h-full rounded-full', pct > 80 ? 'bg-destructive' : 'bg-primary')} style={{ width: `${Math.min(pct, 100)}%` }} />
                </div>
                <p className="text-2xs text-muted-foreground mt-0.5">${(soul.credit_used_cents / 100).toFixed(2)} / ${(soul.credit_budget_cents / 100).toFixed(2)}</p>
              </div>
            )}
            {soul.system_prompt && (
              <div>
                <p className="text-2xs text-muted-foreground mb-1">System prompt</p>
                <pre className="rounded-lg bg-muted px-2.5 py-2 text-2sm max-h-40 overflow-y-auto whitespace-pre-wrap">{soul.system_prompt.slice(0, 500)}</pre>
              </div>
            )}
          </>
        )}
      </div>
    </div>
  );
}

function Row({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between">
      <span className="text-2xs text-muted-foreground">{label}</span>
      <span className="text-xs font-medium">{value}</span>
    </div>
  );
}
