'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).
//
// AgentVoicePill — persistent agent bar above the status bar.
// Row 1: agent switcher (▼ chevron opens searchable dropdown) + mic + chat
// Row 2: voice state visual (orb / waveform) — only when voice active
// No avatar stack — scales to any number of agents.

import { useEffect, useState, useCallback, useRef } from 'react';
import { useRouter } from 'next/navigation';
import { motion, AnimatePresence } from 'motion/react';
import { ChevronDown, Mic, MicOff, MessageSquare, Search } from 'lucide-react';
import { cn } from '@/lib/utils';
import { useVoice } from '@/hooks/use-voice';
import { useVoiceEnabled } from '@/hooks/use-voice-enabled';
import { useStore } from '@/store';
import { agents as agentsApi } from '@/lib/api';
import { agentVoiceRegistry } from '@/lib/voice-registry';
import type { Soul, SoulActivity } from '@/types';

// ── Gradient helper ───────────────────────────────────────────────────────────
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

// ── Activity label ────────────────────────────────────────────────────────────
function activityLabel(a: SoulActivity): string {
  return { running: 'Working', thinking: 'Thinking', idle: 'Ready', offline: 'Offline', error: 'Error' }[a] ?? 'Offline';
}
function activityDotColor(a: SoulActivity): string {
  return { running: 'bg-emerald-400', thinking: 'bg-amber-400', idle: 'bg-emerald-500/50', offline: 'bg-muted-foreground/25', error: 'bg-destructive' }[a] ?? 'bg-muted-foreground/25';
}

// ── Volume-reactive orb (ElevenLabs-inspired) ─────────────────────────────────
function VoiceOrb({ voiceState, volume = 0 }: { voiceState: string; volume?: number }) {
  const isActive = voiceState !== 'idle';
  const glow = Math.min(1, volume * 2);

  const orbColor = voiceState === 'listening'
    ? 'from-emerald-400 to-teal-500'
    : voiceState === 'processing'
      ? 'from-amber-400 to-orange-500'
      : 'from-primary to-violet-500';

  return (
    <div className="flex items-center justify-center gap-3 py-0.5">
      {/* Orb */}
      <div className="relative flex items-center justify-center">
        {/* Glow ring — expands with volume */}
        {isActive && (
          <motion.div
            className={cn('absolute rounded-full bg-gradient-to-br opacity-20', orbColor)}
            animate={{
              width: volume > 0.01 ? `${28 + glow * 16}px` : ['28px', '36px', '28px'],
              height: volume > 0.01 ? `${28 + glow * 16}px` : ['28px', '36px', '28px'],
              opacity: volume > 0.01 ? 0.15 + glow * 0.2 : [0.1, 0.25, 0.1],
            }}
            transition={volume > 0.01
              ? { duration: 0.08, ease: 'easeOut' }
              : { duration: 1.4, repeat: Infinity, ease: 'easeInOut' }
            }
          />
        )}
        {/* Core orb */}
        <motion.div
          className={cn(
            'relative w-5 h-5 rounded-full bg-gradient-to-br',
            isActive ? orbColor : 'from-muted-foreground/20 to-muted-foreground/10',
          )}
          animate={isActive
            ? { scale: volume > 0.01 ? 1 + glow * 0.15 : [1, 1.06, 1] }
            : { scale: 1 }
          }
          transition={volume > 0.01
            ? { duration: 0.08, ease: 'easeOut' }
            : { duration: 1.4, repeat: Infinity, ease: 'easeInOut' }
          }
        />
      </div>

      {/* State label + waveform bars */}
      <div className="flex items-center gap-1.5">
        <span className={cn(
          'text-[10px] font-medium',
          voiceState === 'listening' ? 'text-emerald-400'
            : voiceState === 'processing' ? 'text-amber-400'
            : 'text-primary/80',
        )}>
          {voiceState === 'listening' ? 'Listening…'
            : voiceState === 'processing' ? 'Thinking…'
            : voiceState === 'speaking' ? 'Speaking…'
            : 'Voice active'}
        </span>

        {/* Mini waveform */}
        {(voiceState === 'listening' || voiceState === 'speaking') && (
          <span className="inline-flex items-center gap-[2px]" style={{ height: '10px' }}>
            {[0.6, 1, 0.8, 1, 0.6].map((w, i) => {
              const h = volume > 0.01
                ? Math.max(2, Math.round(2 + 8 * volume * w))
                : undefined;
              return (
                <motion.span
                  key={i}
                  className={cn('w-[2px] rounded-full', voiceState === 'listening' ? 'bg-emerald-400' : 'bg-primary')}
                  animate={h
                    ? { height: `${h}px` }
                    : { height: [`2px`, `${Math.round(8 * w)}px`, `2px`] }
                  }
                  transition={h
                    ? { duration: 0.08, ease: 'easeOut' }
                    : { duration: 0.55, repeat: Infinity, delay: i * 0.08, ease: 'easeInOut' }
                  }
                />
              );
            })}
          </span>
        )}

        {voiceState === 'processing' && (
          <span className="inline-flex items-center gap-[3px]">
            {[0, 1, 2].map((i) => (
              <motion.span
                key={i}
                className="w-[3px] h-[3px] rounded-full bg-amber-400"
                animate={{ y: [0, -3, 0], opacity: [0.4, 1, 0.4] }}
                transition={{ duration: 0.6, repeat: Infinity, delay: i * 0.15, ease: 'easeInOut' }}
              />
            ))}
          </span>
        )}
      </div>
    </div>
  );
}

