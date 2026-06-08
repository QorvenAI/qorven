'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useCallback, useEffect, useRef, useState } from 'react';
import { ArrowRight, Check, Eye, EyeOff } from 'lucide-react';
import { cn } from '@/lib/utils';
import { api, listAgents } from '@/components/setup/setup-api';
import { PROVIDER_OPTIONS_FALLBACK, RECOMMENDED_PRIMARY } from '@/components/setup/setup-config';
import { setToken } from '@/lib/api-core';

// ── Types ─────────────────────────────────────────────────────────────────────

export interface ChatWizardProps {
  appVersion: string;
  onComplete: () => void;
  onPhaseChange?: (phase: number) => void;
}

type InputType = 'confirm' | 'text' | 'password' | 'email' | 'pill' | 'card_pick' | 'info' | 'launch';

interface PillOption { value: string; label: string; desc: string; }

interface ScriptItem {
  key: string;
  text: string | ((answers: Record<string, string>) => string);
  inputType: InputType;
  placeholder?: string;
  required?: boolean;
  minLength?: number;
  skippable?: boolean;
  defaultValue?: string;
  derivedFrom?: string;
  afterAnswer?: 'create_account' | 'test_provider' | 'connect_telegram' | 'finalise';
  options?: PillOption[];
  confirmItems?: string[];
  confirmLabel?: string;
}

interface Message {
  id: string;
  role: 'prime' | 'user' | 'error';
  content: string;
  animate: boolean;
}

// ── Script ────────────────────────────────────────────────────────────────────

const PHASE_BOUNDARIES: Record<number, number> = {
  0: 0,
  1: 1,
  5: 2,
  7: 3,
  10: 4,
};

const SCRIPT: ScriptItem[] = [
  {
    key: 'disclaimer',
    text: "Hi — I'm Prime, your AI chief of staff. Before we begin, please confirm what agents on this system will be able to do.",
    inputType: 'confirm',
    confirmItems: [
      'Execute code and shell commands on this server',
      'Browse the web and call external APIs',
      'Send messages via connected channels (Telegram, Slack, WhatsApp, email)',
      'Read and write files, databases, and connected storage',
      'Run tasks autonomously on a schedule',
    ],
    confirmLabel: "I understand — let's set up Qorven",
  },
  {
    key: 'display_name',
    text: 'What should I call you?',
    inputType: 'text',
    placeholder: 'Your name',
    required: true,
  },
  {
    key: 'username',
    text: 'Pick a username for signing in:',
    inputType: 'text',
    placeholder: 'admin',
    required: true,
    derivedFrom: 'display_name',
  },
  {
    key: 'password',
    text: 'Set a password (minimum 8 characters):',
    inputType: 'password',
    placeholder: '••••••••',
    required: true,
    minLength: 8,
  },
  {
    key: 'email',
    text: "Email address? I'll use it for notifications.",
    inputType: 'email',
    placeholder: 'you@example.com',
    skippable: true,
    afterAnswer: 'create_account',
  },
  {
    key: 'prime_name',
    text: (a) => `Nice to meet you, ${a.display_name}. What should I call myself?`,
    inputType: 'text',
    placeholder: 'Prime',
    defaultValue: 'Prime',
  },
  {
    key: 'work_style',
    text: 'How would you like me to communicate?',
    inputType: 'pill',
    options: [
      { value: 'casual', label: 'Casual', desc: 'Friendly and relaxed' },
      { value: 'formal', label: 'Formal', desc: 'Professional and concise' },
      { value: 'direct', label: 'Direct', desc: 'No fluff, just results' },
    ],
  },
  {
    key: 'provider',
    text: "Now let's connect an AI provider. Which one are you using?",
    inputType: 'card_pick',
  },
  {
    key: 'api_key',
    text: (a) => `Paste your ${PROVIDER_OPTIONS_FALLBACK.find(p => p.id === a.provider)?.label ?? a.provider} API key:`,
    inputType: 'password',
    placeholder: 'sk-…',
    afterAnswer: 'test_provider',
  },
  {
    key: '_provider_status',
    text: (a) => a._provider_ok === 'true'
      ? '✓ Connected. Your agents can think.'
      : "✗ That key didn't work. Paste it again and I'll retry.",
    inputType: 'info',
  },
  {
    key: 'telegram',
    text: 'Want to connect Telegram so I can message you? Paste your bot token or skip.',
    inputType: 'text',
    placeholder: 'Bot token…',
    skippable: true,
    afterAnswer: 'connect_telegram',
  },
  {
    key: '_done',
    text: (a) => `You're all set, ${a.display_name}. Your workspace is ready.`,
    inputType: 'launch',
    afterAnswer: 'finalise',
  },
];

