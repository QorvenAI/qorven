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
import { Headphones, MessageSquare, PhoneOff, Mic, MicOff } from 'lucide-react';
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

// ── Hover tooltip — floats above avatar ──────────────────────────────────────
function AgentTooltip({ soul, activity }: { soul: Soul; activity: SoulActivity }) {
  const activityLabel = {
    running: 'Working', thinking: 'Thinking', idle: 'Idle', offline: 'Offline', error: 'Error',
  }[activity] ?? 'Offline';
  const activityColor = {
    running: 'text-emerald-400', thinking: 'text-amber-400',
    idle: 'text-muted-foreground/70', offline: 'text-muted-foreground/40', error: 'text-destructive',
  }[activity] ?? 'text-muted-foreground/70';

  return (
    <motion.div
      initial={{ opacity: 0, y: 4 }}
      animate={{ opacity: 1, y: 0 }}
      exit={{ opacity: 0, y: 2 }}
      transition={{ duration: 0.1 }}
      className="absolute bottom-full mb-2 left-1/2 -translate-x-1/2 pointer-events-none z-50"
    >
      <div className="rounded-lg border border-border bg-popover shadow-lg px-2.5 py-1.5 text-center whitespace-nowrap">
        <p className="text-[11px] font-semibold text-foreground">{soul.display_name}</p>
        <p className={cn('text-[10px]', activityColor)}>{activityLabel}</p>
        {soul.title && <p className="text-[9px] text-muted-foreground/50 mt-0.5">{soul.title}</p>}
      </div>
      {/* Arrow */}
      <div className="absolute top-full left-1/2 -translate-x-1/2 border-4 border-transparent border-t-border" style={{ marginTop: -1 }} />
      <div className="absolute top-full left-1/2 -translate-x-1/2 border-4 border-transparent border-t-popover" />
    </motion.div>
  );
}