// ── Agent switcher dropdown ───────────────────────────────────────────────────
interface SwitcherDropdownProps {
  souls: Soul[];
  soulStates: Record<string, any>;
  selectedId: string;
  onSelect: (soul: Soul) => void;
  onClose: () => void;
}

function SwitcherDropdown({ souls, soulStates, selectedId, onSelect, onClose }: SwitcherDropdownProps) {
  const [query, setQuery] = useState('');
  const inputRef = useRef<HTMLInputElement>(null);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    inputRef.current?.focus();
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) onClose();
    };
    const esc = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose(); };
    document.addEventListener('mousedown', handler);
    document.addEventListener('keydown', esc);
    return () => {
      document.removeEventListener('mousedown', handler);
      document.removeEventListener('keydown', esc);
    };
  }, [onClose]);

  // Separate COO (org_level === 'l1') and pin at top
  const coo = souls.find((s) => s.org_level === 'l1' || s.org_role === 'coo');
  const rest = souls.filter((s) => s.id !== coo?.id);

  const matchesQuery = (s: Soul) =>
    !query ||
    s.display_name.toLowerCase().includes(query.toLowerCase()) ||
    (s.title || s.role || '').toLowerCase().includes(query.toLowerCase());

  const filteredCoo = coo && matchesQuery(coo) ? coo : null;
  const filteredRest = rest.filter(matchesQuery);

  const AgentItem = ({ soul, isPinned }: { soul: Soul; isPinned?: boolean }) => {
    const state = soulStates[soul.id];
    const activity: SoulActivity = (state?.activity as SoulActivity) ?? 'idle';
    const gradient = gradientFor(soul.id);
    const isSelected = soul.id === selectedId;
    const isActive = activity === 'running' || activity === 'thinking';

    // Status ring colour on avatar
    const ringColor = activity === 'running' ? 'ring-emerald-400'
      : activity === 'thinking' ? 'ring-amber-400'
      : 'ring-transparent';

    // Status text shown next to name
    const statusText = activity === 'running' ? 'Working now'
      : activity === 'thinking' ? 'Thinking…'
      : null;
    const statusColor = activity === 'running' ? 'text-emerald-400'
      : 'text-amber-400';

    return (
      <button
        onClick={() => { onSelect(soul); onClose(); }}
        className={cn(
          'flex w-full items-center gap-3 px-3 py-2.5 text-left transition-colors',
          isSelected ? 'bg-primary/10' : 'hover:bg-accent',
        )}
      >
        {/* Avatar with activity ring */}
        <div className="relative shrink-0">
          {soul.avatar ? (
            <img
              src={soul.avatar}
              alt={soul.display_name}
              className={cn('h-8 w-8 rounded-full object-cover ring-2', isActive ? ringColor : 'ring-transparent')}
            />
          ) : (
            <div className={cn(
              'flex h-8 w-8 items-center justify-center rounded-full bg-gradient-to-br text-[11px] font-bold text-white ring-2',
              gradient,
              isActive ? ringColor : 'ring-transparent',
            )}>
              {(soul.display_name?.[0] ?? '?').toUpperCase()}
            </div>
          )}
          {/* Animated pulse dot — bottom right */}
          <span className="absolute -bottom-0.5 -right-0.5 flex h-3 w-3 items-center justify-center">
            {isActive ? (
              <>
                <motion.span
                  className={cn('absolute inline-flex h-2.5 w-2.5 rounded-full', activity === 'running' ? 'bg-emerald-400' : 'bg-amber-400')}
                  animate={{ scale: [1, 1.6, 1], opacity: [0.8, 0, 0.8] }}
                  transition={{ duration: 1.5, repeat: Infinity, ease: 'easeInOut' }}
                />
                <span className={cn('relative inline-flex h-2 w-2 rounded-full', activity === 'running' ? 'bg-emerald-400' : 'bg-amber-400')} />
              </>
            ) : (
              <span className="relative inline-flex h-2 w-2 rounded-full bg-emerald-500/40 ring-1 ring-background" />
            )}
          </span>
        </div>

        {/* Name + designation + status */}
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2 min-w-0">
            <p className={cn('text-[12px] font-semibold truncate leading-tight', isSelected ? 'text-foreground' : 'text-foreground/90')}>
              {soul.display_name}
            </p>
            {isPinned && (
              <span className="shrink-0 rounded text-[9px] font-bold px-1 py-0.5 bg-primary/15 text-primary uppercase tracking-wide">
                COO
              </span>
            )}
          </div>
          <div className="flex items-center gap-1.5 mt-0.5">
            <p className="text-[10px] text-muted-foreground/60 truncate">
              {soul.title || soul.role || 'Agent'}
            </p>
            {statusText && (
              <>
                <span className="text-muted-foreground/30 shrink-0">·</span>
                <p className={cn('text-[10px] shrink-0', statusColor)}>{statusText}</p>
              </>
            )}
          </div>
        </div>
      </button>
    );
  };

  const empty = !filteredCoo && filteredRest.length === 0;

  return (
    <motion.div
      ref={ref}
      initial={{ opacity: 0, y: 6, scale: 0.97 }}
      animate={{ opacity: 1, y: 0, scale: 1 }}
      exit={{ opacity: 0, y: 4, scale: 0.97 }}
      transition={{ duration: 0.12 }}
      className="absolute bottom-full mb-2 left-0 w-full rounded-xl border border-border bg-popover shadow-xl z-50 overflow-hidden"
    >
      {/* Search */}
      <div className="flex items-center gap-2 px-3 py-2 border-b border-border/60">
        <Search className="h-3.5 w-3.5 text-muted-foreground/50 shrink-0" />
        <input
          ref={inputRef}
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Search agents…"
          className="flex-1 bg-transparent text-[12px] text-foreground placeholder:text-muted-foreground/50 outline-none"
        />
      </div>

      <div className="max-h-64 overflow-y-auto">
        {empty ? (
          <p className="px-3 py-4 text-[11px] text-muted-foreground text-center">No agents found</p>
        ) : (
          <>
            {/* Pinned COO */}
            {filteredCoo && (
              <>
                <AgentItem soul={filteredCoo} isPinned />
                {filteredRest.length > 0 && <div className="mx-3 h-px bg-border/50" />}
              </>
            )}
            {/* Rest of agents */}
            {filteredRest.map((soul) => (
              <AgentItem key={soul.id} soul={soul} />
            ))}
          </>
        )}
      </div>
    </motion.div>
  );
}

