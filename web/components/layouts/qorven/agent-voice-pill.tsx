'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).
//
// AgentVoicePill — persistent bar above the status bar on all pages.
// Left: COO hero with voice controls.
// Middle: other agents stacked, sorted by activity, click to chat/voice.
// Right: voice + chat action buttons.

import { useEffect, useState, useCallback, useRef } from 'react';
import { useRouter } from 'next/navigation';
import { motion, AnimatePresence } from 'motion/react';
import { Headphones, MessageSquare, PhoneOff } from 'lucide-react';
import { cn } from '@/lib/utils';
import { useVoice } from '@/hooks/use-voice';
import { useVoiceEnabled } from '@/hooks/use-voice-enabled';
import { useStore } from '@/store';
import { agents as agentsApi } from '@/lib/api';
import { agentVoiceRegistry } from '@/lib/voice-registry';
import type { Soul, SoulActivity } from '@/types';

// ── Gradient (matches sidebar-agent-row) ─────────────────────────────────────
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

// Activity sort priority — running > thinking > idle > offline > error
function activityPriority(a: SoulActivity): number {
  return { running: 0, thinking: 1, idle: 2, offline: 3, error: 4 }[a] ?? 3;
}

// ── Activity ring around each stacked avatar ──────────────────────────────────
function ActivityRing({ activity, isVoiceActive }: { activity: SoulActivity; isVoiceActive: boolean }) {
  if (isVoiceActive) {
    return (
      <motion.span
        className="absolute inset-0 rounded-full ring-2 ring-primary"
        animate={{ opacity: [1, 0.4, 1] }}
        transition={{ duration: 1.2, repeat: Infinity, ease: 'easeInOut' }}
      />
    );
  }
  if (activity === 'running') {
    return (
      <motion.span
        className="absolute inset-0 rounded-full ring-2 ring-emerald-400"
        animate={{ rotate: 360 }}
        transition={{ duration: 2, repeat: Infinity, ease: 'linear' }}
        style={{ clipPath: 'inset(0 0 50% 0)' }}
      />
    );
  }
  if (activity === 'thinking') {
    return (
      <motion.span
        className="absolute inset-0 rounded-full ring-2 ring-amber-400"
        animate={{ opacity: [1, 0.3, 1] }}
        transition={{ duration: 1, repeat: Infinity, ease: 'easeInOut' }}
      />
    );
  }
  if (activity === 'idle') {
    return <span className="absolute inset-0 rounded-full ring-1 ring-border/60" />;
  }
  // offline / error — very faint
  return <span className="absolute inset-0 rounded-full ring-1 ring-border/20" />;
}

// ── Waveform bars (voice state driven) ───────────────────────────────────────
function VoiceIndicator({ voiceState }: { voiceState: string }) {
  if (voiceState === 'listening') {
    // Short bars pulsing upward — mic input
    return (
      <span className="inline-flex items-end gap-[2px] h-4 shrink-0">
        {[0, 1, 2].map((i) => (
          <motion.span
            key={i}
            className="w-[3px] rounded-full bg-emerald-400"
            animate={{ height: ['3px', '10px', '3px'] }}
            transition={{ duration: 0.5, repeat: Infinity, delay: i * 0.1, ease: 'easeInOut' }}
          />
        ))}
      </span>
    );
  }
  if (voiceState === 'processing') {
    // 3 dots pulsing — thinking
    return (
      <span className="inline-flex items-center gap-[3px] h-4 shrink-0">
        {[0, 1, 2].map((i) => (
          <motion.span
            key={i}
            className="w-[4px] h-[4px] rounded-full bg-amber-400"
            animate={{ opacity: [0.2, 1, 0.2], scale: [0.8, 1.2, 0.8] }}
            transition={{ duration: 0.8, repeat: Infinity, delay: i * 0.2, ease: 'easeInOut' }}
          />
        ))}
      </span>
    );
  }
  if (voiceState === 'speaking') {
    // Taller bars — agent speaking
    return (
      <span className="inline-flex items-end gap-[2px] h-4 shrink-0">
        {[0, 1, 2, 3].map((i) => (
          <motion.span
            key={i}
            className="w-[3px] rounded-full bg-primary"
            animate={{ height: ['4px', '14px', '4px'] }}
            transition={{ duration: 0.6, repeat: Infinity, delay: i * 0.1, ease: 'easeInOut' }}
          />
        ))}
      </span>
    );
  }
  return null;
}

