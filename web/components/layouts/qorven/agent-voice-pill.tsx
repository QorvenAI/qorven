'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).
//
// AgentVoicePill — single-row persistent bar.
// Layout: [avatar/orb-pill] [name + designation + chevron] [mic] [voice-chat] [chat]
// When voice is active: avatar morphs into an expanding waveform pill (Distill-style).
// Mic/voice-chat disabled when voice not configured — hover shows tooltip.

import { useEffect, useState, useCallback, useRef } from 'react';
import { useRouter } from 'next/navigation';
import { motion, AnimatePresence } from 'motion/react';
import { ChevronDown, Mic, MicOff, MessageSquare, Search, PhoneCall } from 'lucide-react';
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

function activityDotColor(a: SoulActivity): string {
  return { running: 'bg-emerald-400', thinking: 'bg-amber-400', idle: 'bg-emerald-500/40', offline: 'bg-muted-foreground/20', error: 'bg-destructive' }[a] ?? 'bg-muted-foreground/20';
}

// ── Waveform pill — expands from avatar position when voice active ─────────────
// Inspired by Distill's sidebar voice indicator
function WaveformPill({ voiceState, volume = 0 }: { voiceState: string; volume?: number }) {
  const bars = [0.4, 0.7, 1, 0.8, 0.6, 0.9, 0.5, 0.7, 0.4];

  const barColor = voiceState === 'listening'
    ? 'bg-emerald-400'
    : voiceState === 'processing'
      ? 'bg-amber-400'
      : 'bg-primary';

  return (
    <motion.div
      initial={{ width: '28px', borderRadius: '50%' }}
      animate={{ width: '72px', borderRadius: '8px' }}
      exit={{ width: '28px', borderRadius: '50%' }}
      transition={{ duration: 0.25, ease: [0.4, 0, 0.2, 1] }}
      className="flex h-7 shrink-0 items-center justify-center gap-[2px] overflow-hidden bg-primary/10 border border-primary/20 px-2"
    >
      {voiceState === 'processing' ? (
        // Dots for thinking
        [0, 1, 2].map((i) => (
          <motion.span
            key={i}
            className="w-[3px] h-[3px] rounded-full bg-amber-400"
            animate={{ y: [0, -3, 0], opacity: [0.4, 1, 0.4] }}
            transition={{ duration: 0.6, repeat: Infinity, delay: i * 0.15, ease: 'easeInOut' }}
          />
        ))
      ) : (
        // Bars for listening/speaking
        bars.map((weight, i) => {
          const maxH = 14;
          const minH = 2;
          const h = volume > 0.02
            ? Math.max(minH, Math.round(minH + (maxH - minH) * volume * weight))
            : undefined;
          return (
            <motion.span
              key={i}
              className={cn('w-[2px] rounded-full', barColor)}
              animate={h
                ? { height: `${h}px` }
                : { height: [`${minH}px`, `${Math.round(maxH * weight)}px`, `${minH}px`] }
              }
              transition={h
                ? { duration: 0.08, ease: 'easeOut' }
                : { duration: 0.5 + i * 0.03, repeat: Infinity, delay: i * 0.06, ease: 'easeInOut' }
              }
            />
          );
        })
      )}
    </motion.div>
  );
}