// ── Hook: resolve default agent (COO/L1) ──────────────────────────────────────
function useDefaultAgent() {
  const souls = useStore((s) => s.souls);
  const [defaultAgent, setDefaultAgent] = useState<Soul | null>(null);

  useEffect(() => {
    agentsApi.chief()
      .then((c) => { if (c?.id) setDefaultAgent(c as Soul); })
      .catch(() => {});
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    if (!defaultAgent && souls.length > 0) setDefaultAgent(souls[0] as Soul);
  }, [souls, defaultAgent]);

  return defaultAgent;
}

// ── Public export ─────────────────────────────────────────────────────────────
export function AgentVoicePill() {
  const { enabled: voiceEnabled, loading: voiceLoading } = useVoiceEnabled();
  if (voiceLoading) return null;
  if (!voiceEnabled) return <PillBasic />;
  return <PillWithVoice />;
}

// ── Shared pill shell ─────────────────────────────────────────────────────────
function PillShell({ children }: { children: React.ReactNode }) {
  return (
    <div
      className="fixed z-29 flex flex-col border-t border-r border-border bg-muted hidden lg:flex"
      style={{
        left: 'var(--rail-width)',
        width: 'var(--sidebar-default-width, 280px)',
        bottom: 0,
        height: 'var(--agent-pill-height, 84px)',
      }}
    >
      {children}
    </div>
  );
}

