'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useState, useEffect, useCallback } from 'react';
import { sessions as sessionsApi } from '@/lib/api-agents';
import { cn } from '@/lib/utils';
import { relativeTime } from '@/lib/relative-time';
import {
  Clock, Hash, Mail, Globe, Zap, Bot, RotateCcw, MessageSquare, Loader2,
  ChevronRight, AlertCircle, User, Cpu,
} from 'lucide-react';
import type { Session, Message } from '@/types';

// Channel display metadata
const CHANNEL_META: Record<string, { label: string; icon: typeof Hash; color: string; group: string }> = {
  web:        { label: 'Direct Chat',       icon: MessageSquare, color: 'text-primary',      group: 'chat' },
  tui:        { label: 'Terminal Chat',     icon: Hash,          color: 'text-emerald-500',  group: 'chat' },
  cron:       { label: 'Scheduled Task',    icon: Clock,         color: 'text-amber-500',    group: 'tasks' },
  task:       { label: 'Delegated Task',    icon: Zap,           color: 'text-blue-500',     group: 'tasks' },
  internal:   { label: 'Internal Request',  icon: Bot,           color: 'text-violet-500',   group: 'tasks' },
  a2a:        { label: 'Agent Request',     icon: Cpu,           color: 'text-violet-500',   group: 'tasks' },
  email:      { label: 'Email',             icon: Mail,          color: 'text-orange-500',   group: 'external' },
  telegram:   { label: 'Telegram',          icon: Globe,         color: 'text-sky-500',      group: 'external' },
  slack:      { label: 'Slack',             icon: Hash,          color: 'text-pink-500',     group: 'external' },
  discord:    { label: 'Discord',           icon: Hash,          color: 'text-indigo-500',   group: 'external' },
  whatsapp:   { label: 'WhatsApp',          icon: Globe,         color: 'text-emerald-400',  group: 'external' },
  teams:      { label: 'Teams',             icon: Globe,         color: 'text-blue-400',     group: 'external' },
  webhook:    { label: 'Webhook',           icon: Zap,           color: 'text-muted-foreground', group: 'external' },
  test:       { label: 'Test',              icon: RotateCcw,     color: 'text-muted-foreground', group: 'tasks' },
};

function chanMeta(channel: string) {
  return CHANNEL_META[channel] ?? { label: channel, icon: Hash, color: 'text-muted-foreground', group: 'other' };
}

function sessionLabel(s: Session): string {
  if (s.label) return s.label;
  if (s.summary) return s.summary.slice(0, 60);
  const meta = chanMeta(s.channel);
  return meta.label;
}

function sessionTime(s: Session): string {
  return relativeTime(s.updated_at ?? s.created_at);
}

// Extract readable text from a message (handles both content and parts)
function msgText(msg: Message): string {
  if (msg.content) return msg.content;
  if (msg.parts && Array.isArray(msg.parts)) {
    return msg.parts
      .filter((p: any) => p.type === 'text')
      .map((p: any) => p.text ?? '')
      .join('\n') || '[tool use]';
  }
  return '';
}

// Tool use summary line for assistant messages that only have tool calls
function ToolSummary({ parts }: { parts: any[] }) {
  const calls = parts.filter((p: any) => p.type === 'tool-call');
  if (calls.length === 0) return null;
  return (
    <div className="mt-1.5 space-y-1">
      {calls.map((c: any, i: number) => (
        <div key={i} className="flex items-center gap-1.5 text-2xs text-amber-500/80">
          <Zap className="h-2.5 w-2.5 shrink-0" />
          <span className="font-mono">{c.toolName}({Object.keys(c.toolArgs ?? {}).join(', ')})</span>
        </div>
      ))}
    </div>
  );
}