// ── Hook: resolve COO agent ───────────────────────────────────────────────────
function useChief() {
  const souls = useStore((s) => s.souls);
  const [chief, setChief] = useState<Soul | null>(null);

  useEffect(() => {
    agentsApi.chief()
      .then((c) => { if (c?.id) setChief(c as Soul); })
      .catch(() => {});
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    if (!chief && souls.length > 0) setChief(souls[0] as Soul);
  }, [souls, chief]);

  return chief;
}

// ── Agent popover — appears above the clicked avatar ─────────────────────────
interface AgentPopoverProps {
  soul: Soul;
  activity: SoulActivity;
  onChat: () => void;
  onVoice?: () => void;
  onClose: () => void;
  voiceEnabled: boolean;
  isVoiceActive: boolean;
}

function AgentPopover({ soul, activity, onChat, onVoice, onClose, voiceEnabled, isVoiceActive }: AgentPopoverProps) {
  const ref = useRef<HTMLDivElement>(null);
  const gradient = gradientFor(soul.id);

  // Close on click-outside
  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) onClose();
    };
    const escHandler = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose(); };
    document.addEventListener('mousedown', handler);
    document.addEventListener('keydown', escHandler);
    return () => {
      document.removeEventListener('mousedown', handler);
      document.removeEventListener('keydown', escHandler);
    };
  }, [onClose]);

  const activityLabel = {
    running: 'Working', thinking: 'Thinking', idle: 'Idle', offline: 'Offline', error: 'Error',
  }[activity] ?? 'Offline';

  const activityColor = {
    running: 'text-emerald-400', thinking: 'text-amber-400',
    idle: 'text-muted-foreground', offline: 'text-muted-foreground/50', error: 'text-destructive',
  }[activity] ?? 'text-muted-foreground';

  return (
    <motion.div
      ref={ref}
      initial={{ opacity: 0, y: 6, scale: 0.95 }}
      animate={{ opacity: 1, y: 0, scale: 1 }}
      exit={{ opacity: 0, y: 4, scale: 0.95 }}
      transition={{ duration: 0.12 }}
      className="absolute bottom-full mb-2 left-1/2 -translate-x-1/2 w-44 rounded-xl border border-border bg-popover shadow-xl z-50 overflow-hidden"
    >
      {/* Agent header */}
      <div className="flex items-center gap-2 px-3 py-2.5 border-b border-border/60">
        {soul.avatar ? (
          <img src={soul.avatar} alt={soul.display_name} className="h-7 w-7 rounded-full object-cover shrink-0" />
        ) : (
          <div className={cn('flex h-7 w-7 items-center justify-center rounded-full bg-gradient-to-br text-[10px] font-bold text-white shrink-0', gradient)}>
            {(soul.display_name?.[0] ?? '?').toUpperCase()}
          </div>
        )}
        <div className="min-w-0">
          <p className="text-[12px] font-semibold truncate">{soul.display_name}</p>
          <p className={cn('text-[10px]', activityColor)}>{activityLabel}</p>
        </div>
      </div>

      {/* Actions */}
      <div className="p-1.5 space-y-0.5">
        <button
          onClick={() => { onChat(); onClose(); }}
          className="flex w-full items-center gap-2.5 rounded-lg px-2.5 py-2 text-[12px] hover:bg-accent transition-colors"
        >
          <MessageSquare className="h-3.5 w-3.5 text-muted-foreground" />
          Open Chat
        </button>
        {voiceEnabled && onVoice && (
          <button
            onClick={() => { onVoice(); onClose(); }}
            className={cn(
              'flex w-full items-center gap-2.5 rounded-lg px-2.5 py-2 text-[12px] transition-colors',
              isVoiceActive
                ? 'text-destructive hover:bg-destructive/10'
                : 'hover:bg-accent',
            )}
          >
            {isVoiceActive
              ? <><PhoneOff className="h-3.5 w-3.5" />End voice</>
              : <><Headphones className="h-3.5 w-3.5 text-muted-foreground" />Start voice</>
            }
          </button>
        )}
      </div>
    </motion.div>
  );
}

