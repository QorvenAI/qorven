'use client';
// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useMemo, useRef, useState } from 'react';
import { Bot, CheckCircle2, Lock, Loader2, Send, Unlock, Wrench } from 'lucide-react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { cn } from '@/lib/utils';
import { markdownComponents } from '@/components/chat/code-block';
import { FileMentionPicker } from './file-mention-picker';
import { SlashCommandPalette, type SlashCommand } from './slash-command-palette';
import { ContextPanel } from './context-panel';
import type { ChatMsg, FileNode } from './code-types';

interface CodeChatSidebarProps {
  messages: ChatMsg[];
  isLoading: boolean;
  onSend: (msg: string) => void;
  onCommand?: (cmd: SlashCommand) => void;
  files?: FileNode[];
  planMode?: boolean;
  onPlanModeChange?: (v: boolean) => void;
  thinkingLevel: 'off' | 'medium' | 'high';
  onThinkingLevelChange: (level: 'off' | 'medium' | 'high') => void;
  onDelegated?: (projectId: string) => void;
  contextFiles?: string[];
  onRemoveContextFile?: (path: string) => void;
  addContextOpen?: boolean;
  onAddContextFile?: (path: string) => void;
  onAddContextClose?: () => void;
  showInitChip?: boolean;
  onDismissInitChip?: () => void;
}

