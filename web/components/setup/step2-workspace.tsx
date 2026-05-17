'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { Sparkles } from 'lucide-react';
import { cn } from '@/lib/utils';
import { AVATAR_GRADIENTS } from './setup-config';
import { SectionTitle, LabeledInput } from './setup-atoms';

const STYLE_OPTIONS = [
  { value: 'professional', label: 'Professional', desc: 'Formal, precise, structured replies' },
  { value: 'casual',       label: 'Casual',       desc: 'Warm, conversational, friendly' },
  { value: 'technical',    label: 'Technical',    desc: 'Direct, code-first, minimal prose' },
];

const LANGUAGE_OPTIONS = [
  { value: 'en', label: 'English' },
  { value: 'es', label: 'Spanish' },
  { value: 'fr', label: 'French' },
  { value: 'de', label: 'German' },
  { value: 'pt', label: 'Portuguese' },
  { value: 'ar', label: 'Arabic' },
  { value: 'hi', label: 'Hindi' },
  { value: 'zh', label: 'Chinese' },
  { value: 'ja', label: 'Japanese' },
];

export function Step2Workspace(p: {
  displayName: string;
  primeName: string; setPrimeName: (v: string) => void;
  gradient: number; setGradient: (v: number) => void;
  callMe: string; setCallMe: (v: string) => void;
  style: string; setStyle: (v: string) => void;
  language: string; setLanguage: (v: string) => void;
}) {
  const greeting = p.displayName ? `Hi ${p.displayName}! ` : '';

  return (
    <div className="space-y-5">
      <SectionTitle icon={Sparkles} title="Meet your assistant"
        subtitle={`${greeting}Set up your AI assistant's name and personality.`} />

      {/* Avatar preview + gradient picker */}
      <div className="flex items-center gap-4">
        <div className={cn(
          'flex h-16 w-16 shrink-0 items-center justify-center rounded-2xl bg-gradient-to-br text-xl font-semibold text-white',
          AVATAR_GRADIENTS[p.gradient]
        )}>
          {p.primeName.charAt(0).toUpperCase() || 'P'}
        </div>
        <div className="flex flex-wrap gap-2">
          {AVATAR_GRADIENTS.map((g, i) => (
            <button key={i} onClick={() => p.setGradient(i)}
              className={cn(
                'h-6 w-6 rounded-lg bg-gradient-to-br transition-transform',
                g,
                i === p.gradient
                  ? 'ring-2 ring-primary ring-offset-2 ring-offset-background scale-110'
                  : 'hover:scale-110'
              )}
            />
          ))}
        </div>
      </div>

      <LabeledInput
        label="Assistant name"
        value={p.primeName}
        onChange={p.setPrimeName}
        autoFocus
        placeholder="Prime"
      />

      <LabeledInput
        label="What should your assistant call you?"
        value={p.callMe}
        onChange={p.setCallMe}
        placeholder={p.displayName || 'Your name or nickname'}
      />

      <div className="space-y-1.5">
        <label className="text-xs font-medium text-foreground">Communication style</label>
        <div className="grid grid-cols-3 gap-2">
          {STYLE_OPTIONS.map(opt => (
            <button
              key={opt.value}
              onClick={() => p.setStyle(opt.value)}
              className={cn(
                'flex flex-col items-start rounded-lg border px-3 py-2.5 text-left transition-colors',
                p.style === opt.value
                  ? 'border-primary bg-primary/8 text-foreground'
                  : 'border-border bg-card/50 text-muted-foreground hover:bg-accent',
              )}
            >
              <span className="text-xs font-medium text-foreground">{opt.label}</span>
              <span className="mt-0.5 text-[11px] leading-tight">{opt.desc}</span>
            </button>
          ))}
        </div>
      </div>

      <div className="space-y-1.5">
        <label className="text-xs font-medium text-foreground">Language</label>
        <select
          value={p.language}
          onChange={e => p.setLanguage(e.target.value)}
          className="w-full rounded-lg border border-border bg-card px-3 py-2 text-xs text-foreground focus:outline-none focus:ring-2 focus:ring-primary/40"
        >
          {LANGUAGE_OPTIONS.map(o => (
            <option key={o.value} value={o.value}>{o.label}</option>
          ))}
        </select>
      </div>
    </div>
  );
}
