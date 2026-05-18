'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

/**
 * /labs — Umbrella for experimental surfaces.
 *
 * Consolidates Scenarios, Sandbox, Training, Council, Research, and
 * Pipeline so the primary rail doesn't carry every half-baked idea.
 * Each entry links to its dedicated page; this page never owns state
 * beyond the catalog. Add a new tile here instead of a new rail icon
 * whenever a feature is pre-production.
 */

import Link from 'next/link';
import { Beaker, ArrowUpRight } from 'lucide-react';
import {
  FlaskConical, Box, GraduationCap, Users, Search, GitBranch,
  type LucideIcon,
} from 'lucide-react';

interface Experiment {
  href: string;
  title: string;
  description: string;
  icon: LucideIcon;
  tone: string; // Tailwind color stem, e.g. 'emerald', 'violet'
}

const experiments: Experiment[] = [
  {
    href: '/scenarios',
    title: 'Scenarios',
    description: 'Multi-agent simulation lab. Generate personas, run a session, read a report — never touches your real data.',
    icon: FlaskConical,
    tone: 'violet',
  },
  {
    href: '/sandbox',
    title: 'Sandbox',
    description: 'A throwaway chat with any Qor. Great for prompt-tuning and one-off experiments without polluting your session list.',
    icon: Box,
    tone: 'cyan',
  },
  {
    href: '/training',
    title: 'Training',
    description: 'Fine-tune prompts and skills against canonical examples. Track which variant wins before promoting it.',
    icon: GraduationCap,
    tone: 'emerald',
  },
  {
    href: '/council',
    title: 'Council',
    description: 'Multi-model consensus — run a prompt across models in parallel, rank the answers, pick a winner.',
    icon: Users,
    tone: 'amber',
  },
  {
    href: '/research',
    title: 'Research',
    description: 'Deep-research runs: iterative searches, citation capture, and a synthesis report at the end.',
    icon: Search,
    tone: 'blue',
  },
  {
    href: '/pipeline',
    title: 'Pipeline',
    description: 'Code change pipeline — propose, validate, apply. Experimental automation for Prime-authored edits.',
    icon: GitBranch,
    tone: 'fuchsia',
  },
];

const TONE_CLASSES: Record<string, { bg: string; text: string; border: string }> = {
  emerald: { bg: 'bg-emerald-500/10',  text: 'text-emerald-500',  border: 'hover:border-emerald-500/40' },
  violet:  { bg: 'bg-violet-500/10',   text: 'text-violet-500',   border: 'hover:border-violet-500/40' },
  cyan:    { bg: 'bg-cyan-500/10',     text: 'text-cyan-500',     border: 'hover:border-cyan-500/40' },
  amber:   { bg: 'bg-amber-500/10',    text: 'text-amber-500',    border: 'hover:border-amber-500/40' },
  blue:    { bg: 'bg-blue-500/10',     text: 'text-blue-500',     border: 'hover:border-blue-500/40' },
  fuchsia: { bg: 'bg-fuchsia-500/10',  text: 'text-fuchsia-500',  border: 'hover:border-fuchsia-500/40' },
};

export default function LabsPage() {
  return (
    <div className="space-y-5">
      <header className="flex items-start gap-3">
        <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-primary/10 text-primary shrink-0">
          <Beaker className="h-5 w-5" />
        </div>
        <div>
          <h1 className="text-lg font-semibold tracking-tight">Labs</h1>
          <p className="text-sm text-muted-foreground mt-1 max-w-2xl">
            Experimental surfaces — not production paths. Things land here first so we can learn from them
            without blocking the daily workflow.
          </p>
        </div>
      </header>

      <div className="rounded-md border border-amber-500/30 bg-amber-500/5 px-3 py-2 text-2xs text-amber-600">
        Heads up: anything under Labs can change behavior, schema, or disappear between releases.
      </div>

      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {experiments.map((e) => {
          const tone = TONE_CLASSES[e.tone] ?? TONE_CLASSES.violet!;
          const Icon = e.icon;
          return (
            <Link
              key={e.href}
              href={e.href}
              className={`group flex flex-col gap-3 rounded-xl border border-border bg-card p-4 transition-colors ${tone.border}`}
            >
              <div className="flex items-center justify-between">
                <div className={`flex h-9 w-9 items-center justify-center rounded-lg ${tone.bg} ${tone.text}`}>
                  <Icon className="h-4 w-4" />
                </div>
                <ArrowUpRight className="h-4 w-4 text-muted-foreground/40 group-hover:text-foreground transition-colors" />
              </div>
              <div>
                <h3 className="text-sm font-semibold">{e.title}</h3>
                <p className="mt-1 text-xs text-muted-foreground leading-relaxed">{e.description}</p>
              </div>
            </Link>
          );
        })}
      </div>
    </div>
  );
}
