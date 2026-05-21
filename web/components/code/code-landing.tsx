'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useState, useRef, useEffect } from 'react';
import { ArrowRight, Loader2, Sparkles, FileText, FolderOpen, Zap } from 'lucide-react';
import { projectBriefs as api } from '@/lib/api';
import type { ProjectBrief } from '@/types';
import { cn } from '@/lib/utils';

interface Props {
  onCreate: (brief: ProjectBrief) => void;
}

const MODES = [
  {
    id: 'vibe',
    icon: Sparkles,
    label: 'Vibe Code',
    hint: 'Chat your way to a working app',
    placeholder: 'I want to build a task manager with real-time sync…',
  },
  {
    id: 'spec',
    icon: FileText,
    label: 'Spec First',
    hint: 'Plan requirements, then build',
    placeholder: 'SaaS app for managing restaurant reservations…',
  },
  {
    id: 'ship',
    icon: Zap,
    label: 'Ship Fast',
    hint: 'MVP in one focused session',
    placeholder: 'A landing page for my new product with email signup…',
  },
] as const;

type Mode = (typeof MODES)[number]['id'];

const SUGGESTIONS = [
  'A REST API with auth and CRUD endpoints',
  'A real-time chat app with rooms',
  'A dashboard with charts and filters',
  'A CLI tool for batch file processing',
];

export function CodeLanding({ onCreate }: Props) {
  const [input, setInput] = useState('');
  const [mode, setMode] = useState<Mode>('vibe');
  const [creating, setCreating] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    inputRef.current?.focus();
  }, []);

  const selectedMode = MODES.find(m => m.id === mode)!;

  const handleSubmit = async () => {
    if (!input.trim() || creating) return;
    setCreating(true);
    try {
      const brief = await api.create({
        title: 'New Project',
        idea: input.trim(),
        quality: mode === 'ship' ? 'mvp' : mode === 'spec' ? 'production' : 'mvp',
      });
      onCreate(brief);
    } catch {
      setCreating(false);
    }
  };

  return (
    <div className="flex h-full flex-col items-center justify-between px-8 py-12">

      {/* Upper: heading + mode cards */}
      <div className="flex flex-1 flex-col items-center justify-center gap-10 w-full max-w-2xl">

        <div className="text-center space-y-2">
          <h2 className="text-2xl font-bold tracking-tight">Let's build something.</h2>
          <p className="text-sm text-muted-foreground">
            Describe your idea — Prime will plan the architecture, assign agents, and ship it.
          </p>
        </div>

        {/* Mode selection cards */}
        <div className="grid grid-cols-3 gap-3 w-full">
          {MODES.map(m => {
            const Icon = m.icon;
            const active = mode === m.id;
            return (
              <button
                key={m.id}
                onClick={() => { setMode(m.id); inputRef.current?.focus(); }}
                className={cn(
                  'qr-card flex flex-col items-start gap-2 p-4 text-left transition-all cursor-pointer',
                  active
                    ? 'border-primary/60 bg-primary/5 shadow-sm shadow-primary/10'
                    : 'hover:border-border/80 hover:bg-muted/30'
                )}
              >
                <Icon className={cn('h-4 w-4', active ? 'text-primary' : 'text-muted-foreground')} />
                <div>
                  <p className={cn('text-xs font-semibold', active ? 'text-foreground' : 'text-muted-foreground')}>
                    {m.label}
                  </p>
                  <p className="text-2xs text-muted-foreground mt-0.5 leading-snug">{m.hint}</p>
                </div>
              </button>
            );
          })}
        </div>

        {/* Quick-start suggestions */}
        <div className="flex flex-wrap gap-2 justify-center">
          {SUGGESTIONS.map(s => (
            <button
              key={s}
              onClick={() => { setInput(s); inputRef.current?.focus(); }}
              className="rounded-full border border-border bg-muted/20 px-3 py-1 text-xs text-muted-foreground hover:border-primary/40 hover:text-foreground hover:bg-primary/5 transition-colors"
            >
              {s}
            </button>
          ))}
        </div>
      </div>

      {/* Bottom: chat input — anchored, always visible */}
      <div className="w-full max-w-2xl space-y-2 shrink-0">
        <div className={cn(
          'flex items-center gap-3 rounded-xl border bg-muted/20 px-4 py-3 transition-all',
          'focus-within:border-primary/50 focus-within:bg-background focus-within:shadow-md focus-within:shadow-primary/5'
        )}>
          <input
            ref={inputRef}
            value={input}
            onChange={e => setInput(e.target.value)}
            onKeyDown={e => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); handleSubmit(); } }}
            placeholder={selectedMode.placeholder}
            disabled={creating}
            className="flex-1 bg-transparent text-sm outline-none placeholder:text-muted-foreground/40 disabled:opacity-50"
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
        <p className="text-center text-2xs text-muted-foreground/40">
          Press Enter to start — Prime will ask follow-up questions in the chat panel
        </p>
      </div>

    </div>
  );
}
