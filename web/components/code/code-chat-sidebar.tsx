'use client';
// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useEffect, useRef, useState } from 'react';
import { Bot, Loader2, Send, Wrench } from 'lucide-react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { cn } from '@/lib/utils';
import { markdownComponents } from '@/components/chat/code-block';
import type { ChatMsg } from './code-types';

interface CodeChatSidebarProps {
  messages: ChatMsg[];
  isLoading: boolean;
  onSend: (msg: string) => void;
  thinkingLevel: 'off' | 'medium' | 'high';
  onThinkingLevelChange: (level: 'off' | 'medium' | 'high') => void;
}

export function CodeChatSidebar({
  messages,
  isLoading,
  onSend,
  thinkingLevel,
  onThinkingLevelChange,
}: CodeChatSidebarProps) {
  const [input, setInput] = useState('');
  const bottomRef = useRef<HTMLDivElement>(null);

  useEffect(() => { bottomRef.current?.scrollIntoView({ behavior: 'smooth' }); }, [messages]);

  const send = () => {
    const text = input.trim();
    if (!text || isLoading) return;
    setInput('');
    onSend(text);
  };

  return (
    <div className="flex h-full flex-col">
      {/* Header */}
      <div className="flex h-10 shrink-0 items-center gap-2 border-b border-border px-3">
        <div className="flex h-6 w-6 items-center justify-center rounded-lg bg-primary/15">
          <Bot className="h-3.5 w-3.5 text-primary" />
        </div>
        <span className="flex-1 text-xs font-semibold">Prime</span>
        <button
          type="button"
          title="Cycle thinking level"
          onClick={() => onThinkingLevelChange(thinkingLevel === 'off' ? 'medium' : thinkingLevel === 'medium' ? 'high' : 'off')}
          className={cn(
            'flex items-center gap-1 rounded px-1.5 py-0.5 text-xs transition-colors',
            thinkingLevel === 'off'
              ? 'text-muted-foreground hover:bg-accent'
              : thinkingLevel === 'medium'
              ? 'text-amber-400 bg-amber-400/10'
              : 'text-violet-400 bg-violet-400/10',
          )}
        >
          {thinkingLevel === 'off' ? 'Think' : <span className="capitalize">{thinkingLevel}</span>}
        </button>
      </div>

      {/* Message list */}
      <div className="flex-1 overflow-y-auto px-3 py-3 space-y-3">
        {messages.map((m, i) => (
          <div key={i} className={cn('flex gap-2', m.role === 'user' ? 'justify-end' : 'justify-start')}>
            {m.role === 'assistant' && (
              <div className="mt-0.5 flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-primary/15">
                <Bot className="h-3.5 w-3.5 text-primary" />
              </div>
            )}
            <div className={cn(
              'max-w-[88%] rounded-2xl px-3 py-2 text-xs',
              m.role === 'user'
                ? 'rounded-br-sm bg-primary text-primary-foreground'
                : 'rounded-bl-sm bg-muted text-foreground',
            )}>
              {m.role === 'assistant' ? (
                <ReactMarkdown remarkPlugins={[remarkGfm]} components={markdownComponents}>
                  {m.content || (m.streaming ? '…' : '')}
                </ReactMarkdown>
              ) : (
                <p className="whitespace-pre-wrap">{m.content}</p>
              )}
              {m.tools && m.tools.length > 0 && (
                <div className="mt-1.5 space-y-0.5">
                  {m.tools.map((t, ti) => (
                    <div key={ti} className="flex items-center gap-1 rounded-lg bg-black/10 px-2 py-1">
                      <Wrench className="h-3 w-3 shrink-0 text-muted-foreground" />
                      <span className="font-mono text-xs text-muted-foreground truncate">{t.name}</span>
                    </div>
                  ))}
                </div>
              )}
            </div>
          </div>
        ))}

        {isLoading && messages[messages.length - 1]?.role !== 'assistant' && (
          <div className="flex gap-2 justify-start">
            <div className="mt-0.5 flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-primary/15">
              <Bot className="h-3.5 w-3.5 text-primary" />
            </div>
            <div className="rounded-2xl rounded-bl-sm bg-muted px-3 py-2">
              <div className="flex gap-1">
                {[0, 1, 2].map(i => (
                  <span key={i} className="h-1.5 w-1.5 rounded-full bg-muted-foreground/50 animate-bounce"
                    style={{ animationDelay: `${i * 150}ms` }} />
                ))}
              </div>
            </div>
          </div>
        )}

        <div ref={bottomRef} />
      </div>

      {/* Input */}
      <div className="border-t border-border px-3 py-2">
        <div className="flex items-end gap-2">
          <textarea
            value={input}
            onChange={e => setInput(e.target.value)}
            onKeyDown={e => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); send(); } }}
            placeholder="Ask Prime anything about this project…"
            rows={1}
            className="qr-textarea flex-1 resize-none text-xs"
          />
          <button
            onClick={send}
            disabled={isLoading || !input.trim()}
            className="flex h-8 w-8 shrink-0 items-center justify-center rounded-xl bg-primary text-primary-foreground hover:bg-primary/90 disabled:opacity-40 transition-colors"
          >
            {isLoading ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Send className="h-3.5 w-3.5" />}
          </button>
        </div>
      </div>
    </div>
  );
}
