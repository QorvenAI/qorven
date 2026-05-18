'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState, useRef, useCallback } from 'react';
import { useParams } from 'next/navigation';
import Link from 'next/link';
import { sessions, chat as chatApi } from '@/lib/api';
import { useStore } from '@/store';
import { ErrorBoundary } from '@/components/error-boundary';
import { ChannelBadge } from '@/components/channel-badge';
import { soulGradient } from '@/components/soul-card';
import { ArrowLeft, AlertCircle, Send, Loader2, RefreshCw, Bot, Zap, MessageSquare } from 'lucide-react';
import { cn } from '@/lib/utils';
import type { Session, Message } from '@/types';

// ─── Chat Page — full WhatsApp/iMessage style with real-time streaming ─────────

export default function SessionDetailPage() {
  const { id } = useParams<{ id: string }>();
  const [session, setSession] = useState<Session | null>(null);
  const [messages, setMessages] = useState<Message[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [input, setInput] = useState('');
  const [sending, setSending] = useState(false);
  const [streamingContent, setStreamingContent] = useState('');
  const bottomRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLTextAreaElement>(null);

  // Real-time: listen for incoming messages on this session via Zustand store
  const incomingMessages = useStore((s) => s.incomingMessages);
  const clearIncomingMessages = useStore((s) => s.clearIncomingMessages);
  const souls = useStore((s) => s.souls);

  const soul = session ? souls.find(s => s.id === session.agent_id) : null;

  const load = useCallback(() => {
    setLoading(true);
    setError(null);
    Promise.all([sessions.get(id), sessions.messages(id)])
      .then(([s, m]) => { setSession(s); setMessages(Array.isArray(m) ? m : []); setLoading(false); })
      .catch((e) => { setError(e.message); setLoading(false); });
  }, [id]);

  useEffect(() => { load(); }, [load]);

  // Auto-scroll to bottom when new messages arrive
  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages, streamingContent]);

  // Real-time: inject WebSocket messages into the chat
  useEffect(() => {
    const relevant = incomingMessages.filter(m => m.sessionId === id);
    if (relevant.length === 0) return;

    relevant.forEach(m => {
      setMessages(prev => {
        const exists = prev.some(p => p.content === m.content && p.role === m.role);
        if (exists) return prev;
        return [...prev, {
          role: m.role as 'user' | 'assistant' | 'system' | 'tool',
          content: m.content,
          timestamp: new Date().toISOString(),
          channel: m.source || undefined,
          sender_name: m.agentId ? undefined : 'You',
        } as Message];
      });
    });
    clearIncomingMessages(id);
    setStreamingContent(''); // clear any streaming indicator
  }, [incomingMessages, id, clearIncomingMessages]);

  const send = async () => {
    const text = input.trim();
    if (!text || sending || !session) return;
    setInput('');
    setSending(true);

    // Optimistically add user message
    const userMsg: Message = { role: 'user', content: text, timestamp: new Date().toISOString() };
    setMessages(prev => [...prev, userMsg]);

    try {
      // Send via streaming API — response will arrive via WebSocket
      const res = await chatApi.send({
        session_id: id,
        agent_id: session.agent_id,
        message: text,
        stream: true,
      });

      // Stream the response
      const reader = res.body?.getReader();
      const decoder = new TextDecoder();
      let accumulated = '';

      if (reader) {
        while (true) {
          const { done, value } = await reader.read();
          if (done) break;
          for (const line of decoder.decode(value, { stream: true }).split('\n')) {
            if (!line.startsWith('data: ')) continue;
            const raw = line.slice(6);
            if (raw === '[DONE]') continue;
            try {
              const evt = JSON.parse(raw);
              const delta = evt.choices?.[0]?.delta?.content ||
                (evt.type === 'text_delta' ? evt.data?.content ?? evt.data : null);
              if (delta) {
                accumulated += delta;
                setStreamingContent(accumulated);
              }
            } catch { /* ignore parse errors */ }
          }
        }
      }

      if (accumulated) {
        const assistantMsg: Message = { role: 'assistant', content: accumulated, timestamp: new Date().toISOString() };
        setMessages(prev => [...prev, assistantMsg]);
        setStreamingContent('');
      }
    } catch (e) {
      // Show error as a system message
      setMessages(prev => [...prev, { role: 'system', content: `Error: ${e instanceof Error ? e.message : 'Failed to send'}`, timestamp: new Date().toISOString() } as Message]);
    } finally {
      setSending(false);
      setStreamingContent('');
      inputRef.current?.focus();
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      send();
    }
  };

  if (loading) return (
    <div className="full-bleed flex flex-col h-[calc(100vh-var(--header-height))]">
      <div className="flex items-center gap-3 px-4 py-3 border-b border-border shrink-0">
        <div className="h-8 w-8 animate-pulse rounded-full bg-muted" />
        <div className="space-y-1.5">
          <div className="h-4 w-32 animate-pulse rounded bg-muted" />
          <div className="h-3 w-20 animate-pulse rounded bg-muted" />
        </div>
      </div>
      <div className="flex-1 p-4 space-y-4">
        {Array.from({ length: 5 }).map((_, i) => (
          <div key={i} className={cn('flex gap-2', i % 2 === 1 && 'flex-row-reverse')}>
            <div className="h-8 w-8 animate-pulse rounded-full bg-muted shrink-0" />
            <div className={cn('animate-pulse rounded-2xl bg-muted', i % 2 === 0 ? 'h-12 w-56' : 'h-8 w-40')} />
          </div>
        ))}
      </div>
    </div>
  );

  if (error) return (
    <div className="flex flex-col items-center py-16 text-center">
      <AlertCircle className="h-8 w-8 text-destructive mb-2" />
      <p className="text-sm text-destructive">{error}</p>
      <button onClick={load} className="mt-3 rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90">Retry</button>
    </div>
  );

  if (!session) return <div className="py-16 text-center text-muted-foreground">Session not found</div>;

  const channelIcon = session.channel && session.channel !== 'web' ? session.channel : null;

  return (
    <ErrorBoundary fallbackTitle="Failed to load session">
      <div className="full-bleed flex flex-col h-[calc(100vh-var(--header-height))]">

        {/* Chat header — WhatsApp/Telegram style */}
        <div className="flex items-center gap-3 px-4 py-2.5 border-b border-border bg-card shrink-0">
          <Link href="/sessions"
            className="h-8 w-8 flex items-center justify-center rounded-lg text-muted-foreground hover:bg-accent transition-colors shrink-0">
            <ArrowLeft className="h-4 w-4" />
          </Link>

          {/* Agent avatar */}
          <div className={cn(
            'flex h-9 w-9 shrink-0 items-center justify-center rounded-full text-sm font-bold text-white',
            soul ? soulGradient(soul.display_name) : 'bg-primary/20',
          )}>
            {soul ? soul.display_name.charAt(0).toUpperCase() : <Bot className="h-4 w-4 text-primary" />}
          </div>

          <div className="flex-1 min-w-0">
            <div className="flex items-center gap-2">
              <h1 className="text-sm font-semibold truncate">
                {soul?.display_name ?? `Agent ${session.agent_id.slice(0, 8)}`}
              </h1>
              {channelIcon && <ChannelBadge type={channelIcon as any} />}
            </div>
            <p className="text-xs text-muted-foreground truncate">
              {session.label || `Session ${id.slice(0, 8)}`}
              {' · '}
              {new Date(session.created_at).toLocaleDateString([], { month: 'short', day: 'numeric' })}
            </p>
          </div>

          <button onClick={load} title="Refresh messages"
            className="h-8 w-8 flex items-center justify-center rounded-lg text-muted-foreground hover:bg-accent cursor-pointer transition-colors shrink-0">
            <RefreshCw className="h-3.5 w-3.5" />
          </button>
        </div>

        {/* Messages — the chat timeline */}
        <div className="flex-1 overflow-y-auto px-4 py-4 space-y-2">
          {messages.length === 0 && !streamingContent ? (
            <div className="flex flex-col items-center justify-center h-full gap-3 text-center">
              <div className={cn(
                'flex h-16 w-16 items-center justify-center rounded-full text-xl font-bold text-white',
                soul ? soulGradient(soul.display_name) : 'bg-primary/20',
              )}>
                {soul ? soul.display_name.charAt(0).toUpperCase() : <MessageSquare className="h-8 w-8 text-primary" />}
              </div>
              <div>
                <p className="text-sm font-medium">{soul?.display_name ?? 'Agent'}</p>
                <p className="text-xs text-muted-foreground mt-0.5">Start a conversation below</p>
              </div>
            </div>
          ) : (
            <>
              {messages.map((msg, i) => (
                <ChatBubble key={i} msg={msg} soul={soul} />
              ))}

              {/* Streaming indicator — shows while agent is typing */}
              {(streamingContent || sending) && (
                <div className="flex gap-2 items-end">
                  <div className={cn(
                    'flex h-7 w-7 shrink-0 items-center justify-center rounded-full text-xs font-bold text-white',
                    soul ? soulGradient(soul.display_name) : 'bg-muted',
                  )}>
                    {soul ? soul.display_name.charAt(0).toUpperCase() : <Bot className="h-3.5 w-3.5" />}
                  </div>
                  <div className="rounded-2xl rounded-bl-sm border border-border bg-card px-4 py-3 max-w-[70%]">
                    {streamingContent ? (
                      <p className="text-sm whitespace-pre-wrap">{streamingContent}<span className="animate-pulse">▌</span></p>
                    ) : (
                      <div className="flex gap-1.5 items-center py-0.5">
                        <span className="h-2 w-2 rounded-full bg-muted-foreground/40 animate-bounce" style={{ animationDelay: '0ms' }} />
                        <span className="h-2 w-2 rounded-full bg-muted-foreground/40 animate-bounce" style={{ animationDelay: '150ms' }} />
                        <span className="h-2 w-2 rounded-full bg-muted-foreground/40 animate-bounce" style={{ animationDelay: '300ms' }} />
                      </div>
                    )}
                  </div>
                </div>
              )}
            </>
          )}
          <div ref={bottomRef} />
        </div>

        {/* Input bar — WhatsApp style */}
        <div className="shrink-0 px-4 py-3 border-t border-border bg-card">
          <div className="flex items-end gap-2">
            <div className="flex-1 rounded-2xl border border-border bg-muted/30 px-4 py-2.5 focus-within:border-primary transition-colors">
              <textarea
                ref={inputRef}
                value={input}
                onChange={e => setInput(e.target.value)}
                onKeyDown={handleKeyDown}
                placeholder={`Message ${soul?.display_name ?? 'agent'}…`}
                rows={1}
                style={{ resize: 'none', minHeight: '24px', maxHeight: '120px' }}
                className="w-full bg-transparent text-sm outline-none placeholder:text-muted-foreground/50 overflow-y-auto"
                onInput={e => {
                  const t = e.target as HTMLTextAreaElement;
                  t.style.height = 'auto';
                  t.style.height = Math.min(t.scrollHeight, 120) + 'px';
                }}
              />
            </div>
            <button
              onClick={send}
              disabled={!input.trim() || sending}
              className={cn(
                'h-10 w-10 shrink-0 flex items-center justify-center rounded-full transition-all cursor-pointer',
                input.trim() && !sending
                  ? 'bg-primary text-primary-foreground hover:bg-primary/90 scale-100'
                  : 'bg-muted text-muted-foreground scale-90 cursor-not-allowed',
              )}
            >
              {sending
                ? <Loader2 className="h-4 w-4 animate-spin" />
                : <Send className="h-4 w-4" />
              }
            </button>
          </div>
          <p className="text-xs text-muted-foreground mt-1.5 px-1">Enter to send · Shift+Enter for new line</p>
        </div>
      </div>
    </ErrorBoundary>
  );
}