// Message bubble in the thread view
function MsgBubble({ msg, agentName }: { msg: Message; agentName: string }) {
  const isAgent = msg.role === 'assistant';
  const isSystem = msg.role === 'system';

  if (isSystem) return null; // hide system prompt messages

  const text = msgText(msg);
  const preview = text.slice(0, 600) + (text.length > 600 ? '…' : '');
  const hasParts = msg.parts && Array.isArray(msg.parts);

  return (
    <div className={cn('mx-4 my-2 flex gap-2.5', isAgent ? 'flex-row' : 'flex-row-reverse')}>
      <div className={cn(
        'flex h-7 w-7 shrink-0 items-center justify-center rounded-full text-xs font-bold',
        isAgent ? 'bg-primary/15 text-primary' : 'bg-muted text-muted-foreground',
      )}>
        {isAgent ? <Cpu className="h-3.5 w-3.5" /> : <User className="h-3.5 w-3.5" />}
      </div>
      <div className={cn(
        'max-w-[80%] rounded-xl px-3.5 py-2.5 text-sm',
        isAgent
          ? 'bg-muted/50 text-foreground rounded-tl-sm'
          : 'bg-primary/10 text-foreground rounded-tr-sm',
      )}>
        <p className="text-2xs font-medium mb-1 text-muted-foreground">
          {isAgent ? agentName : (msg.sender_name ?? 'User')}
          {msg.timestamp !== undefined && (
            <span className="ml-1.5 opacity-60">
              {new Date(typeof msg.timestamp === 'number' ? msg.timestamp * 1000 : msg.timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}
            </span>
          )}
        </p>
        {preview && <p className="leading-relaxed whitespace-pre-wrap break-words">{preview}</p>}
        {hasParts && <ToolSummary parts={msg.parts as any[]} />}
      </div>
    </div>
  );
}

export function InboxTab({ agentId, agentName }: { agentId: string; agentName: string }) {
  const [allSessions, setAllSessions] = useState<Session[]>([]);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [messages, setMessages] = useState<Message[]>([]);
  const [loadingSessions, setLoadingSessions] = useState(true);
  const [loadingMsgs, setLoadingMsgs] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [activeGroup, setActiveGroup] = useState<string>('all');

  // Load all sessions for this agent
  useEffect(() => {
    setLoadingSessions(true);
    sessionsApi.listByAgent(agentId)
      .then((d) => {
        const list = (d.sessions ?? [])
          .filter((s: Session) => s.channel !== 'web') // web = direct user chat (already in Chat tab)
          .sort((a: Session, b: Session) =>
            new Date(b.updated_at ?? b.created_at).getTime() - new Date(a.updated_at ?? a.created_at).getTime()
          );
        setAllSessions(list);
        // Auto-select most recent
        if (list.length > 0 && list[0]) setSelectedId(list[0].id);
      })
      .catch((e) => setError(e instanceof Error ? e.message : 'Failed to load sessions'))
      .finally(() => setLoadingSessions(false));
  }, [agentId]);

  // Load messages when selection changes
  useEffect(() => {
    if (!selectedId) return;
    setLoadingMsgs(true);
    setMessages([]);
    sessionsApi.messages(selectedId, 200)
      .then((d) => setMessages(d.messages ?? []))
      .catch(() => setMessages([]))
      .finally(() => setLoadingMsgs(false));
  }, [selectedId]);

  // Group filter
  const groups = [
    { id: 'all',      label: 'All' },
    { id: 'tasks',    label: 'Tasks' },
    { id: 'external', label: 'External' },
  ];

  const filtered = activeGroup === 'all'
    ? allSessions
    : allSessions.filter((s) => chanMeta(s.channel).group === activeGroup);

  const selected = allSessions.find((s) => s.id === selectedId);

  if (loadingSessions) {
    return (
      <div className="flex h-full items-center justify-center">
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex h-full items-center justify-center gap-2 text-sm text-destructive">
        <AlertCircle className="h-4 w-4" />
        {error}
      </div>
    );
  }

  if (allSessions.length === 0) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-3 text-center p-6">
        <Bot className="h-10 w-10 text-muted-foreground/30" />
        <p className="text-sm font-medium text-muted-foreground">No activity yet</p>
        <p className="text-xs text-muted-foreground/60 max-w-xs">
          Scheduled tasks, delegated work, and external channel messages will appear here.
        </p>
      </div>
    );
  }

  return (
    <div className="flex h-full overflow-hidden">
      {/* Sidebar — session list */}
      <div className="w-64 shrink-0 border-r border-border flex flex-col overflow-hidden">
        {/* Group filter chips */}
        <div className="flex gap-1 p-2 border-b border-border shrink-0">
          {groups.map((g) => (
            <button
              key={g.id}
              onClick={() => setActiveGroup(g.id)}
              className={cn(
                'flex-1 rounded-md px-2 py-1 text-2xs font-medium transition-colors',
                activeGroup === g.id
                  ? 'bg-primary/15 text-primary'
                  : 'text-muted-foreground hover:bg-accent hover:text-foreground',
              )}
            >
              {g.label}
            </button>
          ))}
        </div>

        <div className="flex-1 overflow-y-auto">
          {filtered.length === 0 ? (
            <p className="p-4 text-2xs text-muted-foreground text-center">No sessions in this group</p>
          ) : (
            filtered.map((s) => {
              const meta = chanMeta(s.channel);
              const Icon = meta.icon;
              const isSelected = s.id === selectedId;
              return (
                <button
                  key={s.id}
                  onClick={() => setSelectedId(s.id)}
                  className={cn(
                    'w-full text-left px-3 py-2.5 border-b border-border/50 hover:bg-accent/50 transition-colors',
                    isSelected && 'bg-primary/10 border-l-2 border-l-primary',
                  )}
                >
                  <div className="flex items-center gap-2 mb-0.5">
                    <Icon className={cn('h-3 w-3 shrink-0', meta.color)} />
                    <span className="text-2xs font-medium text-muted-foreground truncate">{meta.label}</span>
                    <span className="ml-auto text-2xs text-muted-foreground/60 shrink-0">{sessionTime(s)}</span>
                  </div>
                  <p className="text-xs text-foreground truncate">{sessionLabel(s)}</p>
                  {((s.input_tokens ?? 0) + (s.output_tokens ?? 0) > 0) && (
                    <p className="text-2xs text-muted-foreground/50 mt-0.5">
                      {(((s.input_tokens ?? 0) + (s.output_tokens ?? 0)) / 1000).toFixed(1)}K tokens
                    </p>
                  )}
                </button>
              );
            })
          )}
        </div>
      </div>

      {/* Main area — messages */}
      <div className="flex-1 flex flex-col overflow-hidden">
        {selected ? (
          <>
            {/* Header */}
            <div className="shrink-0 flex items-center gap-3 px-4 py-2.5 border-b border-border">
              {(() => {
                const meta = chanMeta(selected.channel);
                const Icon = meta.icon;
                return <Icon className={cn('h-4 w-4 shrink-0', meta.color)} />;
              })()}
              <div className="flex-1 min-w-0">
                <p className="text-sm font-medium truncate">{sessionLabel(selected)}</p>
                <p className="text-2xs text-muted-foreground">
                  {chanMeta(selected.channel).label} · {sessionTime(selected)}
                  {(selected.input_tokens ?? 0) > 0 && ` · ${(((selected.input_tokens ?? 0) + (selected.output_tokens ?? 0)) / 1000).toFixed(1)}K tokens`}
                </p>
              </div>
            </div>

            {/* Messages */}
            <div className="flex-1 overflow-y-auto py-3">
              {loadingMsgs ? (
                <div className="flex justify-center py-8">
                  <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
                </div>
              ) : messages.length === 0 ? (
                <div className="flex flex-col items-center justify-center h-full gap-2 text-center">
                  <MessageSquare className="h-8 w-8 text-muted-foreground/20" />
                  <p className="text-xs text-muted-foreground">No messages recorded</p>
                </div>
              ) : (
                messages.map((msg, i) => (
                  <MsgBubble key={i} msg={msg} agentName={agentName} />
                ))
              )}
            </div>
          </>
        ) : (
          <div className="flex flex-1 items-center justify-center">
            <div className="text-center text-sm text-muted-foreground">
              <ChevronRight className="h-6 w-6 mx-auto mb-2 opacity-30" />
              Select a session
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
