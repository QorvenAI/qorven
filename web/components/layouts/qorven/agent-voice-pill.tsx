'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).
//
// AgentVoicePill — persistent COO agent bar pinned above the status bar.
// Split into two components so useVoice (which boots the VAD ONNX runtime)
// is only mounted when voice is actually enabled.

import { useEffect, useState, useCallback } from 'react';
import { useRouter } from 'next/navigation';
import { motion } from 'motion/react';
import { Headphones, MessageSquare, PhoneOff } from 'lucide-react';
import { cn } from '@/lib/utils';
import { SoulPulseRing } from '@/components/soul-pulse-ring';
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

// ── Waveform bars ─────────────────────────────────────────────────────────────
function Waveform() {
  return (
    <span className="inline-flex items-end gap-[2px] h-4 shrink-0">
      {[0, 1, 2, 3].map((i) => (
        <motion.span
          key={i}
          className="w-[3px] rounded-full bg-primary"
          animate={{ height: ['3px', '14px', '3px'] }}
          transition={{ duration: 0.65, repeat: Infinity, delay: i * 0.12, ease: 'easeInOut' }}
        />
      ))}
    </span>
  );
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

  // Fallback to first soul once store loads
  useEffect(() => {
    if (!chief && souls.length > 0) setChief(souls[0] as Soul);
  }, [souls, chief]);

  return chief;
}

// ── Fixed position container ──────────────────────────────────────────────────
function PillContainer({ children }: { children: React.ReactNode }) {
  return (
    <div
      className="fixed z-29 flex items-center border-t border-border bg-muted px-2 hidden lg:flex"
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

// ── Shared pill card visuals ──────────────────────────────────────────────────
interface PillCardProps {
  chief: Soul;
  activity: SoulActivity;
  subtitle: string;
  isVoiceActive: boolean;
  children: React.ReactNode; // action buttons
}

function PillCard({ chief, activity, subtitle, isVoiceActive, children }: PillCardProps) {
  const gradient = gradientFor(chief.id);
  return (
    <div
      className={cn(
        'flex flex-1 items-center gap-2.5 rounded-lg border px-3 py-2 min-w-0 transition-colors',
        isVoiceActive
          ? 'border-primary/30 bg-primary/5 ring-1 ring-primary/20'
          : 'border-border bg-card/50',
      )}
    >
      {/* Avatar + pulse */}
      <div className="relative shrink-0">
        {chief.avatar ? (
          <img src={chief.avatar} alt={chief.display_name} className="h-7 w-7 rounded-full object-cover" />
        ) : (
          <div className={cn('flex h-7 w-7 items-center justify-center rounded-full bg-gradient-to-br text-[11px] font-bold text-white', gradient)}>
            {(chief.display_name?.[0] ?? '?').toUpperCase()}
          </div>
        )}
        <span className="absolute -bottom-0.5 -right-0.5">
          <SoulPulseRing activity={isVoiceActive ? 'running' : activity} size="sm" />
        </span>
      </div>

      {/* Name + subtitle */}
      <div className="flex min-w-0 flex-1 flex-col">
        <div className="flex items-center gap-1.5 min-w-0">
          <span className="truncate text-[12px] font-semibold leading-tight">
            {chief.display_name}
          </span>
          {isVoiceActive && <Waveform />}
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

      {/* Buttons slot */}
      <div className="flex items-center gap-1 shrink-0">
        {children}
      </div>
    </div>
  );
}

// ── Public export — gates on voiceEnabled ─────────────────────────────────────
export function AgentVoicePill() {
  const { enabled: voiceEnabled, loading: voiceLoading } = useVoiceEnabled();
  // While loading, render nothing to avoid flash
  if (voiceLoading) return null;
  // Voice disabled: render pill without VAD machinery
  if (!voiceEnabled) return <PillBasic />;
  // Voice enabled: full version with useVoice mounted
  return <PillWithVoice />;
}

// ── Voice-disabled variant — no useVoice, no VAD ─────────────────────────────
function PillBasic() {
  const router = useRouter();
  const chief = useChief();
  const soulStates = useStore((s) => s.soulStates);

  if (!chief) return null;

  const state = soulStates[chief.id];
  const activity: SoulActivity = (state?.activity as SoulActivity) ?? 'offline';
  const subtitle = chief.title || chief.role || 'Chief of Staff';

  return (
    <PillContainer>
      <PillCard chief={chief} activity={activity} subtitle={subtitle} isVoiceActive={false}>
        <button
          onClick={() => router.push(`/qors/${chief.id}`)}
          title={`Open ${chief.display_name}'s chat`}
          className="flex h-7 w-7 items-center justify-center rounded-md text-muted-foreground hover:text-foreground hover:bg-muted transition-colors"
        >
          <MessageSquare className="h-3.5 w-3.5" />
        </button>
      </PillCard>
    </PillContainer>
  );
}

// ── Voice-enabled variant — mounts useVoice once, registers in registry ───────
function PillWithVoice() {
  const router = useRouter();
  const chief = useChief();
  const soulStates = useStore((s) => s.soulStates);
  const activeVoiceAgentId = useStore((s) => s.activeVoiceAgentId);
  const setActiveVoiceAgent = useStore((s) => s.setActiveVoiceAgent);

  const agentId = chief?.id ?? '';
  const isVoiceActive = activeVoiceAgentId === agentId && !!agentId;

  // Only mount useVoice when we have a real agentId
  const voice = useVoice({ agentId: agentId || '__noop__' });

  // Register in shared registry for cross-component stop
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

  return (
    <PillContainer>
      <PillCard chief={chief} activity={activity} subtitle={subtitle} isVoiceActive={isVoiceActive}>
        {/* Voice toggle */}
        <button
          onClick={handleVoice}
          title={isVoiceActive ? 'End voice session' : `Talk to ${chief.display_name}`}
          className={cn(
            'flex h-7 w-7 items-center justify-center rounded-md transition-colors',
            isVoiceActive
              ? 'text-destructive bg-destructive/10 hover:bg-destructive/20'
              : 'text-muted-foreground hover:text-foreground hover:bg-muted',
          )}
        >
          {isVoiceActive ? <PhoneOff className="h-3.5 w-3.5" /> : <Headphones className="h-3.5 w-3.5" />}
        </button>

        {/* Open chat */}
        <button
          onClick={() => router.push(`/qors/${chief.id}`)}
          title={`Open ${chief.display_name}'s chat`}
          className="flex h-7 w-7 items-center justify-center rounded-md text-muted-foreground hover:text-foreground hover:bg-muted transition-colors"
        >
          <MessageSquare className="h-3.5 w-3.5" />
        </button>
      </PillCard>
    </PillContainer>
  );
}