// ─── Chat Bubble ──────────────────────────────────────────────────────────────

function ChatBubble({ msg, soul }: { msg: Message; soul: any }) {
  const isUser = msg.role === 'user';
  const isSystem = msg.role === 'system';
  const ts = msg.timestamp ? new Date(msg.timestamp) : null;

  if (isSystem) return (
    <div className="flex justify-center">
      <span className="rounded-full bg-muted px-3 py-1 text-xs text-muted-foreground">
        {msg.content}
      </span>
    </div>
  );

  return (
    <div className={cn('flex gap-2 items-end', isUser && 'flex-row-reverse')}>
      {/* Avatar */}
      {!isUser && (
        <div className={cn(
          'flex h-7 w-7 shrink-0 items-center justify-center rounded-full text-xs font-bold text-white',
          soul ? soulGradient(soul.display_name) : 'bg-muted',
        )}>
          {soul ? soul.display_name.charAt(0).toUpperCase() : <Bot className="h-3 w-3" />}
        </div>
      )}

      <div className={cn(
        'rounded-2xl px-4 py-2.5 max-w-[70%] text-sm leading-relaxed',
        isUser
          ? 'rounded-br-sm bg-primary text-primary-foreground'
          : 'rounded-bl-sm bg-card border border-border text-foreground',
      )}>
        {/* Channel badge for external messages */}
        {msg.channel && msg.channel !== 'web' && !isUser && (
          <div className="mb-1.5">
            <ChannelBadge type={msg.channel as any} />
          </div>
        )}
        {msg.sender_name && !isUser && (
          <p className="text-xs font-semibold text-muted-foreground mb-1">{msg.sender_name}</p>
        )}
        <p className="whitespace-pre-wrap">{msg.content}</p>
        {ts && (
          <p className={cn('text-xs mt-1.5 text-right',
            isUser ? 'text-primary-foreground/60' : 'text-muted-foreground')}>
            {ts.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}
          </p>
        )}
      </div>

      {/* User avatar */}
      {isUser && (
        <div className="h-7 w-7 shrink-0 flex items-center justify-center rounded-full bg-primary text-primary-foreground text-xs font-bold">
          Y
        </div>
      )}
    </div>
  );
}
