'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useRef, useState } from 'react';
import { useRouter } from 'next/navigation';
import { Brain, Lightbulb, Loader2, MessageSquare, Send, Sparkles, Users } from 'lucide-react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { cn } from '@/lib/utils';
import { request } from '@/lib/api-core';
import { markdownComponents } from '@/components/chat/code-block';

interface DiscoveryMsg { role: 'user' | 'assistant'; content: string }

type InceptionBriefDraft = {
  title: string; idea: string; stack: string; quality: string;
  timeline: string; budget_cents: number; open_questions: string[];
};

export function ProjectDiscoveryChat({ onReady, onNameGenerated }: {
  onReady: (description: string, name: string, stack: string) => void;
  onNameGenerated?: (name: string) => void;
}) {
  const router = useRouter();
  const [thinkingLevel, setThinkingLevel] = useState<'off' | 'medium' | 'high'>('off');
  const [messages, setMessages] = useState<DiscoveryMsg[]>([{
    role: 'assistant',
    content: "Hey! What would you like to build? Describe your idea and I'll put together a plan, team, and budget estimate for your approval.",
  }]);
  const [input, setInput] = useState('');
  const [loading, setLoading] = useState(false);
  const [briefDraft, setBriefDraft] = useState<InceptionBriefDraft | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [followupOptions, setFollowupOptions] = useState<string[]>([]);
  const bottomRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLTextAreaElement>(null);
  const sessionIdRef = useRef<string | null>(null);

  useEffect(() => { bottomRef.current?.scrollIntoView({ behavior: 'smooth' }); }, [messages, briefDraft]);
  useEffect(() => { inputRef.current?.focus(); }, []);

  const send = async () => {
    if (!input.trim() || loading) return;
    const userMsg = input.trim();
    setInput('');
    setBriefDraft(null);
    setFollowupOptions([]);

    setMessages(prev => [...prev, { role: 'user', content: userMsg }]);
    setLoading(true);

    try {
      if (sessionIdRef.current === null) sessionIdRef.current = crypto.randomUUID();
      const data = await request<{ choices?: Array<{ message?: { content?: string; tool_calls?: Array<{ function: { name: string; arguments: string } }> } }> }>('/chat/completions', {
        method: 'POST',
        body: JSON.stringify({
          agent_id: 'prime', stream: false, message: userMsg,
          session_id: sessionIdRef.current, channel: 'intake',
          ...(thinkingLevel !== 'off' ? { thinking_level: thinkingLevel } : {}),
        }),
      });
      const msg = data.choices?.[0]?.message;
      const toolCalls: Array<{ function: { name: string; arguments: string } }> = msg?.tool_calls ?? [];

      let displayContent = (msg?.content || '').trim();
      let followupQuestion: { question: string; options?: string[] } | null = null;

      for (const tc of toolCalls) {
        try {
          const args = JSON.parse(tc.function.arguments);
          if (tc.function.name === 'produce_project_brief') {
            const parsed: InceptionBriefDraft = {
              title: args.title || '', idea: args.idea || '',
              stack: args.stack || 'auto', quality: args.quality || 'mvp',
              timeline: args.timeline || 'normal', budget_cents: args.budget_cents || 0,
              open_questions: args.open_questions || [],
            };
            setBriefDraft(parsed);
            onNameGenerated?.(parsed.title);
            if (!displayContent) {
              displayContent = args.status === 'proposed'
                ? "Here's the project brief. Review the details and click **Propose Team & Budget** when ready."
                : "Here's what I have so far. I still need a few more details.";
            }
          } else if (tc.function.name === 'ask_followup_question') {
            followupQuestion = { question: args.question, options: args.options };
            if (!displayContent) displayContent = args.question;
          }
        } catch {}
      }

      if (displayContent || followupQuestion) {
        setMessages(prev => [...prev, { role: 'assistant', content: displayContent || (followupQuestion?.question ?? '') }]);
      }
      setFollowupOptions(followupQuestion?.options?.length ? followupQuestion.options : []);
    } catch {
      setMessages(prev => [...prev, { role: 'assistant', content: "Sorry, something went wrong. Try again?" }]);
    } finally {
      setLoading(false);
    }
  };

  const launchInception = async () => {
    if (!briefDraft || submitting) return;
    setSubmitting(true);
    try {
      const brief = await request<{ id: string; error?: string }>('/project-briefs', {
        method: 'POST',
        body: JSON.stringify({
          title: briefDraft.title, idea: briefDraft.idea,
          stack: briefDraft.stack !== 'auto' ? briefDraft.stack : '',
          quality: briefDraft.quality || 'mvp',
          timeline: briefDraft.timeline || 'normal',
          budget_cents: briefDraft.budget_cents || 0,
        }),
      });

      await request(`/project-briefs/${brief.id}/propose`, { method: 'POST', body: '{}' });

      router.push('/code?tab=inception');
    } catch (e) {
      setMessages(prev => [...prev, {
        role: 'assistant',
        content: `Something went wrong creating the brief: ${e instanceof Error ? e.message : String(e)}`,
      }]);
      setSubmitting(false);
    }
  };

  return (
    <div className="flex h-full flex-col">
      <div className="flex h-14 shrink-0 items-center gap-3 border-b border-border px-6">
        <div className="flex h-8 w-8 items-center justify-center rounded-xl bg-primary">
          <Sparkles className="h-4 w-4 text-primary-foreground" />
        </div>
        <div className="flex-1">
          <p className="text-sm font-semibold">Prime</p>
          <p className="text-xs text-muted-foreground">Plan · Team · Approve · Build</p>
        </div>
        <button
          type="button"
          title="Cycle thinking level"
          onClick={() => setThinkingLevel(l => l === 'off' ? 'medium' : l === 'medium' ? 'high' : 'off')}
          className={cn('flex items-center gap-1 rounded-lg px-2 py-1 text-xs transition-colors',
            thinkingLevel === 'off' ? 'text-muted-foreground hover:bg-accent hover:text-foreground'
            : thinkingLevel === 'medium' ? 'text-amber-400 bg-amber-400/10'
            : 'text-violet-400 bg-violet-400/10')}
        >
          <Brain className="h-3.5 w-3.5" />
          <span>{thinkingLevel === 'off' ? 'Think: Off' : thinkingLevel === 'medium' ? 'Think: Normal' : 'Think: High'}</span>
        </button>
      </div>

      <div className="flex-1 overflow-y-auto px-6 py-4 space-y-4">
        {messages.map((msg, i) => (
          <div key={i} className={cn('flex', msg.role === 'user' ? 'justify-end' : 'justify-start')}>
            {msg.role === 'assistant' && (
              <div className="mr-2 mt-1 flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-primary/15">
                <Sparkles className="h-3 w-3 text-primary" />
              </div>
            )}
            <div className={cn(
              'max-w-[75%] rounded-2xl px-4 py-2.5 text-sm leading-relaxed',
              msg.role === 'user'
                ? 'rounded-br-sm bg-primary text-primary-foreground'
                : 'rounded-bl-sm bg-muted text-foreground'
            )}>
              <ReactMarkdown remarkPlugins={[remarkGfm]} components={markdownComponents}>
                {msg.content}
              </ReactMarkdown>
            </div>
          </div>
        ))}

        {loading && (
          <div className="flex justify-start">
            <div className="mr-2 mt-1 flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-primary/15">
              <Sparkles className="h-3 w-3 text-primary" />
            </div>
            <div className="rounded-2xl rounded-bl-sm bg-muted px-4 py-3">
              <div className="flex gap-1">
                {[0, 1, 2].map(i => (
                  <span key={i} className="h-1.5 w-1.5 rounded-full bg-muted-foreground/50 animate-bounce"
                    style={{ animationDelay: `${i * 150}ms` }} />
                ))}
              </div>
            </div>
          </div>
        )}

        {followupOptions.length > 0 && !loading && (
          <div className="flex flex-wrap gap-2 pl-8">
            {followupOptions.map((opt, i) => (
              <button key={i} onClick={() => { setInput(opt); setFollowupOptions([]); }}
                className="rounded-full border border-border bg-muted/50 px-3 py-1.5 text-xs text-foreground hover:bg-muted transition-colors">
                {opt}
              </button>
            ))}
          </div>
        )}

        {briefDraft && !loading && (
          <div className="rounded-2xl border border-primary/30 bg-primary/5 p-4 space-y-3">
            <p className="text-xs font-semibold text-primary uppercase tracking-wider flex items-center gap-1.5">
              <Lightbulb className="h-3.5 w-3.5" /> Project Brief
            </p>
            <div className="space-y-1.5">
              <p className="text-sm font-bold">{briefDraft.title}</p>
              <p className="text-xs text-muted-foreground leading-relaxed">{briefDraft.idea}</p>
            </div>
            <div className="flex flex-wrap gap-1.5">
              {briefDraft.stack && briefDraft.stack !== 'auto' && (
                <span className="rounded-full border border-border px-2 py-0.5 text-xs text-muted-foreground">{briefDraft.stack}</span>
              )}
              <span className="rounded-full border border-border px-2 py-0.5 text-xs text-muted-foreground capitalize">{briefDraft.quality || 'mvp'}</span>
              <span className="rounded-full border border-border px-2 py-0.5 text-xs text-muted-foreground capitalize">{(briefDraft.timeline || 'normal').replace('_', ' ')}</span>
              {briefDraft.budget_cents > 0 && (
                <span className="rounded-full border border-border px-2 py-0.5 text-xs text-muted-foreground">${briefDraft.budget_cents / 100} budget</span>
              )}
            </div>
            {briefDraft.open_questions?.length > 0 && (
              <div className="rounded-xl border border-amber-500/20 bg-amber-500/5 px-3.5 py-2.5">
                <p className="text-xs font-semibold text-amber-600 mb-1.5">Still need clarity on:</p>
                <ul className="space-y-0.5">
                  {briefDraft.open_questions.map((q, i) => (
                    <li key={i} className="text-xs text-muted-foreground flex items-start gap-1.5">
                      <span className="mt-0.5 h-1 w-1 rounded-full bg-amber-500/50 shrink-0" />
                      {q}
                    </li>
                  ))}
                </ul>
              </div>
            )}
            <button
              onClick={launchInception}
              disabled={submitting || (briefDraft.open_questions?.length > 0)}
              className="w-full flex items-center justify-center gap-2 rounded-xl bg-primary py-2.5 text-sm font-semibold text-primary-foreground hover:bg-primary/90 disabled:opacity-50 transition-colors"
            >
              {submitting
                ? <><Loader2 className="h-4 w-4 animate-spin" /> Preparing proposal…</>
                : briefDraft.open_questions?.length > 0
                  ? <><MessageSquare className="h-4 w-4" /> Answer the questions above first</>
                  : <><Users className="h-4 w-4" /> Propose Team &amp; Budget</>
              }
            </button>
            <button onClick={() => setBriefDraft(null)}
              className="w-full text-xs text-muted-foreground hover:text-foreground transition-colors py-1">
              Not quite right — let me adjust
            </button>
          </div>
        )}

        <div ref={bottomRef} />
      </div>

      <div className="border-t border-border px-4 py-3">
        <div className="flex items-end gap-2">
          <textarea
            ref={inputRef}
            value={input}
            onChange={e => setInput(e.target.value)}
            onKeyDown={e => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); send(); } }}
            placeholder="What do you want to build?"
            rows={1}
            className="qr-textarea flex-1 resize-none text-xs"
          />
          <button onClick={send} disabled={loading || !input.trim()}
            className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-primary text-primary-foreground hover:bg-primary/90 disabled:opacity-40 transition-colors">
            {loading ? <Loader2 className="h-4 w-4 animate-spin" /> : <Send className="h-4 w-4" />}
          </button>
        </div>
        <p className="mt-1.5 text-center text-xs text-muted-foreground/50">
          Enter to send · Shift+Enter for new line
        </p>
      </div>
    </div>
  );
}