// ── Row 1: agent switcher + action buttons ────────────────────────────────────
interface AgentRowProps {
  agent: Soul;
  soulStates: Record<string, any>;
  allSouls: Soul[];
  onAgentChange: (soul: Soul) => void;
  isVoiceActive: boolean;
  voiceEnabled: boolean;
  onMic: () => void;
  onChat: () => void;
}

function AgentRow({ agent, soulStates, allSouls, onAgentChange, isVoiceActive, voiceEnabled, onMic, onChat }: AgentRowProps) {
  const [open, setOpen] = useState(false);
  const gradient = gradientFor(agent.id);
  const state = soulStates[agent.id];
  const activity: SoulActivity = (state?.activity as SoulActivity) ?? 'offline';

  return (
    <div className="relative flex flex-1 items-center gap-2 px-3">
      {/* Switcher button — avatar + name + chevron */}
      <button
        onClick={() => setOpen((v) => !v)}
        className="flex flex-1 items-center gap-2 min-w-0 rounded-md px-1 py-1 hover:bg-accent/60 transition-colors group"
      >
        {/* Avatar */}
        {agent.avatar ? (
          <img src={agent.avatar} alt={agent.display_name} className="h-6 w-6 rounded-full object-cover shrink-0" />
        ) : (
          <div className={cn('flex h-6 w-6 items-center justify-center rounded-full bg-gradient-to-br text-[10px] font-bold text-white shrink-0', gradient)}>
            {(agent.display_name?.[0] ?? '?').toUpperCase()}
          </div>
        )}
        {/* Name + activity */}
        <div className="flex-1 min-w-0 text-left">
          <div className="flex items-center gap-1.5 min-w-0">
            <span className="truncate text-[12px] font-semibold text-foreground leading-tight">{agent.display_name}</span>
            <span className={cn('h-1.5 w-1.5 rounded-full shrink-0', activityDotColor(activity))} />
          </div>
          <p className="text-[10px] text-muted-foreground/60 leading-tight truncate">
            {activityLabel(activity)}
          </p>
        </div>
        {/* Chevron */}
        <ChevronDown className={cn('h-3.5 w-3.5 text-muted-foreground/50 shrink-0 transition-transform', open && 'rotate-180')} />
      </button>

      {/* Action buttons */}
      <div className="flex items-center gap-1 shrink-0">
        {voiceEnabled && (
          <button
            onClick={onMic}
            title={isVoiceActive ? 'End voice session' : `Talk to ${agent.display_name}`}
            className={cn(
              'flex h-7 w-7 items-center justify-center rounded-full transition-all',
              isVoiceActive
                ? 'bg-destructive text-white shadow-sm shadow-destructive/30'
                : 'bg-primary/12 text-primary hover:bg-primary/20',
            )}
          >
            {isVoiceActive ? <MicOff className="h-3.5 w-3.5" /> : <Mic className="h-3.5 w-3.5" />}
          </button>
        )}
        <button
          onClick={onChat}
          title={`Open ${agent.display_name}'s chat`}
          className="flex h-7 w-7 items-center justify-center rounded-full bg-muted/80 text-muted-foreground/60 hover:text-foreground hover:bg-accent transition-colors"
        >
          <MessageSquare className="h-3.5 w-3.5" />
        </button>
      </div>

      {/* Dropdown */}
      <AnimatePresence>
        {open && (
          <SwitcherDropdown
            souls={allSouls}
            soulStates={soulStates}
            selectedId={agent.id}
            onSelect={onAgentChange}
            onClose={() => setOpen(false)}
          />
        )}
      </AnimatePresence>
    </div>
  );
}