// ── Agent avatar (idle) or waveform pill (voice active) ───────────────────────
function AgentAvatar({
  soul, isVoiceActive, voiceState, volume,
}: {
  soul: Soul;
  isVoiceActive: boolean;
  voiceState?: string;
  volume?: number;
}) {
  const gradient = gradientFor(soul.id);

  return (
    <div className="shrink-0">
      <AnimatePresence mode="wait">
        {isVoiceActive && voiceState ? (
          <WaveformPill key="waveform" voiceState={voiceState} volume={volume} />
        ) : (
          <motion.div
            key="avatar"
            initial={{ opacity: 0.8 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0.8 }}
            transition={{ duration: 0.15 }}
          >
            {soul.avatar ? (
              <img src={soul.avatar} alt={soul.display_name} className="h-7 w-7 rounded-full object-cover" />
            ) : (
              <div className={cn('flex h-7 w-7 items-center justify-center rounded-full bg-gradient-to-br text-[11px] font-bold text-white', gradient)}>
                {(soul.display_name?.[0] ?? '?').toUpperCase()}
              </div>
            )}
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  );
}

// ── Tooltip wrapper for disabled buttons ──────────────────────────────────────
function DisabledTooltip({ children, message }: { children: React.ReactNode; message: string }) {
  return (
    <div className="relative group/tip">
      {children}
      <div className="pointer-events-none absolute bottom-full mb-2 left-1/2 -translate-x-1/2 hidden group-hover/tip:flex whitespace-nowrap rounded-lg border border-border bg-popover px-2.5 py-1.5 text-[11px] text-foreground shadow-lg z-50">
        {message}
      </div>
    </div>
  );
}

// ── Searchable agent switcher dropdown ────────────────────────────────────────
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

  const coo = souls.find((s) => s.org_level === 'l1' || s.org_role === 'coo');
  const rest = souls.filter((s) => s.id !== coo?.id);
  const match = (s: Soul) => !query
    || s.display_name.toLowerCase().includes(query.toLowerCase())
    || (s.title || s.role || '').toLowerCase().includes(query.toLowerCase());

  const AgentRow = ({ soul, pinned }: { soul: Soul; pinned?: boolean }) => {
    const state = soulStates[soul.id];
    const activity: SoulActivity = (state?.activity as SoulActivity) ?? 'idle';
    const gradient = gradientFor(soul.id);
    const isSelected = soul.id === selectedId;
    const isActive = activity === 'running' || activity === 'thinking';
    const ringColor = activity === 'running' ? 'ring-emerald-400' : activity === 'thinking' ? 'ring-amber-400' : 'ring-transparent';
    const statusText = activity === 'running' ? 'Working now' : activity === 'thinking' ? 'Thinking…' : null;
    const statusColor = activity === 'running' ? 'text-emerald-400' : 'text-amber-400';

    return (
      <button
        onClick={() => { onSelect(soul); onClose(); }}
        className={cn(
          'flex w-full items-center gap-3 px-3 py-2.5 text-left transition-colors',
          isSelected ? 'bg-primary/10' : 'hover:bg-accent',
        )}
      >
        <div className="relative shrink-0">
          {soul.avatar ? (
            <img src={soul.avatar} alt={soul.display_name} className={cn('h-8 w-8 rounded-full object-cover ring-2', isActive ? ringColor : 'ring-transparent')} />
          ) : (
            <div className={cn('flex h-8 w-8 items-center justify-center rounded-full bg-gradient-to-br text-[11px] font-bold text-white ring-2', gradient, isActive ? ringColor : 'ring-transparent')}>
              {(soul.display_name?.[0] ?? '?').toUpperCase()}
            </div>
          )}
          <span className="absolute -bottom-0.5 -right-0.5 flex h-3 w-3 items-center justify-center">
            {isActive ? (
              <>
                <motion.span className={cn('absolute inline-flex h-2.5 w-2.5 rounded-full', activity === 'running' ? 'bg-emerald-400' : 'bg-amber-400')}
                  animate={{ scale: [1, 1.6, 1], opacity: [0.8, 0, 0.8] }}
                  transition={{ duration: 1.5, repeat: Infinity }} />
                <span className={cn('relative inline-flex h-2 w-2 rounded-full', activity === 'running' ? 'bg-emerald-400' : 'bg-amber-400')} />
              </>
            ) : (
              <span className="relative inline-flex h-2 w-2 rounded-full bg-emerald-500/40 ring-1 ring-background" />
            )}
          </span>
        </div>
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2 min-w-0">
            <p className="text-[12px] font-semibold truncate text-foreground/90">{soul.display_name}</p>
            {pinned && <span className="shrink-0 rounded text-[9px] font-bold px-1 py-0.5 bg-primary/15 text-primary uppercase tracking-wide">COO</span>}
          </div>
          <div className="flex items-center gap-1.5 mt-0.5">
            <p className="text-[10px] text-muted-foreground/60 truncate">{soul.title || soul.role || 'Agent'}</p>
            {statusText && <><span className="text-muted-foreground/30">·</span><p className={cn('text-[10px] shrink-0', statusColor)}>{statusText}</p></>}
          </div>
        </div>
      </button>
    );
  };

  const empty = !rest.concat(coo ? [coo] : []).some(match);

  return (
    <motion.div ref={ref}
      initial={{ opacity: 0, y: 6, scale: 0.97 }}
      animate={{ opacity: 1, y: 0, scale: 1 }}
      exit={{ opacity: 0, y: 4, scale: 0.97 }}
      transition={{ duration: 0.12 }}
      className="absolute bottom-full mb-2 left-0 w-full rounded-xl border border-border bg-popover shadow-xl z-50 overflow-hidden"
    >
      <div className="flex items-center gap-2 px-3 py-2 border-b border-border/60">
        <Search className="h-3.5 w-3.5 text-muted-foreground/50 shrink-0" />
        <input ref={inputRef} value={query} onChange={(e) => setQuery(e.target.value)}
          placeholder="Search agents…"
          className="flex-1 bg-transparent text-[12px] text-foreground placeholder:text-muted-foreground/50 outline-none" />
      </div>
      <div className="max-h-64 overflow-y-auto">
        {empty ? (
          <p className="px-3 py-4 text-[11px] text-muted-foreground text-center">No agents found</p>
        ) : (
          <>
            {coo && match(coo) && <><AgentRow soul={coo} pinned />{rest.some(match) && <div className="mx-3 h-px bg-border/50" />}</>}
            {rest.filter(match).map((s) => <AgentRow key={s.id} soul={s} />)}
          </>
        )}
      </div>
    </motion.div>
  );
}

// ── Hook: resolve default agent ───────────────────────────────────────────────
function useDefaultAgent() {
  const souls = useStore((s) => s.souls);
  const [def, setDef] = useState<Soul | null>(null);
  useEffect(() => { agentsApi.chief().then((c) => { if (c?.id) setDef(c as Soul); }).catch(() => {}); }, []); // eslint-disable-line
  useEffect(() => { if (!def && souls.length > 0) setDef(souls[0] as Soul); }, [souls, def]);
  return def;
}

// ── Shell ─────────────────────────────────────────────────────────────────────
function PillShell({ children }: { children: React.ReactNode }) {
  return (
    <div className="fixed z-29 flex items-center border-t border-r border-border bg-muted px-3 hidden lg:flex"
      style={{ left: 'var(--rail-width)', width: 'var(--sidebar-default-width, 280px)', bottom: 0, height: 'var(--agent-pill-height, 56px)' }}>
      {children}
    </div>
  );
}

// ── Public export ─────────────────────────────────────────────────────────────
export function AgentVoicePill() {
  const { enabled: voiceEnabled, loading: voiceLoading } = useVoiceEnabled();
  if (voiceLoading) return null;
  if (!voiceEnabled) return <PillNoVoice />;
  return <PillWithVoice />;
}

// ── Voice disabled ────────────────────────────────────────────────────────────
function PillNoVoice() {
  const router = useRouter();
  const def = useDefaultAgent();
  const souls = useStore((s) => s.souls);
  const soulStates = useStore((s) => s.soulStates);
  const [agent, setAgent] = useState<Soul | null>(null);
  const [open, setOpen] = useState(false);

  useEffect(() => { if (def && !agent) setAgent(def); }, [def, agent]);
  if (!agent) return null;

  const state = soulStates[agent.id];
  const activity: SoulActivity = (state?.activity as SoulActivity) ?? 'idle';
  const gradient = gradientFor(agent.id);

  return (
    <PillShell>
      {/* Switcher */}
      <div className="relative flex flex-1 items-center gap-2 min-w-0">
        <button onClick={() => setOpen((v) => !v)}
          className="flex flex-1 items-center gap-2 min-w-0 rounded-md px-1 py-1 hover:bg-accent/60 transition-colors">
          {agent.avatar
            ? <img src={agent.avatar} alt={agent.display_name} className="h-7 w-7 rounded-full object-cover shrink-0" />
            : <div className={cn('flex h-7 w-7 items-center justify-center rounded-full bg-gradient-to-br text-[11px] font-bold text-white shrink-0', gradient)}>{(agent.display_name?.[0] ?? '?').toUpperCase()}</div>
          }
          <div className="flex-1 min-w-0 text-left">
            <div className="flex items-center gap-1.5">
              <span className="truncate text-[12px] font-semibold text-foreground">{agent.display_name}</span>
              <span className={cn('h-1.5 w-1.5 rounded-full shrink-0', activityDotColor(activity))} />
            </div>
            <p className="text-[10px] text-muted-foreground/60 truncate leading-tight">{agent.title || agent.role || 'Agent'}</p>
          </div>
          <ChevronDown className={cn('h-3.5 w-3.5 text-muted-foreground/50 shrink-0 transition-transform', open && 'rotate-180')} />
        </button>

        {/* Action buttons — disabled with tooltip */}
        <div className="flex items-center gap-1 shrink-0">
          <DisabledTooltip message="Enable voice in Settings → Voice">
            <div className="flex h-7 w-7 items-center justify-center rounded-full bg-muted/60 text-muted-foreground/25 cursor-default">
              <Mic className="h-3.5 w-3.5" />
            </div>
          </DisabledTooltip>
          <DisabledTooltip message="Enable voice in Settings → Voice">
            <div className="flex h-7 w-7 items-center justify-center rounded-full bg-muted/60 text-muted-foreground/25 cursor-default">
              <PhoneCall className="h-3.5 w-3.5" />
            </div>
          </DisabledTooltip>
          <button onClick={() => router.push(`/qors/${agent.id}`)}
            className="flex h-7 w-7 items-center justify-center rounded-full bg-muted/80 text-muted-foreground/60 hover:text-foreground hover:bg-accent transition-colors">
            <MessageSquare className="h-3.5 w-3.5" />
          </button>
        </div>

        <AnimatePresence>
          {open && <SwitcherDropdown souls={souls} soulStates={soulStates} selectedId={agent.id} onSelect={setAgent} onClose={() => setOpen(false)} />}
        </AnimatePresence>
      </div>
    </PillShell>
  );
}

// ── Voice enabled ─────────────────────────────────────────────────────────────
function PillWithVoice() {
  const router = useRouter();
  const def = useDefaultAgent();
  const souls = useStore((s) => s.souls);
  const soulStates = useStore((s) => s.soulStates);
  const activeVoiceAgentId = useStore((s) => s.activeVoiceAgentId);
  const setActiveVoiceAgent = useStore((s) => s.setActiveVoiceAgent);
  const [agent, setAgent] = useState<Soul | null>(null);
  const [open, setOpen] = useState(false);

  useEffect(() => { if (def && !agent) setAgent(def); }, [def, agent]);

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
    if (isVoiceActive) { await voice.stop(); setActiveVoiceAgent(null); }
    else {
      if (activeVoiceAgentId) { const p = agentVoiceRegistry.get(activeVoiceAgentId); if (p) await p(); setActiveVoiceAgent(null); await new Promise(r => setTimeout(r, 80)); }
      await voice.start(); setActiveVoiceAgent(agentId);
    }
  }, [agentId, isVoiceActive, voice, activeVoiceAgentId, setActiveVoiceAgent]);

  const handleSwitch = useCallback(async (soul: Soul) => {
    if (isVoiceActive) { await voice.stop(); setActiveVoiceAgent(null); await new Promise(r => setTimeout(r, 80)); }
    setAgent(soul);
  }, [isVoiceActive, voice, setActiveVoiceAgent]);

  if (!agent) return null;

  const state = soulStates[agent.id];
  const activity: SoulActivity = (state?.activity as SoulActivity) ?? 'idle';
  const gradient = gradientFor(agent.id);

  return (
    <PillShell>
      <div className="relative flex flex-1 items-center gap-2 min-w-0">
        {/* Avatar → waveform pill morphs on voice active */}
        <AgentAvatar
          soul={agent}
          isVoiceActive={isVoiceActive}
          voiceState={isVoiceActive ? voice.state : undefined}
          volume={isVoiceActive ? voice.volume : 0}
        />

        {/* Name + designation + chevron — hidden when voice active to give waveform room */}
        <AnimatePresence>
          {!isVoiceActive && (
            <motion.button
              initial={{ opacity: 0, width: 0 }}
              animate={{ opacity: 1, width: 'auto' }}
              exit={{ opacity: 0, width: 0 }}
              transition={{ duration: 0.2 }}
              onClick={() => setOpen((v) => !v)}
              className="flex flex-1 items-center gap-1.5 min-w-0 rounded-md px-1 py-1 hover:bg-accent/60 transition-colors overflow-hidden"
            >
              <div className="flex-1 min-w-0 text-left">
                <div className="flex items-center gap-1.5">
                  <span className="truncate text-[12px] font-semibold text-foreground">{agent.display_name}</span>
                  <span className={cn('h-1.5 w-1.5 rounded-full shrink-0', activityDotColor(activity))} />
                </div>
                <p className="text-[10px] text-muted-foreground/60 truncate leading-tight">{agent.title || agent.role || 'Agent'}</p>
              </div>
              <ChevronDown className={cn('h-3.5 w-3.5 text-muted-foreground/50 shrink-0 transition-transform', open && 'rotate-180')} />
            </motion.button>
          )}
        </AnimatePresence>

        {/* Spacer when voice active */}
        {isVoiceActive && <div className="flex-1" />}

        {/* Action buttons */}
        <div className="flex items-center gap-1 shrink-0">
          {/* Mic toggle */}
          <button onClick={handleMic}
            title={isVoiceActive ? 'Mute mic' : `Start voice with ${agent.display_name}`}
            className={cn('flex h-7 w-7 items-center justify-center rounded-full transition-all',
              isVoiceActive ? 'bg-destructive text-white' : 'bg-primary/12 text-primary hover:bg-primary/20')}>
            {isVoiceActive ? <MicOff className="h-3.5 w-3.5" /> : <Mic className="h-3.5 w-3.5" />}
          </button>
          {/* End call — only when active, else phone icon */}
          <button onClick={isVoiceActive ? handleMic : undefined}
            title={isVoiceActive ? 'End voice session' : 'Voice chat'}
            className={cn('flex h-7 w-7 items-center justify-center rounded-full transition-all',
              isVoiceActive ? 'bg-destructive/15 text-destructive hover:bg-destructive/25' : 'bg-muted/80 text-muted-foreground/40 cursor-default')}>
            <PhoneCall className="h-3.5 w-3.5" />
          </button>
          {/* Chat */}
          <button onClick={() => router.push(`/qors/${agent.id}`)}
            className="flex h-7 w-7 items-center justify-center rounded-full bg-muted/80 text-muted-foreground/60 hover:text-foreground hover:bg-accent transition-colors">
            <MessageSquare className="h-3.5 w-3.5" />
          </button>
        </div>

        <AnimatePresence>
          {open && !isVoiceActive && (
            <SwitcherDropdown souls={souls} soulStates={soulStates} selectedId={agent.id} onSelect={handleSwitch} onClose={() => setOpen(false)} />
          )}
        </AnimatePresence>
      </div>
    </PillShell>
  );
}
