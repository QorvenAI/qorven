'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

/**
 * /council — multi-model consensus (T2.1).
 *
 * Multi-model council: N drafts in parallel → reciprocal ranking →
 * chairman synthesizes a verdict. Backend returns the full record
 * synchronously (stage1 + stage2 + synthesis), so this page simulates
 * the three phases with a progress strip while the POST is in flight
 * and then fans the result out as a transcript.
 *
 * Non-goals: this is NOT a chat — there's no follow-up turn. Each
 * query is an isolated council session.
 */

import { useEffect, useMemo, useRef, useState } from 'react';
import { cn } from '@/lib/utils';
import {
  Brain, Sparkles, Crown, Send, Loader2, AlertCircle, ChevronDown,
  Zap, Scale, Layers, Infinity as InfinityIcon, Users, Trophy, CircleDot,
} from 'lucide-react';
import {
  council,
  type CouncilConfig,
  type CouncilDepth,
  type CouncilDraft,
  type CouncilRanking,
  type CouncilResult,
} from '@/lib/api';

// Depth metadata drives the selector card. Keep keys in sync with
// CouncilDepth in lib/api.ts.
const DEPTHS: {
  key: CouncilDepth;
  label: string;
  subtitle: string;
  icon: typeof Zap;
}[] = [
  { key: 'quick',    label: 'Quick',    subtitle: 'One model · fast',             icon: Zap },
  { key: 'balanced', label: 'Balanced', subtitle: 'Default · 3 drafts',           icon: Scale },
  { key: 'deep',     label: 'Deep',     subtitle: 'Full council · ranked',        icon: Layers },
  { key: 'max',      label: 'Max',      subtitle: 'Full council · chairman synth', icon: InfinityIcon },
];

// Backend is synchronous; we fake three beats of progress while the
// POST is in flight. Purely UX. Real durations come from the response.
const PHASE_STEPS = [
  { id: 'drafting',  label: 'Drafting parallel responses' },
  { id: 'ranking',   label: 'Models ranking each other'   },
  { id: 'synthesis', label: 'Chairman synthesizing verdict' },
] as const;