// ── Single stacked avatar: hover tooltip + click popover ──────────────────────
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
  const [hovered, setHovered] = useState(false);
  const soulStates = useStore((s) => s.soulStates);
  const gradient = gradientFor(soul.id);
  const state = soulStates[soul.id];
  const activity: SoulActivity = (state?.activity as SoulActivity) ?? 'offline';

  return (
    <div
      className="relative shrink-0"
      style={{ zIndex: open || hovered ? 50 : 20 - index, marginLeft: index === 0 ? 0 : -10 }}
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
    >
      <button
        onClick={() => { setOpen((v) => !v); setHovered(false); }}
        className={cn(
          'relative h-7 w-7 rounded-full focus:outline-none transition-transform',
          hovered && 'scale-110',
        )}
      >
        {soul.avatar ? (
          <img src={soul.avatar} alt={soul.display_name} className="h-7 w-7 rounded-full object-cover" />
        ) : (
          <div className={cn(
            'flex h-7 w-7 items-center justify-center rounded-full bg-gradient-to-br text-[10px] font-bold text-white transition-all',
            gradient,
            hovered && 'brightness-125',
          )}>
            {(soul.display_name?.[0] ?? '?').toUpperCase()}
          </div>
        )}
        <ActivityRing activity={activity} isVoiceActive={isVoiceActive} />
      </button>

      {/* Hover tooltip — only when not showing click popover */}
      <AnimatePresence>
        {hovered && !open && <AgentTooltip soul={soul} activity={activity} />}
      </AnimatePresence>

      {/* Click popover — full actions */}
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

// ── Two-row pill layout ───────────────────────────────────────────────────────
// Row 1: avatar + name + activity + agent stack
// Row 2: full-width voice button + chat icon
// Inspired by LiveKit's bottom bar — breathing room, clear hierarchy.

interface PillLayoutProps {
  chief: Soul;
  activity: SoulActivity;
  voiceState?: string;
  isVoiceActive: boolean;
  onChat: () => void;
  onVoiceTrigger?: () => void;
  others: Soul[];
  overflow: number;
  voiceEnabled: boolean;
  activeVoiceAgentId: string | null;
  onOtherVoice: (soul: Soul) => void;
}

function PillLayout({
  chief, activity, voiceState, isVoiceActive,
  onChat, onVoiceTrigger, others, overflow,
  voiceEnabled, activeVoiceAgentId, onOtherVoice,
}: PillLayoutProps) {
  const gradient = gradientFor(chief.id);

  // Activity dot colour
  const dotColor = isVoiceActive ? 'bg-primary' :
    activity === 'running' ? 'bg-emerald-400' :
    activity === 'thinking' ? 'bg-amber-400' :
    'bg-muted-foreground/30';

  // Voice button label
  const voiceLabel = isVoiceActive
    ? voiceState === 'listening' ? 'Listening…'
      : voiceState === 'processing' ? 'Processing…'
      : voiceState === 'speaking' ? 'Speaking…'
      : 'End call'
    : `Talk to ${chief.display_name}`;

  return (
    <div
      className="fixed z-29 flex flex-col justify-evenly border-t border-r border-border bg-muted px-3 hidden lg:flex"
      style={{
        left: 'var(--rail-width)',
        width: 'var(--sidebar-default-width, 280px)',
        bottom: 0,
        height: 'var(--agent-pill-height, 84px)',
      }}
    >
      {/* ── Row 1: avatar + name + activity dot + chat icon ── */}
      <div className="flex items-center gap-2 min-w-0">
        <div className="shrink-0">
          {chief.avatar ? (
            <img src={chief.avatar} alt={chief.display_name} className="h-6 w-6 rounded-full object-cover" />
          ) : (
            <div className={cn('flex h-6 w-6 items-center justify-center rounded-full bg-gradient-to-br text-[10px] font-bold text-white', gradient)}>
              {(chief.display_name?.[0] ?? '?').toUpperCase()}
            </div>
          )}
        </div>
        <div className="flex items-center gap-1.5 min-w-0 flex-1">
          <span className="truncate text-[12px] font-semibold leading-tight text-foreground">
            {chief.display_name}
          </span>
          {voiceState && voiceState !== 'idle'
            ? <VoiceIndicator voiceState={voiceState} />
            : <span className={cn('inline-block h-1.5 w-1.5 rounded-full shrink-0', dotColor)} />
          }
        </div>
        <button
          onClick={onChat}
          title={`Open ${chief.display_name}'s chat`}
          className="flex h-6 w-6 shrink-0 items-center justify-center rounded-md text-muted-foreground/50 hover:text-foreground hover:bg-accent transition-colors"
        >
          <MessageSquare className="h-3 w-3" />
        </button>
      </div>

      {/* ── Row 2: voice button (left col) + agent stack (right col) ── */}
      <div className="flex items-center gap-2">
        {/* Voice button — takes available space */}
        {onVoiceTrigger ? (
          <button
            onClick={onVoiceTrigger}
            className={cn(
              'flex flex-1 items-center justify-center gap-1.5 h-7 rounded-md text-[11px] font-medium transition-all',
              isVoiceActive
                ? 'bg-destructive/15 text-destructive border border-destructive/25 hover:bg-destructive/25'
                : 'bg-primary/10 text-primary border border-primary/20 hover:bg-primary/20',
            )}
          >
            {isVoiceActive
              ? <><MicOff className="h-3 w-3" />{voiceLabel}</>
              : <><Mic className="h-3 w-3" />{voiceLabel}</>
            }
          </button>
        ) : (
          <div className="flex flex-1 items-center justify-center gap-1.5 h-7 rounded-md text-[11px] text-muted-foreground/40 border border-border/40">
            <Mic className="h-3 w-3" />{chief.display_name.split(' ')[0]}
          </div>
        )}

        {/* Vertical separator */}
        {others.length > 0 && <span className="h-5 w-px bg-border/50 shrink-0" />}

        {/* Agent stack — right column */}
        {others.length > 0 && (
          <div className="flex items-center shrink-0">
            {others.map((s, i) => (
              <StackedAgent
                key={s.id} soul={s} index={i}
                voiceEnabled={voiceEnabled}
                isVoiceActive={activeVoiceAgentId === s.id}
                onVoiceToggle={onOtherVoice}
              />
            ))}
            {overflow > 0 && (
              <div
                className="flex h-6 w-6 items-center justify-center rounded-full bg-muted border border-border text-[8px] font-medium text-muted-foreground shrink-0"
                style={{ marginLeft: -10, zIndex: 0 }}
              >
                +{overflow}
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  );
}

// Keep HeroZone interface for type compat — redirects to PillLayout
interface HeroZoneProps {
  chief: Soul;
  activity: SoulActivity;
  subtitle: string;
  isVoiceActive: boolean;
  voiceState?: string;
  onChat: () => void;
  onVoice?: () => void;
  onMic?: () => void;
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

  const others = souls
    .filter((s) => s.id !== chief.id)
    .sort((a, b) => activityPriority((soulStates[a.id]?.activity as SoulActivity) ?? 'offline') - activityPriority((soulStates[b.id]?.activity as SoulActivity) ?? 'offline'))
    .slice(0, 4);

  return (
    <PillLayout
      chief={chief} activity={activity} isVoiceActive={false}
      onChat={() => router.push(`/qors/${chief.id}`)}
      others={others}
      overflow={Math.max(0, souls.length - 1 - others.length)}
      voiceEnabled={false}
      activeVoiceAgentId={null}
      onOtherVoice={() => {}}
    />
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
    .slice(0, 4); // max 4 avatars fit within 280px sidebar

  const overflow = Math.max(0, souls.length - 1 - others.length);

  return (
    <PillLayout
      chief={chief}
      activity={activity}
      isVoiceActive={isVoiceActive}
      voiceState={isVoiceActive ? voice.state : undefined}
      onChat={() => router.push(`/qors/${chief.id}`)}
      onVoiceTrigger={handleVoice}
      others={others}
      overflow={overflow}
      voiceEnabled={true}
      activeVoiceAgentId={activeVoiceAgentId}
      onOtherVoice={handleOtherVoice}
    />
  );
}
