'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useState, useRef, useEffect } from 'react';
import { ArrowRight, Loader2, Sparkles, FileText, Zap, DollarSign, Clock } from 'lucide-react';
import { projectBriefs as api } from '@/lib/api';
import type { ProjectBrief, ProjectQuality } from '@/types';
import { cn } from '@/lib/utils';

interface Props {
  onCreate: (brief: ProjectBrief) => void;
}

const MODES = [
  { id: 'vibe',  icon: Sparkles,  label: 'Vibe Code',   hint: 'Describe it, Prime builds it',   quality: 'mvp'        as ProjectQuality, placeholder: 'A real-time analytics dashboard for e-commerce...' },
  { id: 'spec',  icon: FileText,  label: 'Spec First',  hint: 'Plan requirements, then execute', quality: 'production' as ProjectQuality, placeholder: 'Multi-tenant SaaS with billing, auth, and admin panel...' },
  { id: 'ship',  icon: Zap,       label: 'Ship Fast',   hint: 'MVP in one focused session',      quality: 'mvp'        as ProjectQuality, placeholder: 'Landing page with email capture and Stripe checkout...' },
] as const;
type Mode = (typeof MODES)[number]['id'];

const BUDGET_OPTIONS = [
  { label: '$10',  cents: 1000  },
  { label: '$25',  cents: 2500  },
  { label: '$50',  cents: 5000  },
  { label: '$100', cents: 10000 },
  { label: '$250', cents: 25000 },
  { label: 'No limit', cents: 0 },
];

const TIMELINE_OPTIONS = [
  { label: 'Today',      value: 'today'      },
  { label: 'This week',  value: 'this_week'  },
  { label: 'This month', value: 'this_month' },
  { label: 'No rush',    value: 'no_rush'    },
];

const SUGGESTIONS = [
  'REST API with JWT auth and role-based access',
  'Real-time collaboration app with WebSockets',
  'Admin dashboard with charts, tables, and filters',
  'CLI tool with subcommands and config file support',
];