export default function CouncilPage() {
  const [query, setQuery] = useState('');
  const [depth, setDepth] = useState<CouncilDepth>('balanced');
  const [phase, setPhase] = useState<number>(-1); // -1 idle, 0..2 while running
  const [result, setResult] = useState<CouncilResult | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [cfg, setCfg] = useState<CouncilConfig | null>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  // Prefetch config so the roster chip is informative before first run.
  useEffect(() => {
    council.config().then(setCfg).catch(() => {
      // Silent — /council/config is informational; the run endpoint
      // doesn't depend on it.
    });
  }, []);

  const running = phase >= 0;
  const canSubmit = query.trim().length > 0 && !running;

  const submit = async () => {
    if (!canSubmit) return;
    setError(null);
    setResult(null);
    setPhase(0);

    // Tick the visual progress forward so the user sees motion even
    // though the backend doesn't stream. Each beat clamped at ~1.2s —
    // if the backend responds before phase 2, we jump there.
    const timers: number[] = [];
    timers.push(window.setTimeout(() => setPhase((p) => (p < 1 ? 1 : p)), 1200));
    timers.push(window.setTimeout(() => setPhase((p) => (p < 2 ? 2 : p)), 2600));

    try {
      const res = await council.run({ query: query.trim(), depth });
      setResult(res);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Council failed');
    } finally {
      timers.forEach(clearTimeout);
      setPhase(-1);
    }
  };

  const seed = (s: string) => {
    setQuery(s);
    textareaRef.current?.focus();
  };

  return (
    <div className="mx-auto max-w-5xl space-y-5 p-4 lg:p-6">
      <header className="flex items-start justify-between gap-4">
        <div>
          <h1 className="flex items-center gap-2 text-lg font-semibold">
            <Brain className="h-6 w-6 text-primary" />
            Council
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Ask something hard. Multiple models draft in parallel, rank each
            other, and a chairman synthesizes the verdict.
          </p>
        </div>
        {cfg && (
          <div className="hidden min-w-[220px] flex-col gap-0.5 rounded-md border border-border bg-muted/30 px-2.5 py-1.5 text-2xs text-muted-foreground lg:flex">
            <span className="font-medium text-foreground/80">Default roster</span>
            <span className="font-mono">{cfg.default.members.join(' · ')}</span>
            <span className="mt-0.5">
              Chairman <span className="font-mono text-foreground/80">{cfg.default.chairman}</span>
            </span>
          </div>
        )}
      </header>

      <DepthSelector value={depth} onChange={setDepth} disabled={running} />

      <div className="flex flex-col gap-2 rounded-xl border border-border bg-card p-3 focus-within:border-primary/60">
        <textarea
          ref={textareaRef}
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
              e.preventDefault();
              submit();
            }
          }}
          placeholder="Ask a hard question that benefits from multiple perspectives…"
          rows={3}
          disabled={running}
          className="w-full resize-none bg-transparent text-sm outline-none placeholder:text-muted-foreground/60 disabled:opacity-60"
        />
        <div className="flex items-center justify-between gap-2 border-t border-border/60 pt-2">
          <span className="text-2xs text-muted-foreground">
            <kbd className="rounded bg-muted px-1 py-0.5 font-mono text-xs">⌘ Enter</kbd> to run
          </span>
          <button
            onClick={submit}
            disabled={!canSubmit}
            className={cn(
              'inline-flex items-center gap-1.5 rounded-md px-3 py-1.5 text-xs font-medium transition-colors',
              'bg-primary text-primary-foreground hover:bg-primary/90',
              'disabled:opacity-50 disabled:cursor-not-allowed',
            )}
          >
            {running ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Send className="h-3.5 w-3.5" />}
            {running ? 'Deliberating' : 'Run council'}
          </button>
        </div>
      </div>

      {error && (
        <div className="flex items-start gap-2 rounded-lg border border-destructive/40 bg-destructive/5 p-3 text-xs text-destructive">
          <AlertCircle className="mt-0.5 h-4 w-4 shrink-0" />
          <div className="flex-1">
            <p className="font-medium">Council failed</p>
            <p className="mt-0.5 text-destructive/80">{error}</p>
          </div>
        </div>
      )}

      {running && <PhaseStrip phase={phase} />}

      {result && <Verdict result={result} depth={depth} />}

      {!running && !result && !error && <EmptyHero onSeed={seed} />}
    </div>
  );
}

// ───────────────────────────────────────────────────────────────────
// Depth selector — four cards. The inverted/outlined state is the
// single strongest "this is a different surface from /chat" cue.
// ───────────────────────────────────────────────────────────────────
function DepthSelector({
  value,
  onChange,
  disabled,
}: {
  value: CouncilDepth;
  onChange: (d: CouncilDepth) => void;
  disabled?: boolean;
}) {
  return (
    <div className="grid grid-cols-2 gap-2 lg:grid-cols-4">
      {DEPTHS.map(({ key, label, subtitle, icon: Icon }) => {
        const active = value === key;
        return (
          <button
            key={key}
            onClick={() => !disabled && onChange(key)}
            disabled={disabled}
            className={cn(
              'group relative flex items-start gap-2.5 rounded-xl border p-3 text-left transition-colors',
              active
                ? 'border-primary bg-primary/10'
                : 'border-border bg-card hover:border-border/80 hover:bg-accent/40',
              disabled && 'cursor-not-allowed opacity-60',
            )}
          >
            <Icon className={cn('mt-0.5 h-4 w-4 shrink-0', active ? 'text-primary' : 'text-muted-foreground')} />
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-1.5">
                <span className={cn('text-sm font-medium', active ? 'text-foreground' : 'text-foreground/80')}>
                  {label}
                </span>
                {active && <span className="font-mono text-xs uppercase text-primary">selected</span>}
              </div>
              <p className="mt-0.5 text-2xs text-muted-foreground">{subtitle}</p>
            </div>
          </button>
        );
      })}
    </div>
  );
}

