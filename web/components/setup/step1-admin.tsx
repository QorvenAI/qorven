'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { CheckCircle2, Eye, EyeOff, ShieldCheck } from 'lucide-react';
import { SectionTitle, LabeledInput } from './setup-atoms';

export function Step1Admin(p: {
  displayName: string; setDisplayName: (v: string) => void;
  email: string; setEmail: (v: string) => void;
  username: string; setUsername: (v: string) => void;
  password: string; setPassword: (v: string) => void;
  showPw: boolean; setShowPw: (v: boolean) => void;
  busy: boolean; adminCreated: boolean; submit: () => void; skippable: boolean;
}) {
  if (p.adminCreated) {
    return (
      <div className="space-y-3">
        <SectionTitle icon={ShieldCheck} title="Admin account" subtitle="Already configured." />
        <div className="rounded-lg border border-emerald-500/30 bg-emerald-500/10 px-3 py-3 text-sm text-emerald-400 flex items-center gap-2">
          <CheckCircle2 className="h-4 w-4" /> Admin is ready. Continue to the next step.
        </div>
      </div>
    );
  }
  return (
    <div className="space-y-4">
      <SectionTitle icon={ShieldCheck} title="Create admin account"
        subtitle="Set up the login you'll use to sign in to Qorven." />
      <LabeledInput label="Full name" value={p.displayName} onChange={p.setDisplayName} autoFocus placeholder="Your name" />
      <LabeledInput label="Email" value={p.email} onChange={p.setEmail} type="email" placeholder="you@example.com" />
      <LabeledInput label="Login username" value={p.username} onChange={p.setUsername} placeholder="admin" />
      <div>
        <label className="block text-sm font-medium text-muted-foreground mb-1.5">
          Password <span className="text-muted-foreground/60">(min 8 chars)</span>
        </label>
        <div className="relative">
          <input
            type={p.showPw ? 'text' : 'password'}
            value={p.password}
            onChange={e => p.setPassword(e.target.value)}
            className="qr-input pr-9"
          />
          <button type="button" onClick={() => p.setShowPw(!p.showPw)}
            className="absolute right-2 top-1/2 -translate-y-1/2 p-1 text-muted-foreground hover:text-foreground">
            {p.showPw ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
          </button>
        </div>
      </div>
    </div>
  );
}