export function CodeLanding({ onCreate }: Props) {
  const [input, setInput]       = useState('');
  const [mode, setMode]         = useState<Mode>('vibe');
  const [budget, setBudget]     = useState<number>(5000);   // cents
  const [timeline, setTimeline] = useState('this_week');
  const [creating, setCreating] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => { inputRef.current?.focus(); }, []);

  const selectedMode = MODES.find(m => m.id === mode)!;

  const handleSubmit = async () => {
    if (!input.trim() || creating) return;
    setCreating(true);
    try {
      const brief = await api.create({
        title:        'New Project',
        idea:         input.trim(),
        quality:      selectedMode.quality,
        budget_cents: budget,
        timeline,
      });
      onCreate(brief);
    } catch {
      setCreating(false);
    }
  };

  return (
    <div className="flex h-full flex-col items-center justify-between px-10 py-10">

      {/* Heading */}
      <div className="flex flex-1 flex-col items-center justify-center gap-8 w-full max-w-[680px]">

        <div className="text-center space-y-1.5">
          <h2 className="text-2xl font-bold tracking-tight">New project</h2>
          <p className="text-sm text-muted-foreground">
            Prime plans the architecture, assembles a team, and ships — you approve at every stage.
          </p>
        </div>

        {/* Mode cards */}
        <div className="grid grid-cols-3 gap-2.5 w-full">
          {MODES.map(m => {
            const Icon = m.icon;
            const active = mode === m.id;
            return (
              <button
                key={m.id}
                onClick={() => { setMode(m.id); inputRef.current?.focus(); }}
                className={cn(
                  'qr-card flex flex-col items-start gap-2.5 p-4 text-left transition-all cursor-pointer',
                  active
                    ? 'border-primary/60 bg-primary/5 ring-1 ring-primary/20'
                    : 'hover:border-border/70 hover:bg-muted/20'
                )}
              >
                <Icon className={cn('h-3.5 w-3.5', active ? 'text-primary' : 'text-muted-foreground')} />
                <div className="space-y-0.5">
                  <p className={cn('text-xs font-semibold', active ? 'text-foreground' : 'text-muted-foreground/80')}>
                    {m.label}
                  </p>
                  <p className="text-2xs text-muted-foreground leading-snug">{m.hint}</p>
                </div>
              </button>
            );
          })}
        </div>

        {/* Budget + Timeline inline selectors */}
        <div className="flex items-center gap-6 w-full">

          <div className="flex flex-col gap-1.5 flex-1">
            <div className="flex items-center gap-1.5 text-2xs font-medium text-muted-foreground uppercase tracking-wider">
              <DollarSign className="h-3 w-3" />
              Budget
            </div>
            <div className="flex flex-wrap gap-1.5">
              {BUDGET_OPTIONS.map(b => (
                <button
                  key={b.cents}
                  onClick={() => setBudget(b.cents)}
                  className={cn(
                    'rounded-md border px-3 py-1 text-xs font-medium transition-all',
                    budget === b.cents
                      ? 'border-primary/60 bg-primary/10 text-primary'
                      : 'border-border bg-muted/20 text-muted-foreground hover:border-border/70 hover:text-foreground'
                  )}
                >
                  {b.label}
                </button>
              ))}
            </div>
          </div>

          <div className="w-px self-stretch bg-border/50" />

          <div className="flex flex-col gap-1.5 flex-1">
            <div className="flex items-center gap-1.5 text-2xs font-medium text-muted-foreground uppercase tracking-wider">
              <Clock className="h-3 w-3" />
              Timeline
            </div>
            <div className="flex flex-wrap gap-1.5">
              {TIMELINE_OPTIONS.map(t => (
                <button
                  key={t.value}
                  onClick={() => setTimeline(t.value)}
                  className={cn(
                    'rounded-md border px-3 py-1 text-xs font-medium transition-all',
                    timeline === t.value
                      ? 'border-primary/60 bg-primary/10 text-primary'
                      : 'border-border bg-muted/20 text-muted-foreground hover:border-border/70 hover:text-foreground'
                  )}
                >
                  {t.label}
                </button>
              ))}
            </div>
          </div>

        </div>

        {/* Quick-start suggestions */}
        <div className="flex flex-wrap gap-2 justify-center">
          {SUGGESTIONS.map(s => (
            <button
              key={s}
              onClick={() => { setInput(s); inputRef.current?.focus(); }}
              className="rounded-full border border-border bg-transparent px-3 py-1 text-xs text-muted-foreground hover:border-primary/40 hover:text-foreground hover:bg-primary/5 transition-colors"
            >
              {s}
            </button>
          ))}
        </div>

      </div>

      {/* Bottom input bar */}
      <div className="w-full max-w-[680px] shrink-0 space-y-1.5">
        <div className={cn(
          'flex items-center gap-3 rounded-xl border bg-muted/20 px-4 py-3 transition-all',
          'focus-within:border-primary/40 focus-within:bg-background focus-within:shadow-lg focus-within:shadow-primary/5'
        )}>
          <input
            ref={inputRef}
            value={input}
            onChange={e => setInput(e.target.value)}
            onKeyDown={e => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); handleSubmit(); } }}
            placeholder={selectedMode.placeholder}
            disabled={creating}
            className="flex-1 bg-transparent text-sm outline-none placeholder:text-muted-foreground/35 disabled:opacity-50"
          />
          <button
            onClick={handleSubmit}
            disabled={!input.trim() || creating}
            className="qr-btn qr-btn-primary qr-btn-sm shrink-0"
          >
            {creating
              ? <Loader2 className="h-3.5 w-3.5 animate-spin" />
              : <ArrowRight className="h-3.5 w-3.5" />}
          </button>
        </div>
        <p className="text-center text-2xs text-muted-foreground/35">
          Prime reviews the brief, proposes a spec and team, and waits for your approval before executing.
        </p>
      </div>

    </div>
  );
}