function PhaseStrip({ phase }: { phase: number }) {
  return (
    <div className="flex items-center gap-2 rounded-xl border border-border bg-card px-4 py-3">
      {PHASE_STEPS.map((step, i) => {
        const done = i < phase;
        const active = i === phase;
        return (
          <div key={step.id} className="flex min-w-0 flex-1 items-center gap-2">
            <div
              className={cn(
                'flex h-6 w-6 shrink-0 items-center justify-center rounded-full border',
                done && 'border-emerald-500 bg-emerald-500/15 text-emerald-500',
                active && 'border-primary bg-primary/15 text-primary',
                !done && !active && 'border-border text-muted-foreground',
              )}
            >
              {done ? (
                <CircleDot className="h-3 w-3" />
              ) : active ? (
                <Loader2 className="h-3 w-3 animate-spin" />
              ) : (
                <span className="font-mono text-2xs">{i + 1}</span>
              )}
            </div>
            <span
              className={cn(
                'truncate text-xs',
                done && 'text-emerald-500/80',
                active && 'text-foreground',
                !done && !active && 'text-muted-foreground/60',
              )}
            >
              {step.label}
            </span>
            {i < PHASE_STEPS.length - 1 && <div className="h-px flex-1 bg-border/60" />}
          </div>
        );
      })}
    </div>
  );
}

function Verdict({ result, depth }: { result: CouncilResult; depth: CouncilDepth }) {
  return (
    <div className="space-y-4">
      {/* Synthesis — hero card */}
      <section className="rounded-xl border border-primary/30 bg-primary/5 p-5">
        <div className="flex items-center gap-2">
          <Crown className="h-4 w-4 text-amber-400" />
          <h2 className="text-sm font-semibold uppercase tracking-wider text-primary">Verdict</h2>
          <span className="ml-auto flex items-center gap-2 font-mono text-2xs text-muted-foreground">
            <Sparkles className="h-3 w-3" />
            {(result.duration / 1000).toFixed(1)}s
            <span>·</span>
            {result.tokens_used.toLocaleString()} tok
            <span>·</span>
            depth={depth}
          </span>
        </div>
        <p className="mt-3 whitespace-pre-wrap text-sm leading-relaxed text-foreground/90">
          {result.synthesis}
        </p>
      </section>

      <CollapsibleSection
        icon={Users}
        title={`Parallel drafts (${result.stage1.length})`}
        subtitle="Each model answered independently"
        defaultOpen
      >
        <div className="grid gap-3 lg:grid-cols-2">
          {result.stage1.map((d) => (
            <DraftCard key={d.label} draft={d} />
          ))}
        </div>
      </CollapsibleSection>

      {result.gate_skipped ? (
        <div className="flex items-center gap-2 rounded-md border border-emerald-500/30 bg-emerald-500/5 px-3 py-2 text-xs text-emerald-400">
          <Trophy className="h-3.5 w-3.5" />
          <span>High agreement — ranking stage skipped. Drafts were close enough that the chairman synthesized directly.</span>
        </div>
      ) : (
        <CollapsibleSection
          icon={Trophy}
          title={`Rankings (${result.stage2.length})`}
          subtitle="Each model ordered the others by quality"
        >
          <div className="space-y-2">
            {result.stage2.map((r) => (
              <RankingCard key={r.ranker} ranking={r} />
            ))}
          </div>
        </CollapsibleSection>
      )}
    </div>
  );
}

function DraftCard({ draft }: { draft: CouncilDraft }) {
  return (
    <div className="rounded-xl border border-border bg-card p-4">
      <header className="flex items-center justify-between gap-2 border-b border-border/60 pb-2">
        <div className="flex min-w-0 items-center gap-2">
          <span className="rounded-sm bg-primary/10 px-1.5 py-0.5 font-mono text-xs font-semibold text-primary">
            {draft.label}
          </span>
          <span className="truncate font-mono text-xs text-muted-foreground" title={draft.model}>
            {draft.model}
          </span>
        </div>
        <span className="shrink-0 font-mono text-2xs text-muted-foreground">
          {(draft.duration / 1000).toFixed(1)}s · {draft.tokens} tok
        </span>
      </header>
      <p className="mt-3 whitespace-pre-wrap text-xs leading-relaxed text-foreground/80">
        {draft.response}
      </p>
    </div>
  );
}

