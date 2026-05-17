'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useState } from 'react';
import { Wand2 } from 'lucide-react';
import { cn } from '@/lib/utils';
import { PRIME_ROLE_PRESETS, AVATAR_GRADIENTS, LANGUAGES } from './setup-config';
import { SectionTitle, LabeledInput } from './setup-atoms';

export function Step3Prime(p: {
  primeName: string; setPrimeName: (v: string) => void;
  gradient: number; setGradient: (v: number) => void;
  roleDesc: string; setRoleDesc: (v: string) => void;
  style: 'professional'|'casual'|'technical'|'creative';
  setStyle: (v: 'professional'|'casual'|'technical'|'creative') => void;
  language: string; setLanguage: (v: string) => void;
}) {
  const [selectedPreset, setSelectedPreset] = useState<string>(() => {
    const match = PRIME_ROLE_PRESETS.find(pr => pr.value === p.roleDesc && pr.label !== 'Custom');
    return match ? match.label : 'Custom';
  });

  function pickPreset(label: string, value: string) {
    setSelectedPreset(label);
    if (value !== '') p.setRoleDesc(value);
  }

  return (
    <div className="space-y-4">
      <SectionTitle icon={Wand2} title="Configure your assistant"
        subtitle="These choices become Prime's name and system prompt. You can revise anytime." />
      <div className="flex items-center gap-4">
        <div className={cn('flex h-16 w-16 items-center justify-center rounded-2xl bg-gradient-to-br text-xl font-semibold text-white', AVATAR_GRADIENTS[p.gradient])}>
          {p.primeName.charAt(0).toUpperCase() || 'P'}
        </div>
        <div className="flex flex-wrap gap-2">
          {AVATAR_GRADIENTS.map((g, i) => (
            <button key={i} onClick={() => p.setGradient(i)}
              className={cn('h-6 w-6 rounded-lg bg-gradient-to-br transition-transform', g,
                i === p.gradient ? 'ring-2 ring-primary ring-offset-2 ring-offset-background scale-110' : 'hover:scale-110')}
            />
          ))}
        </div>
      </div>
      <LabeledInput label="Display name" value={p.primeName} onChange={p.setPrimeName} />

      <div>
        <label className="block text-sm font-medium text-muted-foreground mb-2">What should Prime be good at?</label>
        <div className="grid grid-cols-2 gap-1.5">
          {PRIME_ROLE_PRESETS.map(preset => (
            <button key={preset.label}
              onClick={() => pickPreset(preset.label, preset.value)}
              className={cn('rounded-lg border px-3 py-2 text-left text-sm font-medium transition-colors',
                selectedPreset === preset.label
                  ? 'border-primary bg-primary/10 text-primary'
                  : 'border-border text-foreground hover:border-primary/50')}>
              {preset.label}
            </button>
          ))}
        </div>
        {selectedPreset === 'Custom' && (
          <textarea value={p.roleDesc} onChange={e => p.setRoleDesc(e.target.value)}
            rows={2} placeholder="Describe what Prime should be good at…"
            className="qr-input mt-1" />
        )}
      </div>

      <div>
        <label className="block text-sm font-medium text-muted-foreground mb-1.5">Communication style</label>
        <div className="grid grid-cols-4 gap-2">
          {(['professional','casual','technical','creative'] as const).map(s => (
            <button key={s} onClick={() => p.setStyle(s)}
              className={cn('rounded-lg border px-2 py-2 text-sm font-medium capitalize transition-colors',
                p.style === s ? 'border-primary bg-primary/10 text-primary' : 'border-border text-foreground hover:border-primary/50')}>
              {s}
            </button>
          ))}
        </div>
      </div>
      <div>
        <label className="block text-sm font-medium text-muted-foreground mb-1.5">Language</label>
        <select value={p.language} onChange={e => p.setLanguage(e.target.value)}
          className="qr-select">
          {LANGUAGES.map(l => <option key={l} value={l}>{l}</option>)}
        </select>
      </div>
    </div>
  );
}
