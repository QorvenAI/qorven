'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useCallback } from 'react';
import { motion } from 'motion/react';
import { Headphones } from 'lucide-react';
import { cn } from '@/lib/utils';
import { SoulPulseRing } from '@/components/soul-pulse-ring';
import { useVoiceEnabled } from '@/hooks/use-voice-enabled';
import { useStore } from '@/store';
import type { Soul, SoulActivity } from '@/types';
import { agentVoiceRegistry } from '@/lib/voice-registry';

// ── Gradient map matches soul-card.tsx ──────────────────────────────────────
const GRADIENTS = [
  'from-primary to-primary/80',
  'from-emerald-500 to-teal-600',
  'from-orange-500 to-red-600',
  'from-pink-500 to-rose-600',
  'from-cyan-500 to-blue-600',
  'from-amber-500 to-yellow-600',
  'from-fuchsia-500 to-purple-600',
  'from-lime-500 to-green-600',
];

function gradientFor(id: string): string {
  let hash = 0;
  for (let i = 0; i < id.length; i++) hash = id.charCodeAt(i) + ((hash << 5) - hash);
  return GRADIENTS[Math.abs(hash) % GRADIENTS.length]!;
}

// ── Waveform bars — shown when voice is active for this agent ─────────────
function Waveform() {
  return (
    <span className="inline-flex items-end gap-[2px] h-3 ml-1 shrink-0">
      {[0, 1, 2].map((i) => (
        <motion.span
          key={i}
          className="w-[3px] rounded-full bg-primary"
          animate={{ height: ['4px', '10px', '4px'] }}
          transition={{
            duration: 0.7,
            repeat: Infinity,
            delay: i * 0.15,
            ease: 'easeInOut',
          }}
        />
      ))}
    </span>
  );
}

// ── Activity subtitle text ────────────────────────────────────────────────
function activitySubtitle(
  activity: SoulActivity,
  lastEvent: string | undefined,
  fallback: string,
): string {
  if (activity === 'thinking' || activity === 'running') {
    const ev = lastEvent?.trim();
    if (ev) return ev.length > 42 ? ev.slice(0, 40) + '…' : ev;
    return activity === 'thinking' ? 'Thinking…' : 'Working…';
  }
  if (activity === 'error') return 'Error';
  if (activity === 'offline') return 'Offline';
  // idle
  const ev = lastEvent?.trim();
  if (ev) return ev.length > 42 ? ev.slice(0, 40) + '…' : ev;
  return fallback;
}

// ── Props ─────────────────────────────────────────────────────────────────
export interface SidebarAgentRowProps {
  soul: Soul;
  activity: SoulActivity;
  lastEvent?: string;
  isActive: boolean; // currently navigated to this agent
  onClick: () => void;
}

// ── Component ─────────────────────────────────────────────────────────────
export function SidebarAgentRow({
  soul,
  activity,
  lastEvent,
  isActive,
  onClick,
}: SidebarAgentRowProps) {
  const { enabled: voiceEnabled } = useVoiceEnabled();
  const activeVoiceAgentId = useStore((s) => s.activeVoiceAgentId);
  const setActiveVoiceAgent = useStore((s) => s.setActiveVoiceAgent);

  const isVoiceActive = activeVoiceAgentId === soul.id;

  // Sidebar rows do NOT own a VAD instance — that would mount useMicVAD once
  // per agent row (12+ simultaneous ONNX model loads → race condition errors).
  // Instead, the row delegates to the single AgentVoicePill instance via the
  // agentVoiceRegistry. The Pill holds the one shared useMicVAD instance.
  const handleVoiceClick = useCallback(
    async (e: React.MouseEvent) => {
      e.stopPropagation();
      if (!voiceEnabled) return;

      if (isVoiceActive) {
        // Stop current agent — call the Pill's registered stop fn
        const stop = agentVoiceRegistry.get(soul.id);
        if (stop) await stop();
        setActiveVoiceAgent(null);
        return;
      }

      // Stop previously active agent first
      if (activeVoiceAgentId) {
        const prevStop = agentVoiceRegistry.get(activeVoiceAgentId);
        if (prevStop) await prevStop();
        setActiveVoiceAgent(null);
        await new Promise((r) => setTimeout(r, 80));
      }

      // Switch the Pill to this agent by updating the store.
      // The AgentVoicePill watches activeVoiceAgentId and starts the session.
      setActiveVoiceAgent(soul.id);
    },
    [isVoiceActive, voiceEnabled, activeVoiceAgentId, setActiveVoiceAgent, soul.id],
  );

  const subtitle = activitySubtitle(
    activity,
    lastEvent,
    soul.title || soul.role || soul.org_role || 'Agent',
  );

  const gradient = gradientFor(soul.id);

  return (
    <div
      role="button"
      tabIndex={0}
      onClick={onClick}
      onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); onClick(); } }}
      className={cn(
        'group/row relative flex w-full cursor-pointer items-center gap-2.5 rounded-lg px-2 py-1.5 text-left transition-colors',
        isActive
          ? 'bg-accent text-foreground'
          : 'text-muted-foreground hover:bg-muted/60 hover:text-foreground',
        isVoiceActive && 'ring-1 ring-primary/30',
      )}
    >
      {/* Avatar with pulse overlay */}
      <div className="relative shrink-0">
        {soul.avatar ? (
          <img
            src={soul.avatar}
            alt={soul.display_name}
            className="h-8 w-8 rounded-full object-cover"
          />
        ) : (
          <div
            className={cn(
              'flex h-8 w-8 items-center justify-center rounded-full bg-gradient-to-br text-[11px] font-bold text-white',
              gradient,
            )}
          >
            {(soul.display_name?.[0] ?? '?').toUpperCase()}
          </div>
        )}
        {/* Status dot — bottom-right of avatar */}
        <span className="absolute -bottom-0.5 -right-0.5">
          <SoulPulseRing activity={activity} size="sm" />
        </span>
      </div>

      {/* Name + subtitle */}
      <div className="flex min-w-0 flex-1 flex-col">
        <div className="flex items-center gap-1 min-w-0">
          <span className="truncate text-[13px] font-medium leading-tight">
            {soul.display_name}
          </span>
          {isVoiceActive && <Waveform />}
        </div>
        <span
          className={cn(
            'truncate text-[11px] leading-tight',
            activity === 'thinking' || activity === 'running'
              ? 'text-amber-400/80'
              : 'text-muted-foreground/60',
          )}
        >
          {subtitle}
        </span>
      </div>

      {/* Voice button — shown on hover or when active */}
      {voiceEnabled && (
        <button
          onClick={handleVoiceClick}
          title={isVoiceActive ? 'Stop voice' : `Talk to ${soul.display_name}`}
          className={cn(
            'shrink-0 rounded-md p-1 transition-all',
            isVoiceActive
              ? 'text-primary opacity-100'
              : 'text-muted-foreground/40 opacity-0 group-hover/row:opacity-100 hover:text-foreground',
          )}
        >
          <Headphones className="h-3.5 w-3.5" />
        </button>
      )}
    </div>
  );
}