// ── Single stacked avatar with ring + popover ─────────────────────────────────
interface StackedAgentProps {
  soul: Soul;
  index: number;
  voiceEnabled: boolean;
  isVoiceActive: boolean;
  onVoiceToggle: (soul: Soul) => void;
}

function StackedAgent({ soul, index, voiceEnabled, isVoiceActive, onVoiceToggle }: StackedAgentProps) {
  const router = useRouter();
  const [open, setOpen] = useState(false);
  const soulStates = useStore((s) => s.soulStates);
  const gradient = gradientFor(soul.id);
  const state = soulStates[soul.id];
  const activity: SoulActivity = (state?.activity as SoulActivity) ?? 'offline';

  return (
    <div
      className="relative shrink-0"
      style={{ zIndex: 20 - index, marginLeft: index === 0 ? 0 : -6 }}
    >
      <button
        onClick={() => setOpen((v) => !v)}
        className="relative h-7 w-7 rounded-full focus:outline-none focus:ring-2 focus:ring-primary/40"
        title={soul.display_name}
      >
        {soul.avatar ? (
          <img src={soul.avatar} alt={soul.display_name} className="h-7 w-7 rounded-full object-cover" />
        ) : (
          <div className={cn('flex h-7 w-7 items-center justify-center rounded-full bg-gradient-to-br text-[10px] font-bold text-white', gradient)}>
            {(soul.display_name?.[0] ?? '?').toUpperCase()}
          </div>
        )}
        <ActivityRing activity={activity} isVoiceActive={isVoiceActive} />
      </button>

      <AnimatePresence>
        {open && (
          <AgentPopover
            soul={soul}
            activity={activity}
            voiceEnabled={voiceEnabled}
            isVoiceActive={isVoiceActive}
            onChat={() => router.push(`/qors/${soul.id}`)}
            onVoice={() => onVoiceToggle(soul)}
            onClose={() => setOpen(false)}
          />
        )}
      </AnimatePresence>
    </div>
  );
}

// ── Fixed position container ──────────────────────────────────────────────────
function PillContainer({ children }: { children: React.ReactNode }) {
  return (
    <div
      className="fixed z-29 flex items-center gap-2 border-t border-r border-border bg-muted px-2 hidden lg:flex"
      style={{
        left: 'var(--rail-width)',
        width: 'var(--sidebar-default-width, 280px)',
        bottom: 'var(--status-bar-height, 24px)',
        height: 'var(--agent-pill-height, 48px)',
      }}
    >
      {children}
    </div>
  );
}

// ── COO hero zone ─────────────────────────────────────────────────────────────
interface HeroZoneProps {
  chief: Soul;
  activity: SoulActivity;
  subtitle: string;
  isVoiceActive: boolean;
  voiceState?: string;
}

function HeroZone({ chief, activity, subtitle, isVoiceActive, voiceState }: HeroZoneProps) {
  const gradient = gradientFor(chief.id);
  return (
    <div className={cn('flex items-center gap-2 min-w-0 flex-1', isVoiceActive && 'text-primary/90')}>
      {/* Avatar */}
      <div className="shrink-0">
        {chief.avatar ? (
          <img src={chief.avatar} alt={chief.display_name} className="h-7 w-7 rounded-full object-cover" />
        ) : (
          <div className={cn('flex h-7 w-7 items-center justify-center rounded-full bg-gradient-to-br text-[11px] font-bold text-white', gradient)}>
            {(chief.display_name?.[0] ?? '?').toUpperCase()}
          </div>
        )}
      </div>

      {/* Name + state */}
      <div className="flex min-w-0 flex-col flex-1">
        <div className="flex items-center gap-1.5 min-w-0">
          <span className="truncate text-[12px] font-semibold leading-tight">{chief.display_name}</span>
          {voiceState && voiceState !== 'idle' && <VoiceIndicator voiceState={voiceState} />}
        </div>
        <span className={cn(
          'truncate text-[10px] leading-tight',
          isVoiceActive ? 'text-primary/80'
            : activity === 'thinking' || activity === 'running' ? 'text-amber-400/80'
            : 'text-muted-foreground/60',
        )}>
          {subtitle}
        </span>
      </div>
    </div>
  );
}

