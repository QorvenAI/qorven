'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { CheckCircle2, MessageCircle, Send, Smartphone, Bell, Lock } from 'lucide-react';
import { QorvenSpinner, SectionTitle } from './setup-atoms';

const TELEGRAM_BENEFITS = [
  { icon: Smartphone, text: 'Chat with your AI from any device, anywhere' },
  { icon: Bell,       text: 'Get proactive updates and briefings pushed to you' },
  { icon: Lock,       text: 'End-to-end encrypted — more private than email' },
];

export function Step7Channels(p: {
  primeName: string;
  telegram: string; setTelegram: (v: string) => void;
  connected: string[]; busy: string | null; error: string | null;
  connect: (t: 'telegram') => void;
  onSkip: () => void;
}) {
  const connected = p.connected.includes('telegram');

  return (
    <div className="space-y-5">
      <SectionTitle icon={Send} title="Connect Telegram (optional)"
        subtitle={`Let ${p.primeName} reach you on Telegram. You can also skip and set this up later.`} />

      {/* Telegram card */}
      <div className="rounded-xl border border-border bg-card/80 overflow-hidden">
        {/* Header */}
        <div className="flex items-center gap-3 px-4 py-3.5 border-b border-border">
          <img src="/icons/providers/telegram.svg" alt="Telegram"
            className="h-8 w-8 rounded-lg object-contain shrink-0"
            onError={e => { (e.target as HTMLImageElement).style.display = 'none'; }} />
          <div>
            <div className="text-sm font-semibold text-foreground">Telegram</div>
            <div className="text-xs text-muted-foreground">Connect via a Telegram bot</div>
          </div>
          {connected && (
            <span className="ml-auto flex items-center gap-1 text-xs text-emerald-400">
              <CheckCircle2 className="h-3.5 w-3.5" /> Connected
            </span>
          )}
        </div>

        {/* Benefits */}
        <div className="px-4 py-3.5 space-y-2.5">
          {TELEGRAM_BENEFITS.map(({ icon: Icon, text }) => (
            <div key={text} className="flex items-start gap-2.5">
              <div className="flex h-5 w-5 shrink-0 items-center justify-center rounded-md bg-primary/10 text-primary mt-0.5">
                <Icon className="h-3 w-3" />
              </div>
              <span className="text-xs text-muted-foreground leading-relaxed">{text}</span>
            </div>
          ))}
        </div>

        {/* Token input */}
        {!connected && (
          <div className="px-4 pb-4 space-y-2">
            <label className="block text-xs font-medium text-muted-foreground">
              Bot token from <span className="text-foreground">@BotFather</span>
            </label>
            <div className="flex gap-2">
              <input
                type="password"
                value={p.telegram}
                onChange={e => p.setTelegram(e.target.value)}
                placeholder="123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11"
                className="qr-textarea flex-1 resize-none text-xs font-mono"
              />
              <button
                onClick={() => p.connect('telegram')}
                disabled={p.busy === 'telegram' || !p.telegram}
                className="inline-flex items-center gap-1.5 rounded-lg bg-primary px-3 py-2 text-xs font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-40 cursor-pointer shrink-0"
              >
                {p.busy === 'telegram' ? <><QorvenSpinner className="h-3 w-3" /> Connecting…</> : 'Connect'}
              </button>
            </div>
          </div>
        )}

        {connected && (
          <div className="px-4 pb-4">
            <div className="rounded-lg bg-emerald-500/10 border border-emerald-500/30 px-3 py-2 text-xs text-emerald-400 flex items-center gap-1.5">
              <CheckCircle2 className="h-3.5 w-3.5 shrink-0" />
              Telegram bot connected — {p.primeName} will respond to your messages
            </div>
          </div>
        )}
      </div>

      {/* Error */}
      {p.error && (
        <div className="rounded-lg border border-destructive/30 bg-destructive/10 px-3 py-2 text-xs text-destructive">
          {p.error}
        </div>
      )}

      {/* How to get token hint */}
      {!connected && (
        <p className="text-xs text-muted-foreground">
          Don't have a bot yet?{' '}
          <span className="text-foreground">Open Telegram → search @BotFather → /newbot</span>
          {' '}and paste the token above.
        </p>
      )}

      {/* Skip */}
      {!connected && (
        <button
          onClick={p.onSkip}
          className="w-full text-xs text-muted-foreground hover:text-foreground transition-colors py-1"
        >
          Skip for now — I'll set this up in Settings
        </button>
      )}
    </div>
  );
}