// ── Voice-disabled variant ────────────────────────────────────────────────────
function PillBasic() {
  const router = useRouter();
  const defaultAgent = useDefaultAgent();
  const souls = useStore((s) => s.souls);
  const soulStates = useStore((s) => s.soulStates);
  const [agent, setAgent] = useState<Soul | null>(null);

  useEffect(() => {
    if (defaultAgent && !agent) setAgent(defaultAgent);
  }, [defaultAgent, agent]);

  if (!agent) return null;

  return (
    <PillShell>
      <AgentRow
        agent={agent}
        soulStates={soulStates}
        allSouls={souls}
        onAgentChange={setAgent}
        isVoiceActive={false}
        voiceEnabled={false}
        onMic={() => {}}
        onChat={() => router.push(`/qors/${agent.id}`)}
      />
      {/* No voice row when disabled */}
    </PillShell>
  );
}

// ── Voice-enabled variant ─────────────────────────────────────────────────────
function PillWithVoice() {
  const router = useRouter();
  const defaultAgent = useDefaultAgent();
  const souls = useStore((s) => s.souls);
  const soulStates = useStore((s) => s.soulStates);
  const activeVoiceAgentId = useStore((s) => s.activeVoiceAgentId);
  const setActiveVoiceAgent = useStore((s) => s.setActiveVoiceAgent);
  const [agent, setAgent] = useState<Soul | null>(null);

  useEffect(() => {
    if (defaultAgent && !agent) setAgent(defaultAgent);
  }, [defaultAgent, agent]);

  const agentId = agent?.id ?? '';
  const isVoiceActive = activeVoiceAgentId === agentId && !!agentId;
  const voice = useVoice({ agentId: agentId || '__noop__' });

  useEffect(() => {
    if (!agentId) return;
    agentVoiceRegistry.set(agentId, voice.stop);
    return () => { agentVoiceRegistry.delete(agentId); };
  }, [agentId, voice.stop]);

  const handleMic = useCallback(async () => {
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

  // When user switches agent while voice is active — stop first
  const handleAgentChange = useCallback(async (soul: Soul) => {
    if (isVoiceActive) {
      await voice.stop();
      setActiveVoiceAgent(null);
      await new Promise((r) => setTimeout(r, 80));
    }
    setAgent(soul);
  }, [isVoiceActive, voice, setActiveVoiceAgent]);

  if (!agent) return null;

  return (
    <PillShell>
      {/* Row 1: switcher + mic + chat */}
      <AgentRow
        agent={agent}
        soulStates={soulStates}
        allSouls={souls}
        onAgentChange={handleAgentChange}
        isVoiceActive={isVoiceActive}
        voiceEnabled={true}
        onMic={handleMic}
        onChat={() => router.push(`/qors/${agent.id}`)}
      />

      {/* Row 2: voice orb — only when active */}
      <AnimatePresence>
        {isVoiceActive && (
          <motion.div
            initial={{ opacity: 0, height: 0 }}
            animate={{ opacity: 1, height: 'auto' }}
            exit={{ opacity: 0, height: 0 }}
            transition={{ duration: 0.2 }}
            className="border-t border-border/50 px-3 overflow-hidden"
          >
            <VoiceOrb voiceState={voice.state} volume={voice.volume} />
          </motion.div>
        )}
      </AnimatePresence>
    </PillShell>
  );
}