// ── Public export ─────────────────────────────────────────────────────────────
export function AgentVoicePill() {
  const { enabled: voiceEnabled, loading: voiceLoading } = useVoiceEnabled();
  if (voiceLoading) return null;
  if (!voiceEnabled) return <PillBasic />;
  return <PillWithVoice />;
}

// ── Voice-disabled variant ────────────────────────────────────────────────────
function PillBasic() {
  const router = useRouter();
  const chief = useChief();
  const souls = useStore((s) => s.souls);
  const soulStates = useStore((s) => s.soulStates);

  if (!chief) return null;

  const state = soulStates[chief.id];
  const activity: SoulActivity = (state?.activity as SoulActivity) ?? 'offline';
  const subtitle = chief.title || chief.role || 'Chief of Staff';

  const others = souls
    .filter((s) => s.id !== chief.id)
    .sort((a, b) => {
      const aA = (soulStates[a.id]?.activity as SoulActivity) ?? 'offline';
      const bA = (soulStates[b.id]?.activity as SoulActivity) ?? 'offline';
      return activityPriority(aA) - activityPriority(bA);
    })
    .slice(0, 6);

  const overflow = souls.length - 1 - others.length;

  return (
    <PillContainer>
      <HeroZone chief={chief} activity={activity} subtitle={subtitle} isVoiceActive={false} />

      {/* Divider */}
      {others.length > 0 && <span className="h-5 w-px bg-border/60 shrink-0" />}

      {/* Other agents stack */}
      <div className="flex items-center shrink-0">
        {others.map((s, i) => (
          <StackedAgent key={s.id} soul={s} index={i} voiceEnabled={false} isVoiceActive={false} onVoiceToggle={() => {}} />
        ))}
        {overflow > 0 && (
          <div className="flex h-7 w-7 items-center justify-center rounded-full bg-muted border border-border text-[9px] font-medium text-muted-foreground shrink-0"
            style={{ marginLeft: -6, zIndex: 0 }}>
            +{overflow}
          </div>
        )}
      </div>

      {/* Chat button */}
      <button
        onClick={() => router.push(`/qors/${chief.id}`)}
        title={`Open ${chief.display_name}'s chat`}
        className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-muted-foreground hover:text-foreground hover:bg-muted/80 transition-colors"
      >
        <MessageSquare className="h-3.5 w-3.5" />
      </button>
    </PillContainer>
  );
}

