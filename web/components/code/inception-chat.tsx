'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useState, useRef, useEffect } from 'react';
import { Send, Loader2, Sparkles } from 'lucide-react';
import { projectBriefs as api } from '@/lib/api';
import type { ProjectBrief, ProjectQuality } from '@/types';
import { cn } from '@/lib/utils';

interface Props {
  brief: ProjectBrief;
  onBriefUpdate: (b: ProjectBrief) => void;
}

interface ChatMessage {
  role: 'assistant' | 'user';
  content: string;
}

type Field = 'title' | 'idea' | 'stack' | 'budget' | 'timeline' | 'quality';

const QUESTIONS: Record<Field, string> = {
  title:    "Let's start — what's a short name for this project?",
  idea:     "Describe the idea. What problem does it solve and who is it for?",
  stack:    "What's the tech stack? (e.g. \"Go + Postgres + Next.js\") — or type 'skip' to let Prime decide.",
  budget:   "What's the max budget in USD? (e.g. 50 for $50) — or type 'skip' for no limit.",
  timeline: "How urgent is this? Type: today, this_week, this_month, or no_rush.",
  quality:  "What quality tier? Type: mvp, production, or enterprise.",
};

const FIELD_ORDER: Field[] = ['title', 'idea', 'stack', 'budget', 'timeline', 'quality'];

const QUALITY_OPTIONS: { value: ProjectQuality; label: string; hint: string }[] = [
  { value: 'mvp',        label: 'MVP',        hint: 'Fastest, cheapest — get it working' },
  { value: 'production', label: 'Production', hint: 'Code review + integration tests' },
  { value: 'enterprise', label: 'Enterprise', hint: 'Full docs + security audit' },
];

const TIMELINE_OPTIONS = [
  { value: 'today',      label: 'Today' },
  { value: 'this_week',  label: 'This week' },
  { value: 'this_month', label: 'This month' },
  { value: 'no_rush',    label: 'No rush' },
];

// Safe bold renderer — splits on **...** and returns React nodes. No HTML injection.
function renderMessage(text: string): React.ReactNode {
  const parts = text.split(/\*\*(.*?)\*\*/g);
  return parts.map((part, i) =>
    i % 2 === 1 ? <strong key={i}>{part}</strong> : part
  );
}

function getBriefField(brief: ProjectBrief, field: Field): string {
  switch (field) {
    case 'title':    return brief.title && brief.title !== 'New Project' ? brief.title : '';
    case 'idea':     return brief.idea;
    case 'stack':    return brief.stack;
    case 'budget':   return brief.budget_cents > 0 ? String(brief.budget_cents / 100) : '';
    case 'timeline': return brief.timeline ?? '';
    case 'quality':  return brief.quality ?? '';
  }
}

function formatFieldDisplay(field: Field, value: string): string {
  if (field === 'budget')   return `$${value}`;
  if (field === 'timeline') return value.replace('_', ' ');
  if (field === 'quality')  return value.charAt(0).toUpperCase() + value.slice(1);
  return value;
}

function briefToMessages(brief: ProjectBrief): ChatMessage[] {
  const msgs: ChatMessage[] = [];
  for (const field of FIELD_ORDER) {
    const value = getBriefField(brief, field);
    msgs.push({ role: 'assistant', content: QUESTIONS[field] });
    if (value) {
      msgs.push({ role: 'user', content: formatFieldDisplay(field, value) });
    } else {
      break;
    }
  }
  return msgs;
}

function getNextField(brief: ProjectBrief): Field | null {
  for (const field of FIELD_ORDER) {
    if (!getBriefField(brief, field)) return field;
  }
  return null;
}