export function CodeChatSidebar({
  messages,
  isLoading,
  onSend,
  onCommand,
  files = [],
  planMode = false,
  onPlanModeChange,
  thinkingLevel,
  onThinkingLevelChange,
  onDelegated,
  contextFiles = [],
  onRemoveContextFile,
  addContextOpen = false,
  onAddContextFile,
  onAddContextClose,
  showInitChip = false,
  onDismissInitChip,
}: CodeChatSidebarProps) {
  const [input, setInput] = useState('');
  const bottomRef = useRef<HTMLDivElement>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  useEffect(() => { bottomRef.current?.scrollIntoView({ behavior: 'smooth' }); }, [messages]);

  // When Prime's response contains [DELEGATED:coder:<projectId>], notify parent to switch to build tab
  useEffect(() => {
    if (!onDelegated) return;
    const last = messages[messages.length - 1];
    if (!last || last.role !== 'assistant') return;
    const match = last.content?.match(/\[DELEGATED:coder:([^\]]*)\]/);
    if (match) {
      onDelegated(match[1] ?? '');
    }
  }, [messages, onDelegated]);

  // ---- Background job tracking ----
  // jobID → true means done (BACKGROUND_JOB_DONE seen in a later message)
  const completedJobs = useMemo(() => {
    const done = new Set<string>();
    for (const m of messages) {
      const matches = m.content?.matchAll(/\[BACKGROUND_JOB_DONE:([^\]]+)\]/g) ?? [];
      for (const match of matches) {
        if (match[1]) done.add(match[1]);
      }
    }
    return done;
  }, [messages]);

  // ---- Cumulative token tracking ----
  const { totalTokens } = useMemo(() => {
    let totalTokens = 0;
    for (const m of messages) {
      if (m.usage) totalTokens += m.usage.total_tokens;
    }
    return { totalTokens };
  }, [messages]);

  const tokenColor = totalTokens > 80000
    ? 'text-destructive'
    : totalTokens > 40000
    ? 'text-amber-400'
    : 'text-muted-foreground/50';

  // ---- Picker state ----
  // File mention: trigger on @
  const atIdx = input.lastIndexOf('@');
  const atQuery = atIdx >= 0 ? input.slice(atIdx + 1) : '';
  const showFilePicker = atIdx >= 0 && !atQuery.includes(' ') && atQuery.length < 60;

  // Slash command: trigger on / at start of input
  const showSlashPalette = input.startsWith('/') && !input.includes(' ');
  const slashQuery = showSlashPalette ? input.slice(1) : '';

  function insertFileMention(path: string) {
    const before = input.slice(0, atIdx);
    setInput(before + '@' + path + ' ');
    textareaRef.current?.focus();
  }

  function handleSlashCommand(cmd: SlashCommand) {
    setInput('');
    onCommand?.(cmd);
  }

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
        {onPlanModeChange && (
          <button
            type="button"
            title={planMode ? 'Plan mode — no file writes. Click to switch to Build.' : 'Build mode. Click to switch to Plan (read-only).'}
            onClick={() => onPlanModeChange(!planMode)}
            className={cn(
              'flex items-center gap-1 rounded px-1.5 py-0.5 text-xs transition-colors',
              planMode
                ? 'bg-amber-400/10 text-amber-400'
                : 'text-muted-foreground hover:bg-accent',
            )}
          >
            {planMode ? <Lock className="h-3 w-3" /> : <Unlock className="h-3 w-3" />}
            {planMode ? 'Plan' : 'Build'}
          </button>
        )}
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

      {/* /init success chip */}
      {showInitChip && (
        <div className="mx-3 mt-2 flex items-center gap-2 rounded-lg border border-emerald-500/30 bg-emerald-500/10 px-3 py-2 text-xs text-emerald-400">
          <CheckCircle2 className="h-3.5 w-3.5 shrink-0" />
          <span className="flex-1">AGENTS.md created — Prime now knows your project</span>
          <button type="button" onClick={onDismissInitChip} className="ml-1 hover:text-emerald-300">
            <span className="sr-only">Dismiss</span>×
          </button>
        </div>
      )}

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
                <>
                  <ReactMarkdown remarkPlugins={[remarkGfm]} components={markdownComponents}>
                    {(m.content || (m.streaming ? '…' : '')).replace(/\[BACKGROUND_JOB(?:_DONE)?:[^\]]+\][^\n]*/g, '')}
                  </ReactMarkdown>
                  {/* Background job chips */}
                  {[...((m.content || '').matchAll(/\[BACKGROUND_JOB:([^\]]+)\]/g) ?? [])].map(match => {
                    const jobId = match[1];
                    if (!jobId) return null;
                    const done = completedJobs.has(jobId);
                    return (
                      <div key={jobId} className={cn(
                        'mt-2 inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-2xs font-medium',
                        done
                          ? 'bg-emerald-500/10 text-emerald-400'
                          : 'bg-primary/10 text-primary',
                      )}>
                        {done
                          ? <CheckCircle2 className="h-3 w-3" />
                          : <Loader2 className="h-3 w-3 animate-spin" />}
                        {done ? 'Done' : 'Working…'}
                        <span className="font-mono opacity-50">{jobId.slice(0, 8)}</span>
                      </div>
                    );
                  })}
                </>
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

      {/* Context files — shown above input when present */}
      {contextFiles.length > 0 && onRemoveContextFile && (
        <ContextPanel files={contextFiles} onRemove={onRemoveContextFile} />
      )}

      {/* Input */}
      <div className="relative border-t border-border px-3 py-2">
        {/* File mention picker — rendered above textarea */}
        {showFilePicker && files.length > 0 && (
          <FileMentionPicker
            query={atQuery}
            files={files}
            onSelect={insertFileMention}
            onClose={() => setInput(input.slice(0, atIdx))}
            anchorRef={textareaRef as React.RefObject<HTMLElement>}
          />
        )}

        {/* Slash command palette — rendered above textarea */}
        {showSlashPalette && (
          <SlashCommandPalette
            query={slashQuery}
            onSelect={handleSlashCommand}
            onClose={() => setInput('')}
            anchorRef={textareaRef as React.RefObject<HTMLElement>}
          />
        )}

        {/* /add context picker — triggered by /add command from parent */}
        {addContextOpen && files.length > 0 && onAddContextFile && (
          <FileMentionPicker
            query=""
            files={files}
            onSelect={(path) => { onAddContextFile(path); onAddContextClose?.(); }}
            onClose={() => onAddContextClose?.()}
            anchorRef={textareaRef as React.RefObject<HTMLElement>}
          />
        )}

        <div className="flex items-end gap-2">
          <textarea
            ref={textareaRef}
            value={input}
            onChange={e => setInput(e.target.value)}
            onKeyDown={e => {
              if (e.key === 'Enter' && !e.shiftKey) {
                if (showFilePicker || showSlashPalette || addContextOpen) { e.preventDefault(); return; }
                e.preventDefault();
                send();
              }
              if (e.key === 'Escape') {
                if (showFilePicker) setInput(input.slice(0, atIdx));
                if (showSlashPalette) setInput('');
                if (addContextOpen) onAddContextClose?.();
              }
            }}
            placeholder="Ask Prime… (@file, /command)"
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
        <div className="mt-1 flex items-center justify-between">
          <p className="text-2xs text-muted-foreground/50">Type <kbd className="bg-muted/60 px-0.5 rounded text-2xs">@</kbd> for files &middot; <kbd className="bg-muted/60 px-0.5 rounded text-2xs">/</kbd> for commands</p>
          {totalTokens > 0 && (
            <span className={cn('font-mono text-2xs', tokenColor)}>
              {totalTokens > 1000 ? `${(totalTokens / 1000).toFixed(1)}K` : totalTokens} tokens
            </span>
          )}
        </div>
      </div>
    </div>
  );
}