function RankingCard({ ranking }: { ranking: CouncilRanking }) {
  return (
    <div className="rounded-lg border border-border bg-card p-3">
      <header className="flex items-center gap-2">
        <span className="rounded-sm bg-muted px-1.5 py-0.5 font-mono text-xs uppercase tracking-wider text-muted-foreground">
          ranker
        </span>
        <span className="font-mono text-xs text-foreground/80">{ranking.ranker}</span>
        <span className="ml-auto flex items-center gap-1 text-2xs">
          {ranking.ranking.map((label, i) => (
            <span
              key={label}
              className={cn(
                'rounded-sm px-1.5 py-0.5 font-mono',
                i === 0 && 'bg-amber-400/15 text-amber-400',
                i === 1 && 'bg-muted text-foreground/70',
                i > 1 && 'bg-muted/50 text-muted-foreground',
              )}
            >
              {i === 0 && '🥇 '}
              {label}
            </span>
          ))}
        </span>
      </header>
      {ranking.reason && (
        <p className="mt-2 text-2xs italic leading-relaxed text-muted-foreground">
          “{ranking.reason}”
        </p>
      )}
    </div>
  );
}

function CollapsibleSection({
  icon: Icon,
  title,
  subtitle,
  defaultOpen,
  children,
}: {
  icon: typeof Users;
  title: string;
  subtitle?: string;
  defaultOpen?: boolean;
  children: React.ReactNode;
}) {
  const [open, setOpen] = useState(!!defaultOpen);
  return (
    <section className="rounded-xl border border-border bg-card/40">
      <button
        onClick={() => setOpen((v) => !v)}
        className="flex w-full items-center gap-2 px-4 py-2.5 text-left transition-colors hover:bg-accent/40"
      >
        <Icon className="h-4 w-4 text-muted-foreground" />
        <div className="min-w-0 flex-1">
          <p className="text-sm font-medium">{title}</p>
          {subtitle && <p className="text-2xs text-muted-foreground">{subtitle}</p>}
        </div>
        <ChevronDown className={cn('h-4 w-4 text-muted-foreground transition-transform', open && 'rotate-180')} />
      </button>
      {open && <div className="border-t border-border/60 p-4">{children}</div>}
    </section>
  );
}

function EmptyHero({ onSeed }: { onSeed: (s: string) => void }) {
  const SEEDS = useMemo(
    () => [
      'Why does Qorven separate the rail into "primary" and "advanced" sections?',
      'Which of these three migrations should land first, and why?',
      'Compare TinyGo vs Go-wasip1 for plugin builds — what trade-offs matter?',
    ],
    [],
  );
  return (
    <div className="flex flex-col items-center gap-4 rounded-xl border border-dashed border-border/60 bg-card/40 px-6 py-10 text-center">
      <div className="flex h-12 w-12 items-center justify-center rounded-full bg-primary/10 text-primary">
        <Brain className="h-6 w-6" />
      </div>
      <div>
        <p className="text-sm font-medium">Ready to deliberate</p>
        <p className="mt-1 text-xs text-muted-foreground">
          The council shines on judgment calls — architecture trade-offs, high-stakes
          debugging, anywhere you want more than one opinion before acting.
        </p>
      </div>
      <div className="flex flex-wrap justify-center gap-2">
        {SEEDS.map((s) => (
          <button
            key={s}
            onClick={() => onSeed(s)}
            className="rounded-full border border-border/80 bg-background/80 px-3 py-1 text-2xs text-muted-foreground transition-colors hover:border-primary/40 hover:text-foreground"
          >
            {s.length > 80 ? s.slice(0, 80) + '…' : s}
          </button>
        ))}
      </div>
    </div>
  );
}