export function InceptionChat({ brief, onBriefUpdate }: Props) {
  const [messages, setMessages] = useState<ChatMessage[]>(() => briefToMessages(brief));
  const [input, setInput] = useState('');
  const [saving, setSaving] = useState(false);
  const [proposing, setProposing] = useState(false);
  const bottomRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    setMessages(briefToMessages(brief));
  }, [brief.id]);

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages]);

  useEffect(() => {
    inputRef.current?.focus();
  }, [brief.id]);

  const nextField = getNextField(brief);
  const allFilled = nextField === null;
  const canPropose = allFilled && brief.status === 'intake';

  const send = async () => {
    const text = input.trim();
    if (!text || saving || !nextField) return;
    setInput('');
    setMessages(prev => [...prev, { role: 'user', content: text }]);
    setSaving(true);
    try {
      const patch: Record<string, unknown> = {};
      if (nextField === 'title')    patch.title = text;
      if (nextField === 'idea')     patch.idea = text;
      if (nextField === 'stack')    patch.stack = text === 'skip' ? '' : text;
      if (nextField === 'budget')   patch.budget_cents = text === 'skip' ? 0 : Math.round(parseFloat(text) * 100) || 0;
      if (nextField === 'timeline') patch.timeline = text;
      if (nextField === 'quality')  patch.quality = text as ProjectQuality;

      const updated = await api.update(brief.id, patch as Parameters<typeof api.update>[1]);
      onBriefUpdate(updated);

      const newNextField = getNextField(updated);
      if (newNextField) {
        setMessages(prev => [...prev, { role: 'assistant', content: QUESTIONS[newNextField] }]);
      } else {
        setMessages(prev => [...prev, {
          role: 'assistant',
          content: "All set! Hit **Generate Proposal** and I'll design the team and task plan for you.",
        }]);
      }
    } finally {
      setSaving(false);
    }
  };

  const propose = async () => {
    if (!canPropose || proposing) return;
    setProposing(true);
    try {
      const updated = await api.propose(brief.id);
      onBriefUpdate(updated);
      setMessages(prev => [...prev, {
        role: 'assistant',
        content: "Proposal ready! Review the team on the right, then approve to kick off work.",
      }]);
    } finally {
      setProposing(false);
    }
  };

  return (
    <div className="flex flex-col h-full">
      {/* Header */}
      <div className="shrink-0 px-4 py-3 border-b border-border bg-gradient-to-r from-primary/5 to-transparent">
        <h3 className="text-sm font-semibold">{brief.title || 'New Project'}</h3>
        <p className="text-xs text-muted-foreground capitalize mt-0.5">{brief.status}</p>
      </div>

      {/* Messages */}
      <div className="flex-1 overflow-y-auto px-4 py-4 space-y-3">
        {messages.map((msg, i) => (
          <div key={i} className={cn('flex', msg.role === 'user' ? 'justify-end' : 'justify-start')}>
            <div className={cn(
              'max-w-[85%] rounded-2xl px-3.5 py-2 text-sm leading-relaxed',
              msg.role === 'assistant'
                ? 'bg-muted/60 text-foreground rounded-tl-sm'
                : 'bg-primary text-primary-foreground rounded-tr-sm'
            )}>
              {renderMessage(msg.content)}
            </div>
          </div>
        ))}
        {saving && (
          <div className="flex justify-start">
            <div className="bg-muted/60 rounded-2xl rounded-tl-sm px-3.5 py-2">
              <Loader2 className="h-3.5 w-3.5 animate-spin text-muted-foreground" />
            </div>
          </div>
        )}
        <div ref={bottomRef} />
      </div>

      {/* Quick-pick options */}
      {nextField === 'quality' && (
        <div className="shrink-0 px-4 pb-2 flex flex-wrap gap-1.5">
          {QUALITY_OPTIONS.map(o => (
            <button
              key={o.value}
              onClick={() => { setInput(o.value); inputRef.current?.focus(); }}
              className="flex flex-col rounded-lg border border-border bg-muted/30 px-3 py-1.5 text-left hover:border-primary/50 hover:bg-primary/5 transition-colors"
            >
              <span className="text-xs font-semibold">{o.label}</span>
              <span className="text-xs text-muted-foreground">{o.hint}</span>
            </button>
          ))}
        </div>
      )}
      {nextField === 'timeline' && (
        <div className="shrink-0 px-4 pb-2 flex flex-wrap gap-1.5">
          {TIMELINE_OPTIONS.map(o => (
            <button
              key={o.value}
              onClick={() => { setInput(o.value); inputRef.current?.focus(); }}
              className="rounded-lg border border-border bg-muted/30 px-3 py-1.5 text-xs font-medium hover:border-primary/50 hover:bg-primary/5 transition-colors"
            >
              {o.label}
            </button>
          ))}
        </div>
      )}

      {/* Input */}
      <div className="shrink-0 border-t border-border px-3 py-2.5">
        {!allFilled ? (
          <div className="flex items-center gap-2">
            <input
              ref={inputRef}
              value={input}
              onChange={e => setInput(e.target.value)}
              onKeyDown={e => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); send(); } }}
              disabled={saving}
              placeholder="Answer…"
              className="qr-input flex-1"
            />
            <button
              onClick={send}
              disabled={!input.trim() || saving}
              className="flex items-center justify-center h-8 w-8 rounded-lg bg-primary text-primary-foreground hover:bg-primary/90 disabled:opacity-40 transition-colors"
            >
              <Send className="h-3.5 w-3.5" />
            </button>
          </div>
        ) : canPropose ? (
          <button
            onClick={propose}
            disabled={proposing}
            className="flex w-full items-center justify-center gap-2 rounded-lg bg-primary px-4 py-2 text-sm font-semibold text-primary-foreground hover:bg-primary/90 disabled:opacity-50 transition-colors"
          >
            {proposing ? <Loader2 className="h-4 w-4 animate-spin" /> : <Sparkles className="h-4 w-4" />}
            {proposing ? 'Generating…' : 'Generate Proposal'}
          </button>
        ) : null}
      </div>
    </div>
  );
}
