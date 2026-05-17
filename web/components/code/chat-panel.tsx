'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useEffect, useRef, useState } from 'react';
import { Send } from 'lucide-react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { markdownComponents } from '@/components/chat/code-block';
import { ToolCallBlock } from '@/components/chat/tool-call-block';
import { TelemetryLog } from '@/components/chat/telemetry-log';
import { ApprovalCards } from '@/components/chat/approval-card';
import { GitHubPRCards } from '@/components/chat/github-pr-card';
import { CommitApprovalCards } from '@/components/chat/commit-approval-card';
import type { ChatMsg } from './code-types';

export function ChatPanel({ messages, onSend, isLoading, sessionId }: {
  messages: ChatMsg[]; onSend: (m: string) => void; isLoading: boolean; sessionId: string;
}) {
  const [input, setInput] = useState('');
  const bottomRef = useRef<HTMLDivElement>(null);
  useEffect(() => { bottomRef.current?.scrollIntoView({ behavior: 'smooth' }); }, [messages]);
  const send = () => { if (input.trim() && !isLoading) { onSend(input.trim()); setInput(''); } };

  return (
    <div className="flex h-full flex-col">
      <div className="flex-1 overflow-y-auto p-3 space-y-3">
        {messages.map((msg, i) => (
          <div key={i}>
            {msg.role === 'user' ? (
              <div className="flex justify-end">
                <div className="max-w-[90%] rounded-xl rounded-br-sm bg-primary/10 border border-primary/20 px-3 py-1.5 text-xs">{msg.content}</div>
              </div>
            ) : (
              <div className="space-y-1.5">
                {msg.tools?.map((t, j) => <ToolCallBlock key={j} name={t.name} args={t.args} result={t.result} />)}
                {msg.content && (
                  <div className="prose prose-xs dark:prose-invert max-w-none text-xs leading-relaxed">
                    <ReactMarkdown remarkPlugins={[remarkGfm]} components={markdownComponents}>
                      {msg.content}
                    </ReactMarkdown>
                    {msg.streaming && <span className="inline-block h-3 w-0.5 bg-primary animate-pulse ml-0.5" />}
                  </div>
                )}
              </div>
            )}
          </div>
        ))}
        {isLoading && messages[messages.length - 1]?.role !== 'assistant' && (
          <TelemetryLog sessionId={sessionId} active={isLoading} />
        )}
        <ApprovalCards sessionId={sessionId} />
        <GitHubPRCards sessionId={sessionId} />
        <CommitApprovalCards sessionId={sessionId} />
        <div ref={bottomRef} />
      </div>
      <div className="border-t border-border p-2">
        <div className="flex items-end gap-1.5">
          <textarea
            value={input}
            onChange={e => setInput(e.target.value)}
            onKeyDown={e => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); send(); } }}
            rows={2}
            placeholder="Ask Prime Coder to build, fix, or explain…"
            className="qr-textarea flex-1 resize-none text-xs"
          />
          <button onClick={send} disabled={isLoading || !input.trim()}
            className="flex h-[52px] w-8 items-center justify-center rounded-lg bg-primary text-primary-foreground hover:bg-primary/90 disabled:opacity-40 shrink-0">
            <Send className="h-3.5 w-3.5" />
          </button>
        </div>
      </div>
    </div>
  );
}