// ── Voice-enabled variant ─────────────────────────────────────────────────────
function PillWithVoice() {
  const router = useRouter();
  const chief = useChief();
  const souls = useStore((s) => s.souls);
  const soulStates = useStore((s) => s.soulStates);
  const activeVoiceAgentId = useStore((s) => s.activeVoiceAgentId);
  const setActiveVoiceAgent = useStore((s) => s.setActiveVoiceAgent);

  const agentId = chief?.id ?? '';
  const isVoiceActive = activeVoiceAgentId === agentId && !!agentId;
  const voice = useVoice({ agentId: agentId || '__noop__' });

  useEffect(() => {
    if (!agentId) return;
    agentVoiceRegistry.set(agentId, voice.stop);
    return () => { agentVoiceRegistry.delete(agentId); };
  }, [agentId, voice.stop]);

  const handleVoice = useCallback(async () => {
    if (!agentId) return;
    if (isVoiceActive) {
      await voice.stop();
      setActiveVoiceAgent(null);
    } else {
      if (activeVoiceAgentId) {
        const prev = agentVoiceRegistry.get(activeVoiceAgentId);
        if (prev) await prev();
        setActiveVoiceAgent(null);
        await new Promise((r) => setTimeout(r, 80));
      }
      await voice.start();
      setActiveVoiceAgent(agentId);
    }
  }, [agentId, isVoiceActive, voice, activeVoiceAgentId, setActiveVoiceAgent]);

  // Voice toggle for stacked agents
  const handleOtherVoice = useCallback(async (soul: Soul) => {
    const otherId = soul.id;
    const otherIsActive = activeVoiceAgentId === otherId;
    if (otherIsActive) {
      const prev = agentVoiceRegistry.get(otherId);
      if (prev) await prev();
      setActiveVoiceAgent(null);
    } else {
      if (activeVoiceAgentId) {
        const prev = agentVoiceRegistry.get(activeVoiceAgentId);
        if (prev) await prev();
        setActiveVoiceAgent(null);
        await new Promise((r) => setTimeout(r, 80));
      }
      // Navigate to the agent's page — voice from there
      router.push(`/qors/${otherId}`);
    }
  }, [activeVoiceAgentId, setActiveVoiceAgent, router]);

  if (!chief) return null;

  const state = soulStates[chief.id];
  const activity: SoulActivity = (state?.activity as SoulActivity) ?? 'offline';

  let subtitle = chief.title || chief.role || 'Chief of Staff';
  if (isVoiceActive) {
    subtitle = voice.state === 'listening' ? 'Listening…'
      : voice.state === 'processing' ? 'Processing…'
      : voice.state === 'speaking' ? 'Speaking…'
      : 'Voice active';
  } else if (activity === 'thinking') {
    subtitle = state?.lastEvent?.trim().slice(0, 44) || 'Thinking…';
  } else if (activity === 'running') {
    subtitle = state?.lastEvent?.trim().slice(0, 44) || 'Working…';
  }

  const others = souls
    .filter((s) => s.id !== chief.id)
    .sort((a, b) => {
      const aA = (soulStates[a.id]?.activity as SoulActivity) ?? 'offline';
      const bA = (soulStates[b.id]?.activity as SoulActivity) ?? 'offline';
      return activityPriority(aA) - activityPriority(bA);
    })
    .slice(0, 6);

  const overflow = souls.length - 1 - others.length;

  return (
    <PillContainer>
      {/* COO hero zone */}
      <HeroZone
        chief={chief}
        activity={activity}
        subtitle={subtitle}
        isVoiceActive={isVoiceActive}
        voiceState={isVoiceActive ? voice.state : undefined}
      />

      {/* Divider */}
      {others.length > 0 && <span className="h-5 w-px bg-border/60 shrink-0" />}

      {/* Other agents stacked */}
      <div className="flex items-center shrink-0">
        {others.map((s, i) => (
          <StackedAgent
            key={s.id}
            soul={s}
            index={i}
            voiceEnabled={true}
            isVoiceActive={activeVoiceAgentId === s.id}
            onVoiceToggle={handleOtherVoice}
          />
        ))}
        {overflow > 0 && (
          <div
            className="flex h-7 w-7 items-center justify-center rounded-full bg-muted border border-border text-[9px] font-medium text-muted-foreground shrink-0"
            style={{ marginLeft: -6, zIndex: 0 }}
          >
            +{overflow}
          </div>
        )}
      </div>

      {/* COO action buttons */}
      <div className="flex items-center gap-0.5 shrink-0">
        <button
          onClick={handleVoice}
          title={isVoiceActive ? 'End voice session' : `Talk to ${chief.display_name}`}
          className={cn(
            'flex h-7 w-7 items-center justify-center rounded-md transition-colors',
            isVoiceActive
              ? 'text-destructive bg-destructive/10 hover:bg-destructive/20'
              : 'text-muted-foreground hover:text-foreground hover:bg-muted/80',
          )}
        >
          {isVoiceActive ? <PhoneOff className="h-3.5 w-3.5" /> : <Headphones className="h-3.5 w-3.5" />}
        </button>

        <button
          onClick={() => router.push(`/qors/${chief.id}`)}
          title={`Open ${chief.display_name}'s chat`}
          className="flex h-7 w-7 items-center justify-center rounded-md text-muted-foreground hover:text-foreground hover:bg-muted/80 transition-colors"
        >
          <MessageSquare className="h-3.5 w-3.5" />
        </button>
      </div>
    </PillContainer>
  );
}