// ── Component ─────────────────────────────────────────────────────────────────

export function ChatWizard({ appVersion: _appVersion, onComplete, onPhaseChange }: ChatWizardProps) {
  const [thread,   setThread]   = useState<Message[]>([]);
  const [qIndex,   setQIndex]   = useState(0);
  const [answers,  setAnswers]  = useState<Record<string, string>>({});
  const [inputVal, setInputVal] = useState('');
  const [loading,  setLoading]  = useState(false);
  const [error,    setError]    = useState('');
  const [showPw,   setShowPw]   = useState(false);
  const bottomRef = useRef<HTMLDivElement>(null);
  const submittingRef = useRef(false);

  const currentQ = SCRIPT[qIndex];

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [thread, loading]);

  useEffect(() => {
    if (thread.length === 0 && SCRIPT[0]) {
      const item = SCRIPT[0]!;
      const text = typeof item.text === 'function' ? item.text({}) : item.text;
      setThread([{ id: 'p-0', role: 'prime', content: text, animate: false }]);
      onPhaseChange?.(0);
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    if (!currentQ) return;
    if (currentQ.derivedFrom && answers[currentQ.derivedFrom]) {
      const derived = answers[currentQ.derivedFrom]!
        .toLowerCase().replace(/[^a-z0-9]/g, '').slice(0, 20);
      setInputVal(derived);
    } else if (currentQ.defaultValue) {
      setInputVal(currentQ.defaultValue);
    } else {
      setInputVal('');
    }
    setError('');
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [qIndex]);

  function advance(newAnswers: Record<string, string>, nextIdx: number) {
    setAnswers(newAnswers);
    setQIndex(nextIdx);
    if (PHASE_BOUNDARIES[nextIdx] !== undefined) {
      onPhaseChange?.(PHASE_BOUNDARIES[nextIdx]);
    }
    if (nextIdx < SCRIPT.length) {
      setTimeout(() => {
        const item = SCRIPT[nextIdx]!;
        const text = typeof item.text === 'function' ? item.text(newAnswers) : item.text;
        setThread(prev => [...prev, { id: `p-${nextIdx}`, role: 'prime', content: text, animate: true }]);
      }, 150);
    }
  }

  async function doCreateAccount(a: Record<string, string>) {
    await api('/auth/setup', {
      method: 'POST',
      body: JSON.stringify({
        username: a.username,
        password: a.password,
        display_name: a.display_name || undefined,
        email: a.email || undefined,
      }),
    });
    const login = await api<{ token: string }>('/auth/login', {
      method: 'POST',
      body: JSON.stringify({ username: a.username, password: a.password }),
    });
    setToken(login.token);
  }

  async function doTestProvider(a: Record<string, string>): Promise<void> {
    const opt = PROVIDER_OPTIONS_FALLBACK.find(p => p.id === a.provider);
    if (!opt) throw new Error('Unknown provider');
    const body: Record<string, unknown> = {
      name: opt.id,
      provider_type: opt.providerType,
      api_key: a.api_key ?? '',
      api_base: opt.defaultApiBase ?? '',
    };
    const res = await api<{ success: boolean; error?: string }>('/v1/providers/test', {
      method: 'POST',
      body: JSON.stringify(body),
    });
    if (!res.success) throw new Error(res.error ?? 'provider test failed');

    const { providers: list } = await api<{ providers: Array<{ id: string; name: string }> }>('/v1/providers');
    const existing = Array.isArray(list) ? list.find(p => p.name === opt.id) : null;
    let providerDbId: string;
    if (existing) {
      providerDbId = existing.id;
    } else {
      const created = await api<{ id: string }>('/v1/providers', {
        method: 'POST',
        body: JSON.stringify({
          name: opt.id, display_name: opt.label,
          provider_type: opt.providerType,
          api_base: opt.defaultApiBase ?? '',
          api_key: a.api_key ?? '',
          enabled: true, settings: {},
        }),
      });
      providerDbId = created.id;
    }

    const agents = await listAgents();
    const prime = agents.find(ag => ag.agent_key === 'chief') ?? agents.find(ag => ag.agent_key === 'prime');
    if (prime) {
      const primeName = a.prime_name || 'Prime';
      await api(`/v1/agents/${prime.id}`, {
        method: 'PUT',
        body: JSON.stringify({
          display_name: primeName,
          provider_id: providerDbId,
          model: RECOMMENDED_PRIMARY,
          system_prompt: `You are ${primeName}, a personal AI assistant. Be helpful, clear, and direct.`,
        }),
      });
    } else {
      console.warn('setup: prime agent not found, skipping model assignment');
    }
  }

  async function doConnectTelegram(a: Record<string, string>) {
    const token = a.telegram;
    if (!token) return;
    const agents = await listAgents();
    const prime = agents.find(ag => ag.agent_key === 'chief') ?? agents.find(ag => ag.agent_key === 'prime');
    if (!prime) throw new Error('Prime agent not found');
    const ch = await api<{ id: string }>('/v1/channels', {
      method: 'POST',
      body: JSON.stringify({
        agent_id: prime.id,
        channel_type: 'telegram',
        name: 'telegram-main',
        config: { bot_token: token },
      }),
    });
    await api(`/v1/channels/${ch.id}/start`, { method: 'POST' }).catch(() => {});
  }

  async function doFinalise(a: Record<string, string>) {
    await api('/v1/setup/finalize', {
      method: 'POST',
      body: JSON.stringify({
        instance_name: 'My Workspace',
        prime_name: a.prime_name || 'Prime',
        call_me: a.display_name || '',
        style: a.work_style || 'casual',
        language: 'en',
      }),
    }).catch(() => {});
  }

  const submit = useCallback(async (rawValue: string, skip = false) => {
    if (submittingRef.current) return;
    submittingRef.current = true;
    try {
      if (!currentQ) return;
      if (loading) return;

      const value = skip ? '' : rawValue.trim();

      if (!skip && currentQ.required && !value) {
        setError('This field is required.'); return;
      }
      if (!skip && currentQ.minLength && value.length < currentQ.minLength) {
        setError(`Minimum ${currentQ.minLength} characters.`); return;
      }

      setError('');

      // Push user bubble
      if (currentQ.inputType === 'confirm') {
        setThread(prev => [...prev, { id: `u-${Date.now()}`, role: 'user', content: currentQ.confirmLabel ?? 'Confirmed', animate: false }]);
      } else if (currentQ.inputType !== 'info' && currentQ.inputType !== 'launch') {
        const display = skip ? 'Skipped' : currentQ.inputType === 'password' ? '•'.repeat(Math.min(value.length, 12)) : value;
        setThread(prev => [...prev, { id: `u-${Date.now()}`, role: 'user', content: display || 'Skipped', animate: false }]);
      }

      const newAnswers = { ...answers, [currentQ.key]: value };
      setLoading(true);

      try {
        if (currentQ.afterAnswer === 'create_account') {
          await doCreateAccount(newAnswers);
        }
        if (currentQ.afterAnswer === 'test_provider') {
          try {
            await doTestProvider(newAnswers);
            newAnswers._provider_ok = 'true';
          } catch (e) {
            newAnswers._provider_ok = 'false';
            newAnswers._provider_err = e instanceof Error ? e.message : 'failed';
          }
        }
        if (currentQ.afterAnswer === 'connect_telegram' && value) {
          await doConnectTelegram(newAnswers).catch(() => {});
        }
        if (currentQ.afterAnswer === 'finalise') {
          await doFinalise(newAnswers);
          setLoading(false);
          onComplete();
          return;
        }
      } catch (e) {
        setLoading(false);
        setError(e instanceof Error ? e.message : 'Something went wrong. Please try again.');
        return;
      }

      setLoading(false);
      advance(newAnswers, qIndex + 1);
    } finally {
      submittingRef.current = false;
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [currentQ, answers, qIndex, loading, onComplete]);

  const handleRetry = useCallback(() => {
    const prevIdx = qIndex - 1;
    setQIndex(prevIdx);
    setError('');
    const item = SCRIPT[prevIdx]!;
    const text = typeof item.text === 'function' ? item.text(answers) : item.text;
    setThread(prev => [...prev, { id: `p-retry-${Date.now()}`, role: 'prime', content: text, animate: true }]);
  }, [qIndex, answers]);

  const visibleThread = thread.slice(-6);

  return (
    <div className="flex flex-col h-full">
      <div className="flex-1 min-h-0 overflow-y-auto px-8 py-6 space-y-4">
        {visibleThread.map((msg) => (
          <div
            key={msg.id}
            className={cn(
              'flex gap-3',
              msg.role === 'user' ? 'justify-end' : 'justify-start',
              msg.animate ? 'animate-in slide-in-from-left-4 fade-in duration-200' : '',
            )}
          >
            {msg.role === 'prime' && (
              <div className="h-7 w-7 shrink-0 rounded-full bg-gradient-to-br from-violet-500 to-fuchsia-600 flex items-center justify-center mt-0.5">
                <span className="text-[10px] font-bold text-white">P</span>
              </div>
            )}
            <div className={cn(
              'rounded-2xl px-4 py-2.5 text-sm leading-relaxed max-w-[580px]',
              msg.role === 'prime' ? 'bg-card border border-border text-foreground rounded-tl-sm' : '',
              msg.role === 'user' ? 'bg-primary text-primary-foreground rounded-tr-sm' : '',
              msg.role === 'error' ? 'bg-destructive/10 border border-destructive/30 text-destructive' : '',
            )}>
              {msg.content}
            </div>
          </div>
        ))}

        {loading && (
          <div className="flex gap-3 justify-start">
            <div className="h-7 w-7 shrink-0 rounded-full bg-gradient-to-br from-violet-500 to-fuchsia-600 flex items-center justify-center">
              <span className="text-[10px] font-bold text-white">P</span>
            </div>
            <div className="rounded-2xl rounded-tl-sm bg-card border border-border px-4 py-3 flex items-center gap-1">
              {[0, 1, 2].map(i => (
                <span key={i} className="h-1.5 w-1.5 rounded-full bg-muted-foreground/60 animate-bounce"
                  style={{ animationDelay: `${i * 0.15}s` }} />
              ))}
            </div>
          </div>
        )}

        <div ref={bottomRef} />
      </div>

      {error && (
        <div className="shrink-0 px-8 pb-2">
          <p className="text-xs text-destructive">{error}</p>
        </div>
      )}

      {currentQ && !loading && (
        <InputArea
          item={currentQ}
          value={inputVal}
          onChange={setInputVal}
          showPw={showPw}
          onTogglePw={() => setShowPw(v => !v)}
          onSubmit={submit}
          onRetry={handleRetry}
          answers={answers}
        />
      )}
    </div>
  );
}

// ── InputArea ─────────────────────────────────────────────────────────────────

interface InputAreaProps {
  item: ScriptItem;
  value: string;
  onChange: (v: string) => void;
  showPw: boolean;
  onTogglePw: () => void;
  onSubmit: (value: string, skip?: boolean) => void;
  onRetry?: () => void;
  answers: Record<string, string>;
}

function InputArea({ item, value, onChange, showPw, onTogglePw, onSubmit, onRetry, answers }: InputAreaProps) {
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    inputRef.current?.focus();
  }, [item.key]);

  const handleKey = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); onSubmit(value); }
  };

  if (item.inputType === 'confirm') {
    return (
      <div className="shrink-0 border-t border-border px-8 py-5 space-y-4">
        <ul className="space-y-2">
          {(item.confirmItems ?? []).map(c => (
            <li key={c} className="flex items-start gap-2.5 text-sm text-foreground">
              <Check className="h-3.5 w-3.5 shrink-0 text-primary mt-0.5" />
              {c}
            </li>
          ))}
        </ul>
        <button
          onClick={() => onSubmit('confirmed')}
          className="inline-flex items-center gap-2 rounded-xl bg-primary px-6 py-2.5 text-sm font-semibold text-primary-foreground hover:bg-primary/90 transition-opacity cursor-pointer">
          {item.confirmLabel ?? 'Continue'} <ArrowRight className="h-4 w-4" />
        </button>
      </div>
    );
  }

  if (item.inputType === 'pill') {
    return (
      <div className="shrink-0 border-t border-border px-8 py-5 flex gap-3 flex-wrap">
        {(item.options ?? []).map(opt => (
          <button
            key={opt.value}
            onClick={() => onSubmit(opt.value)}
            className="flex flex-col items-start rounded-xl border border-border bg-card px-4 py-3 hover:border-primary/50 hover:bg-accent transition-colors cursor-pointer text-left min-w-[140px]">
            <span className="text-sm font-semibold text-foreground">{opt.label}</span>
            <span className="text-xs text-muted-foreground">{opt.desc}</span>
          </button>
        ))}
      </div>
    );
  }

  if (item.inputType === 'card_pick') {
    const clouds = PROVIDER_OPTIONS_FALLBACK.filter(p => p.category === 'cloud');
    const others = PROVIDER_OPTIONS_FALLBACK.filter(p => p.category !== 'cloud');
    return (
      <div className="shrink-0 border-t border-border px-8 py-5">
        <div className="grid grid-cols-3 gap-2 max-w-[680px]">
          {[...clouds, ...others].map(opt => (
            <button
              key={opt.id}
              onClick={() => onSubmit(opt.id)}
              className="flex flex-col items-start rounded-xl border border-border bg-card px-3 py-2.5 hover:border-primary/50 hover:bg-accent transition-colors cursor-pointer text-left">
              <span className="text-sm font-semibold text-foreground">{opt.label}</span>
              <span className="text-xs text-muted-foreground leading-snug">{opt.hint}</span>
            </button>
          ))}
        </div>
      </div>
    );
  }

  if (item.inputType === 'info') {
    const ok = answers._provider_ok === 'true';
    return (
      <div className="shrink-0 border-t border-border px-8 py-5">
        <button
          onClick={() => ok ? onSubmit('ack') : onRetry?.()}
          className={cn(
            'inline-flex items-center gap-2 rounded-xl px-6 py-2.5 text-sm font-semibold transition-opacity cursor-pointer',
            ok
              ? 'bg-primary text-primary-foreground hover:bg-primary/90'
              : 'bg-destructive/10 border border-destructive/30 text-destructive hover:bg-destructive/20',
          )}>
          {ok ? <><span>Continue</span> <ArrowRight className="h-4 w-4" /></> : <span>Try again</span>}
        </button>
      </div>
    );
  }

  if (item.inputType === 'launch') {
    return (
      <div className="shrink-0 border-t border-border px-8 py-5">
        <button
          onClick={() => onSubmit('launch')}
          className="inline-flex items-center gap-2 rounded-xl bg-primary px-8 py-3 text-base font-semibold text-primary-foreground hover:bg-primary/90 transition-opacity cursor-pointer">
          Launch Qorven <ArrowRight className="h-5 w-5" />
        </button>
      </div>
    );
  }

  // text / email / password
  return (
    <div className="shrink-0 border-t border-border px-8 py-5">
      <div className="flex gap-3 max-w-[680px]">
        <div className="relative flex-1">
          <input
            ref={inputRef}
            type={item.inputType === 'password' && !showPw ? 'password' : item.inputType === 'email' ? 'email' : 'text'}
            value={value}
            onChange={e => onChange(e.target.value)}
            onKeyDown={handleKey}
            placeholder={item.placeholder}
            autoComplete={item.inputType === 'password' ? 'new-password' : 'off'}
            className="h-11 w-full rounded-xl border border-border bg-card px-4 pr-10 text-sm outline-none transition-colors placeholder:text-muted-foreground/40 focus:border-primary"
          />
          {item.inputType === 'password' && (
            <button
              type="button"
              onClick={onTogglePw}
              className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground transition-colors">
              {showPw ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
            </button>
          )}
        </div>
        <button
          onClick={() => onSubmit(value)}
          disabled={!value.trim()}
          className="h-11 rounded-xl bg-primary px-4 text-primary-foreground hover:bg-primary/90 disabled:opacity-40 disabled:cursor-not-allowed transition-opacity cursor-pointer">
          <ArrowRight className="h-4 w-4" />
        </button>
        {item.skippable && (
          <button
            onClick={() => onSubmit('', true)}
            className="h-11 rounded-xl border border-border px-4 text-sm text-muted-foreground hover:bg-accent transition-colors cursor-pointer">
            Skip
          </button>
        )}
      </div>
    </div>
  );
}
