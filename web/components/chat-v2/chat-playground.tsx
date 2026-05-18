'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useRef, useState, useCallback, useEffect } from 'react';
import { useChat } from '@ai-sdk/react';
import { DefaultChatTransport, isToolUIPart, getToolName, type UIMessage, type DynamicToolUIPart, type ReasoningUIPart, type TextUIPart } from 'ai';
import { Composer } from './composer';
import { ToolInvocation } from './tool-invocation';
import { CitationBar, type Source } from './citation-bar';
import { MessageBubble } from './message-bubble';
import { markdownComponents } from '@/components/chat/code-block';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { cn } from '@/lib/utils';
import { ChevronDown, ShieldAlert, Check, X, Loader2, Infinity, Clock, Timer } from 'lucide-react';
import { sessions as sessionsApi, permissions as permissionsApi, agents } from '@/lib/api';
import { request } from '@/lib/api-core';
import { useStore } from '@/store';
import type { Message as SessionMessage } from '@/types';

type ConversationMessage = {
  role: 'user' | 'assistant';
  content: string;
  session_id: string;
  discussion_id?: string;
  source_channel: string;
  ts?: string;
};

type Discussion = {
  id: string;
  ai_label: string;
  user_label?: string;
  last_active_at: string;
  message_count: number;
};

const CHANNEL_BADGES: Record<string, string> = {
  telegram: '[TG]',
  whatsapp: '[WA]',
  slack: '[SL]',
  email: '[✉]',
  tui: '[CLI]',
  web: '',
};

const getToken = () =>
  typeof window !== 'undefined' ? (localStorage.getItem('qorven_token') ?? '') : '';

interface ChatPlaygroundProps {
  agentId: string;
  sessionId: string;
  className?: string;
  systemContext?: string;
  agentName?: string;
  initialThinkingLevel?: 'off' | 'medium' | 'high';
}

interface WidgetData {
  type: string;
  data: Record<string, unknown>;
}

function sessionMessageToUIMessage(msg: SessionMessage, idx: number): UIMessage {
  const id = `hist-${idx}`;
  const metadata: Record<string, unknown> = {};
  if (msg.timestamp) metadata.timestamp = msg.timestamp;
  if (msg.channel) metadata.source_channel = msg.channel;

  if (msg.role === 'user') {
    return { id, role: 'user', parts: [{ type: 'text', text: msg.content }], metadata } as unknown as UIMessage;
  }
  // Build parts from stored parts array (may include text, reasoning, widget, tool-call, tool-result)
  const parts: UIMessage['parts'] = [];
  const rawParts = (msg.parts ?? []) as Array<{ type: string; content?: string; widgetType?: string; widgetData?: unknown; [k: string]: unknown }>;
  if (rawParts.length > 0) {
    for (const p of rawParts) {
      if (p.type === 'text') {
        parts.push({ type: 'text', text: (p.content ?? '') as string });
      } else if (p.type === 'reasoning') {
        parts.push({ type: 'reasoning', text: (p.content ?? '') as string, providerMetadata: {} });
      } else {
        // widget, tool-call, tool-result etc — pass through as unknown for MessageBubble rendering
        parts.push(p as unknown as UIMessage['parts'][0]);
      }
    }
  } else if (msg.content) {
    parts.push({ type: 'text', text: msg.content });
  }
  return { id, role: 'assistant', parts, metadata } as unknown as UIMessage;
}

export function ChatPlayground({ agentId, sessionId, className, systemContext, agentName, initialThinkingLevel = 'off' }: ChatPlaygroundProps) {
  const [streamSources, setStreamSources] = useState<Source[]>([]);
  const [followUps, setFollowUps] = useState<string[]>([]);
  const [streamWidgets, setStreamWidgets] = useState<WidgetData[]>([]);
  const [inputValue, setInputValue] = useState('');
  const [thinkingLevel, setThinkingLevelState] = useState<'off' | 'medium' | 'high'>(initialThinkingLevel);
  const setThinkingLevel = useCallback((level: 'off' | 'medium' | 'high') => {
    setThinkingLevelState(level);
    // Persist to agent record so it applies across all channels and sessions
    agents.update(agentId, { thinking_level: level } as never).catch(() => {});
  }, [agentId]);
  // Tracks send-time timestamps for live useChat messages (SDK doesn't stamp them)
  const msgTimestamps = useRef<Map<string, number>>(new Map());
  // Flips to true the moment main response text arrives; used to hide the stop
  // button early — background tasks (title, follow-ups) keep the SSE stream
  // open for a few extra seconds after the visible reply is complete.
  const responseTextReceivedRef = useRef(false);
  const [, forceRender] = useState(0);
  const [showScrollBtn, setShowScrollBtn] = useState(false);
  const [initialMessages, setInitialMessages] = useState<UIMessage[]>([]);
  const [historyLoaded, setHistoryLoaded] = useState(false);
  const [decidingId, setDecidingId] = useState<string | null>(null);
  // Shell-security approvals arriving inline via SSE (exec tool AskUser path)
  const [shellApprovals, setShellApprovals] = useState<Array<{
    approvalId: string; tool: string; command: string; reason: string;
  }>>([]);
  const [shellDecidingId, setShellDecidingId] = useState<string | null>(null);
  const [discussions, setDiscussions] = useState<Discussion[]>([]);
  const [historyMsgs, setHistoryMsgs] = useState<ConversationMessage[]>([]);
  const [historyLoaded2, setHistoryLoaded2] = useState(false);

  // Pending permission-gate approvals for this session — sourced from the
  // WS-populated store so the banner appears the instant the agent pauses.
  const storeApprovals = useStore((s) => s.approvals);
  const markApprovalResolved = useStore((s) => s.markApprovalResolved);
  const incomingMessages = useStore((s) => s.incomingMessages);
  const clearIncomingMessages = useStore((s) => s.clearIncomingMessages);
  const pendingApprovals = Object.values(storeApprovals).filter(
    (e) => e.resolved === null && e.request.session_id === sessionId,
  );

  const toolLabel = (tool: string) => {
    const labels: Record<string, string> = {
      exec:           'Shell command execution',
      write_file:     'Write file to disk',
      read_file:      'Read file from disk',
      apply_patch:    'Apply patch to files',
      delete_file:    'Delete a file',
      cron:           'Schedule a recurring task',
      gh_push_file:   'Push file to GitHub',
      gh_open_pr:     'Open a GitHub pull request',
      gh_merge_pr:    'Merge a GitHub pull request',
      gh_create_repo: 'Create a GitHub repository',
      web_search:     'Web search',
      web_fetch:      'Fetch a web page',
      memory_search:  'Search agent memory',
      undo:           'Undo / revert changes',
    };
    return labels[tool] ?? tool.replace(/_/g, ' ');
  };

  const handleDecide = useCallback(async (requestId: string, decision: 'allow' | 'allow_always' | 'allow_session' | 'allow_1h' | 'deny') => {
    setDecidingId(requestId);
    try {
      await permissionsApi.reply(requestId, { decision });
    } catch {
      // Optimistically resolve — WS permission.replied will confirm
    } finally {
      markApprovalResolved(requestId, decision === 'deny' ? 'deny' : 'allow');
      setDecidingId(null);
    }
  }, [markApprovalResolved]);

  const handleShellDecide = useCallback(async (approvalId: string, decision: 'approve' | 'reject') => {
    setShellDecidingId(approvalId);
    try {
      await request<void>(`/approvals/${approvalId}/decide`, {
        method: 'POST',
        body: JSON.stringify({ decision }),
      });
    } catch {
      // best-effort; the loop polls the DB
    } finally {
      setShellApprovals((prev) => prev.filter((a) => a.approvalId !== approvalId));
      setShellDecidingId(null);
    }
  }, []);

  const scrollContainerRef = useRef<HTMLDivElement>(null);
  const userScrolledUp = useRef(false);
  const bottomRef = useRef<HTMLDivElement>(null);
  const topSentinelRef = useRef<HTMLDivElement>(null);
  const [totalMessages, setTotalMessages] = useState(0);
  const [loadingOlder, setLoadingOlder] = useState(false);
  const loadedOffsetRef = useRef(0); // how many we've loaded from the end

  const PAGE_SIZE = 50;

  useEffect(() => {
    if (!sessionId) { setHistoryLoaded(true); return; }
    let active = true;
    sessionsApi.messages(sessionId, PAGE_SIZE, 0)
      .then(({ messages: msgs, total }) => {
        if (!active) return;
        setInitialMessages(msgs.map(sessionMessageToUIMessage));
        setTotalMessages(total);
        loadedOffsetRef.current = msgs.length;
        setHistoryLoaded(true);
      })
      .catch(() => { if (active) setHistoryLoaded(true); });
    return () => { active = false; };
  }, [sessionId]);

  useEffect(() => {
    if (!agentId) return;
    let active = true;
    Promise.all([
      request<{ discussions: Discussion[] }>(`/agents/${agentId}/discussions`),
      request<{ messages: ConversationMessage[] }>(`/agents/${agentId}/messages?limit=100`),
    ]).then(([discData, msgData]) => {
      if (!active) return;
      setDiscussions(discData.discussions ?? []);
      // API returns newest-first; reverse for chronological display.
      // Exclude messages from the current session — those are already shown
      // via sessionsApi.messages() in initialMessages, so keeping them here
      // would render every message twice (once with [TG] badge, once without).
      setHistoryMsgs((msgData.messages ?? []).filter((m) => m.session_id !== sessionId).reverse());
      setHistoryLoaded2(true);
    }).catch(() => {
      if (active) setHistoryLoaded2(true);
    });
    return () => { active = false; };
  }, [agentId]);

  // Realtime: append WS-pushed messages for this session into the displayed list.
  // This is what makes Telegram (and any other channel) messages appear instantly
  // without a page refresh — the WS new_message event lands in the store and we
  // convert it to a UIMessage here.
  useEffect(() => {
    const mine = incomingMessages.filter((m) => m.sessionId === sessionId);
    if (mine.length === 0) return;
    clearIncomingMessages(sessionId);
    setInitialMessages((prev) => {
      const existingIds = new Set(prev.map((m) => m.id));
      const newMsgs: UIMessage[] = mine
        .filter((m) => {
          // Skip user messages from web/webchat — useChat already rendered them locally.
          // Keep user messages from external channels (telegram, whatsapp, etc.) — they
          // arrive via WS and would never appear otherwise.
          if (m.role === 'user') {
            const ch = m.source ?? '';
            return ch !== '' && ch !== 'web' && ch !== 'webchat';
          }
          return true;
        })
        .map((m, i): UIMessage => ({
          id: `ws-${Date.now()}-${i}`,
          role: m.role as 'user' | 'assistant',
          parts: [{ type: 'text', text: m.content }],
          metadata: { source_channel: m.source ?? '', timestamp: Date.now() },
        } as unknown as UIMessage))
        .filter((m) => !existingIds.has(m.id));
      return newMsgs.length > 0 ? [...prev, ...newMsgs] : prev;
    });
  }, [incomingMessages, sessionId, clearIncomingMessages]);

  // Keep a ref to the latest values so the transport callbacks always read
  // current props/state without recreating the transport object on every render.
  // A new DefaultChatTransport every render destabilizes useChat's internal
  // useSyncExternalStore subscribe reference → React error #185.
  const transportParamsRef = useRef({ agentId, sessionId, systemContext, thinkingLevel });
  transportParamsRef.current = { agentId, sessionId, systemContext, thinkingLevel };

  const transportRef = useRef<DefaultChatTransport<UIMessage> | null>(null);
  if (!transportRef.current) {
    transportRef.current = new DefaultChatTransport({
      api: '/api/chat',
      headers: () => ({ Authorization: `Bearer ${getToken()}` }),
      body: () => {
        const { agentId: aid, sessionId: sid, systemContext: sc, thinkingLevel: tl } = transportParamsRef.current;
        return {
          agentId: aid,
          sessionId: sid,
          ...(sc ? { systemContext: sc } : {}),
          ...(tl !== 'off' ? { thinkingLevel: tl } : {}),
        };
      },
    });
  }

  // Do NOT pass initialMessages to useChat — @ai-sdk/react v3 reassigns its
  // internal chatRef.current when the messages prop changes, destabilizing the
  // useSyncExternalStore subscribe reference and causing React error #185.
  // Instead we render history manually and let useChat manage only new messages.
  const { messages: chatMessages, sendMessage, status, stop, regenerate } = useChat({
    transport: transportRef.current,
    onData(dataPart) {
      const p = dataPart as { type?: string; data?: unknown };
      if (p.type === 'data-sources' && Array.isArray(p.data)) {
        setStreamSources(p.data as Source[]);
      }
      if (p.type === 'data-follow_ups' && Array.isArray(p.data)) {
        setFollowUps(p.data as string[]);
      }
      if (p.type === 'data-widget' && p.data && typeof p.data === 'object') {
        const w = p.data as { type?: string; data?: Record<string, unknown> };
        if (w.type) {
          setStreamWidgets((prev) => [...prev, { type: w.type!, data: w.data ?? {} }]);
        }
      }
      // Shell-security approval gate — emitted by exec tool's AskUser path
      if (p.type === 'tool_approval' && p.data && typeof p.data === 'object') {
        const d = p.data as { approval_id?: string; tool?: string; command?: string; reason?: string };
        if (d.approval_id) {
          setShellApprovals((prev) => {
            if (prev.some((a) => a.approvalId === d.approval_id)) return prev;
            return [...prev, {
              approvalId: d.approval_id!,
              tool: d.tool ?? 'exec',
              command: d.command ?? '',
              reason: d.reason ?? 'command not in allowlist',
            }];
          });
        }
      }
    },
    onFinish(result) {
      setStreamSources([]);
      setStreamWidgets([]);
      setShellApprovals([]);
      // Stamp the completed assistant message with current time, then re-render to show it
      const id = (result as { responseMessage?: { id?: string } })?.responseMessage?.id;
      if (id) {
        msgTimestamps.current.set(id, Date.now());
        forceRender((n) => n + 1);
      }
    },
  });

  // Combine history (loaded once) with live chat messages from useChat
  const messages = [...initialMessages, ...chatMessages];

  const isStreaming = status === 'streaming' || status === 'submitted';

  // Get parts from the last assistant message (which is in-progress during streaming)
  const lastMsg = chatMessages[chatMessages.length - 1];
  const lastParts = (isStreaming && lastMsg?.role === 'assistant') ? (lastMsg.parts ?? []) : [];

  // Track whether visible response text has arrived so the stop button can disappear
  // as soon as the reply is readable, even if background tasks (title, follow-ups)
  // are still keeping the SSE stream alive.
  if (isStreaming && lastMsg?.role === 'assistant') {
    const hasText = (lastMsg.parts ?? []).some(
      (p) => p.type === 'text' && (p as { type: 'text'; text: string }).text.length > 0,
    );
    if (hasText) responseTextReceivedRef.current = true;
  }
  const showStopButton = isStreaming && !responseTextReceivedRef.current;

  // Load older messages when user scrolls to the top
  const loadOlderMessages = useCallback(async () => {
    if (loadingOlder || loadedOffsetRef.current >= totalMessages) return;
    setLoadingOlder(true);
    const el = scrollContainerRef.current;
    const prevHeight = el?.scrollHeight ?? 0;
    try {
      const nextOffset = loadedOffsetRef.current;
      const { messages: older, total } = await sessionsApi.messages(sessionId, PAGE_SIZE, nextOffset);
      if (older.length === 0) return;
      setTotalMessages(total);
      loadedOffsetRef.current += older.length;
      setInitialMessages((prev) => [...older.map(sessionMessageToUIMessage), ...prev]);
      // Maintain scroll position — jump to same relative position after prepend
      requestAnimationFrame(() => {
        if (el) el.scrollTop = el.scrollHeight - prevHeight;
      });
    } finally {
      setLoadingOlder(false);
    }
  }, [loadingOlder, totalMessages, sessionId]);

  // Auto-scroll
  const handleScroll = () => {
    const el = scrollContainerRef.current;
    if (!el) return;
    const isAtBottom = el.scrollTop + el.clientHeight >= el.scrollHeight - 80;
    userScrolledUp.current = !isAtBottom;
    setShowScrollBtn(!isAtBottom);
    // Load older messages when near the top
    if (el.scrollTop < 80) loadOlderMessages();
  };

  const scrollToBottom = useCallback(() => {
    userScrolledUp.current = false;
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, []);

  // Scroll to bottom when history first loads
  useEffect(() => {
    if (historyLoaded) {
      bottomRef.current?.scrollIntoView({ behavior: 'instant' });
    }
  }, [historyLoaded]);

  // Auto-scroll on new streaming messages
  useEffect(() => {
    if (!userScrolledUp.current) {
      bottomRef.current?.scrollIntoView({ behavior: 'smooth' });
    }
  }, [chatMessages, isStreaming]);

  const pendingSendTs = useRef<number>(0);

  const handleComposerSubmit = useCallback((attachments?: Array<{ name: string; type: string; url: string; size: number }>) => {
    if (!inputValue.trim() && !attachments?.length) return;
    setFollowUps([]);
    setStreamSources([]);
    setStreamWidgets([]);
    userScrolledUp.current = false;
    pendingSendTs.current = Date.now();
    responseTextReceivedRef.current = false;

    if (attachments?.length) {
      const files = attachments.map((a) => ({
        type: 'file' as const,
        mediaType: a.type || 'application/octet-stream',
        filename: a.name,
        url: a.url,
      }));
      sendMessage({ text: inputValue.trim(), files });
    } else {
      sendMessage({ text: inputValue.trim() });
    }
    setInputValue('');
  }, [inputValue, sendMessage]);

  // Stamp the newly added user message with the send time
  useEffect(() => {
    if (pendingSendTs.current === 0) return;
    const lastMsg = chatMessages[chatMessages.length - 1];
    if (lastMsg?.role === 'user' && !msgTimestamps.current.has(lastMsg.id)) {
      msgTimestamps.current.set(lastMsg.id, pendingSendTs.current);
      pendingSendTs.current = 0;
      forceRender((n) => n + 1);
    }
  }, [chatMessages]);

  if (!historyLoaded) {
    return (
      <div className={cn('flex flex-col relative items-center justify-center', className)}>
        <div className="flex gap-1">
          {[0, 1, 2].map((i) => (
            <span key={i} className="h-1.5 w-1.5 rounded-full bg-muted-foreground/40 animate-bounce" style={{ animationDelay: `${i * 150}ms` }} />
          ))}
        </div>
      </div>
    );
  }

  return (
    <div className={cn('flex flex-col relative', className)}>
      {/* Message area */}
      <div
        ref={scrollContainerRef}
        onScroll={handleScroll}
        className="flex-1 overflow-y-auto"
      >
        <div className="mx-auto max-w-3xl px-4 py-6 space-y-6">
          {/* Load-older indicator */}
          {loadedOffsetRef.current < totalMessages && (
            <div className="flex justify-center">
              {loadingOlder
                ? <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
                : <button onClick={loadOlderMessages} className="text-xs text-muted-foreground hover:text-foreground transition-colors">Load earlier messages</button>
              }
            </div>
          )}

          {messages.length === 0 && !isStreaming && <EmptyState />}

          {/* Historical discussion context */}
          {historyLoaded2 && historyMsgs.length > 0 && (
            <div className="pb-4">
              {historyMsgs.map((msg, i) => {
                const prevMsg = historyMsgs[i - 1];
                const showDivider = msg.discussion_id && msg.discussion_id !== prevMsg?.discussion_id;
                const disc = discussions.find(d => d.id === msg.discussion_id);
                const badge = CHANNEL_BADGES[msg.source_channel] ?? '';

                return (
                  <div key={`hist-${i}`}>
                    {showDivider && disc && (
                      <div className="flex items-center gap-3 my-3 px-4">
                        <div className="flex-1 h-px bg-border/30" />
                        <span className="text-xs text-muted-foreground/50 px-2">
                          {disc.user_label ?? disc.ai_label}
                        </span>
                        <div className="flex-1 h-px bg-border/30" />
                      </div>
                    )}
                    <div className={cn(
                      'flex gap-2 px-4 py-1',
                      msg.role === 'user' ? 'flex-row-reverse' : ''
                    )}>
                      <div className={cn(
                        'max-w-[75%] rounded-xl px-3 py-2 text-sm leading-relaxed opacity-70',
                        msg.role === 'user'
                          ? 'bg-primary/80 text-primary-foreground rounded-tr-sm'
                          : 'bg-muted/80 rounded-tl-sm'
                      )}>
                        {badge && <span className="mr-1 text-xs opacity-60">{badge}</span>}
                        {msg.content}
                      </div>
                    </div>
                  </div>
                );
              })}
              {/* Divider between history and current session */}
              <div className="flex items-center gap-3 my-4 px-4">
                <div className="flex-1 h-px bg-border/50" />
                <span className="text-xs text-muted-foreground/40 px-2">Current session</span>
                <div className="flex-1 h-px bg-border/50" />
              </div>
            </div>
          )}

          {messages.map((msg, idx) => {
            // The last assistant message is rendered live during streaming
            const isLastLive = isStreaming && idx === messages.length - 1 && msg.role === 'assistant';
            if (isLastLive) return null;
            // Merge in client-side timestamp for live messages that SDK doesn't stamp
            const liveTs = msgTimestamps.current.get(msg.id);
            const msgWithTs = liveTs
              ? { ...msg, metadata: { ...(msg.metadata as object ?? {}), timestamp: liveTs } }
              : msg;
            return (
              <MessageBubble
                key={msg.id}
                message={msgWithTs as UIMessage & { timestamp?: string; model?: string }}
                isStreaming={false}
                onRegenerate={msg.role === 'assistant' ? () => regenerate() : undefined}
                agentName={agentName}
                agentId={agentId}
              />
            );
          })}

          {/* Live streaming message */}
          {isStreaming && (
            <LiveMessage parts={lastParts} sources={streamSources} widgets={streamWidgets} agentName={agentName} />
          )}

          {/* Follow-up suggestions */}
          {!isStreaming && followUps.length > 0 && (
            <div className="flex flex-wrap gap-2">
              {followUps.map((q, i) => (
                <button
                  key={i}
                  onClick={() => { setInputValue(q); setFollowUps([]); }}
                  className="rounded-full border border-border bg-card px-3 py-1.5 text-xs text-muted-foreground hover:border-primary/30 hover:text-foreground transition-colors"
                >
                  {q}
                </button>
              ))}
            </div>
          )}

          <div ref={bottomRef} />
        </div>
      </div>

      {/* Scroll to bottom button */}
      {showScrollBtn && (
        <button
          onClick={scrollToBottom}
          className="absolute bottom-24 right-6 z-10 flex h-8 w-8 items-center justify-center rounded-full bg-card border border-border shadow-md text-muted-foreground hover:text-foreground transition-colors"
        >
          <ChevronDown className="h-4 w-4" />
        </button>
      )}

      {/* Shell-security approval banners (exec tool AskUser path) */}
      {shellApprovals.map((sa) => {
        const isDeciding = shellDecidingId === sa.approvalId;
        return (
          <div key={sa.approvalId} className="mx-4 mb-2 flex items-start gap-3 rounded-xl border border-amber-500/30 bg-amber-500/5 px-4 py-3 text-sm">
            <ShieldAlert className="mt-0.5 h-4 w-4 shrink-0 text-amber-500" />
            <div className="flex-1 min-w-0">
              <p className="font-medium text-foreground">
                {toolLabel(sa.tool)}
                <code className="ml-1.5 rounded bg-muted px-1 py-0.5 text-2xs text-muted-foreground font-mono">{sa.tool}</code>
              </p>
              {sa.command && (
                <pre className="mt-1.5 max-h-20 overflow-auto rounded bg-muted p-2 text-2xs text-muted-foreground whitespace-pre-wrap break-all">{sa.command}</pre>
              )}
            </div>
            <div className="flex items-center gap-1.5 shrink-0">
              <button disabled={isDeciding} onClick={() => handleShellDecide(sa.approvalId, 'approve')}
                className="flex items-center gap-1 rounded-lg bg-emerald-500/10 px-2.5 py-1 text-xs font-medium text-emerald-600 hover:bg-emerald-500/20 disabled:opacity-50 transition-colors">
                {isDeciding ? <Loader2 className="h-3 w-3 animate-spin" /> : <Check className="h-3 w-3" />}
                Allow
              </button>
              <button disabled={isDeciding} onClick={() => handleShellDecide(sa.approvalId, 'reject')}
                className="flex items-center gap-1 rounded-lg bg-destructive/10 px-2.5 py-1 text-xs font-medium text-destructive hover:bg-destructive/20 disabled:opacity-50 transition-colors">
                <X className="h-3 w-3" />
                Deny
              </button>
            </div>
          </div>
        );
      })}

      {/* Pending approval banners — one per gate, stacked, dismiss only via Allow/Deny */}
      {pendingApprovals.map((entry) => {
        const req = entry.request;
        const isDeciding = decidingId === req.request_id;
        const isSensitive = ['exec', 'gh_push_file', 'gh_open_pr', 'gh_merge_pr', 'gh_create_repo'].includes(req.tool);
        return (
          <div
            key={req.request_id}
            className="mx-4 mb-2 flex items-start gap-3 rounded-xl border border-amber-500/30 bg-amber-500/5 px-4 py-3 text-sm"
          >
            <ShieldAlert className="mt-0.5 h-4 w-4 shrink-0 text-amber-500" />
            <div className="flex-1 min-w-0">
              <p className="font-medium text-foreground">
                {toolLabel(req.tool)}
                <code className="ml-1.5 rounded bg-muted px-1 py-0.5 text-2xs text-muted-foreground font-mono">{req.tool}</code>
              </p>
              {req.reason && req.reason !== 'command not in allowlist' && (
                <p className="mt-0.5 text-xs text-muted-foreground">{req.reason}</p>
              )}
              {Object.keys(req.args ?? {}).length > 0 && (
                <pre className="mt-1.5 max-h-20 overflow-auto rounded bg-muted p-2 text-2xs text-muted-foreground whitespace-pre-wrap break-all">
                  {JSON.stringify(req.args, null, 2)}
                </pre>
              )}
            </div>
            <div className="flex flex-wrap items-center gap-1.5 shrink-0 max-w-[200px] justify-end">
              {/* Allow once — always shown */}
              <button
                disabled={isDeciding}
                onClick={() => handleDecide(req.request_id, 'allow')}
                className="flex items-center gap-1 rounded-lg bg-emerald-500/10 px-2.5 py-1 text-xs font-medium text-emerald-600 hover:bg-emerald-500/20 disabled:opacity-50 transition-colors"
              >
                {isDeciding ? <Loader2 className="h-3 w-3 animate-spin" /> : <Check className="h-3 w-3" />}
                Once
              </button>
              {/* Sensitive tools get session + 1h options */}
              {isSensitive && (
                <>
                  <button
                    disabled={isDeciding}
                    onClick={() => handleDecide(req.request_id, 'allow_session')}
                    title="Allow for this session — won't ask again until you start a new chat"
                    className="flex items-center gap-1 rounded-lg bg-blue-500/10 px-2.5 py-1 text-xs font-medium text-blue-500 hover:bg-blue-500/20 disabled:opacity-50 transition-colors"
                  >
                    <Timer className="h-3 w-3" />
                    Session
                  </button>
                  <button
                    disabled={isDeciding}
                    onClick={() => handleDecide(req.request_id, 'allow_1h')}
                    title="Allow for the next 1 hour"
                    className="flex items-center gap-1 rounded-lg bg-violet-500/10 px-2.5 py-1 text-xs font-medium text-violet-500 hover:bg-violet-500/20 disabled:opacity-50 transition-colors"
                  >
                    <Clock className="h-3 w-3" />
                    1 hour
                  </button>
                </>
              )}
              {/* Always allow — both tiers */}
              <button
                disabled={isDeciding}
                onClick={() => handleDecide(req.request_id, 'allow_always')}
                title="Always allow — no more prompts for this tool"
                className="flex items-center gap-1 rounded-lg bg-slate-500/10 px-2.5 py-1 text-xs font-medium text-slate-500 hover:bg-slate-500/20 disabled:opacity-50 transition-colors"
              >
                <Infinity className="h-3 w-3" />
                Always
              </button>
              <button
                disabled={isDeciding}
                onClick={() => handleDecide(req.request_id, 'deny')}
                className="flex items-center gap-1 rounded-lg bg-destructive/10 px-2.5 py-1 text-xs font-medium text-destructive hover:bg-destructive/20 disabled:opacity-50 transition-colors"
              >
                <X className="h-3 w-3" />
                Deny
              </button>
            </div>
          </div>
        );
      })}

      {/* Composer */}
      <Composer
        input={inputValue}
        isLoading={showStopButton}
        onInputChange={setInputValue}
        onSubmit={handleComposerSubmit}
        onStop={stop}
        agentId={agentId}
        placeholder="Message…"
        thinkingLevel={thinkingLevel}
        onThinkingLevelChange={setThinkingLevel}
      />
    </div>
  );
}

function LiveMessage({
  parts,
  sources,
  widgets,
  agentName,
}: {
  parts: UIMessage['parts'];
  sources: Source[];
  widgets: WidgetData[];
  agentName?: string;
}) {
  const textContent = parts
    .filter((p): p is TextUIPart => p.type === 'text')
    .map((p) => p.text)
    .join('');

  const reasoningContent = parts
    .filter((p): p is ReasoningUIPart => p.type === 'reasoning')
    .map((p) => p.text)
    .join('');

  const toolParts = parts.filter((p): p is DynamicToolUIPart => p.type === 'dynamic-tool');

  const hasContent = textContent || reasoningContent || toolParts.length > 0;

  return (
    <div className="group flex gap-3">
      <div className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-primary/15 text-primary text-xs font-bold mt-0.5">
        {(agentName ?? 'Q').charAt(0).toUpperCase()}
      </div>
      <div className="flex-1 min-w-0 space-y-1">
        {toolParts.map((t) => (
          <ToolInvocation
            key={t.toolCallId}
            toolCallId={t.toolCallId}
            toolName={t.toolName}
            input={t.state !== 'input-streaming' ? (t as DynamicToolUIPart & { input: unknown }).input : undefined}
            output={'output' in t ? t.output : undefined}
            state={(t.state === 'output-available' || t.state === 'output-error' || ('output' in t && t.output !== undefined)) ? 'result' : 'calling'}
          />
        ))}

        {reasoningContent && (
          <div className="rounded-xl border border-border/40 bg-card/30 px-3 py-2 text-xs text-muted-foreground/70 font-mono whitespace-pre-wrap">
            <span className="text-amber-400 mr-1">◆</span>{reasoningContent}
          </div>
        )}

        {textContent && (
          <div className="prose prose-sm max-w-none text-foreground leading-relaxed font-normal [&_p]:text-foreground [&_p]:font-normal [&_p]:text-sm [&_li]:text-foreground [&_li]:font-normal [&_li]:text-sm [&_strong]:text-foreground [&_strong]:font-semibold [&_h1]:text-foreground [&_h2]:text-foreground [&_h3]:text-foreground [&_h4]:text-foreground [&_h1]:font-semibold [&_h2]:font-semibold [&_h3]:font-semibold [&_h4]:font-medium [&_h1]:text-base [&_h2]:text-sm [&_h3]:text-sm [&_blockquote]:text-muted-foreground [&_a]:text-primary [&_code]:text-foreground">
            <ReactMarkdown remarkPlugins={[remarkGfm]} components={markdownComponents as Parameters<typeof ReactMarkdown>[0]['components']}>
              {textContent}
            </ReactMarkdown>
            <span className="inline-block w-0.5 h-4 bg-primary/70 animate-pulse ml-0.5 align-middle" />
          </div>
        )}

        {!hasContent && (
          <div className="flex items-center gap-1.5 text-sm text-muted-foreground">
            {[0, 1, 2].map((i) => (
              <span
                key={i}
                className="h-1.5 w-1.5 rounded-full bg-muted-foreground/50 animate-bounce"
                style={{ animationDelay: `${i * 150}ms` }}
              />
            ))}
          </div>
        )}

        {widgets.length > 0 && (
          <div className="space-y-2">
            {widgets.map((w, i) => <StreamWidget key={i} widget={w} />)}
          </div>
        )}

        {sources.length > 0 && <CitationBar sources={sources} />}
      </div>
    </div>
  );
}

function StreamWidget({ widget }: { widget: WidgetData }) {
  if (widget.type === 'audio') {
    const src = widget.data.src as string | undefined;
    if (!src) return null;
    return (
      <div className="rounded-xl border border-border bg-card/50 p-3 max-w-sm">
        <p className="text-xs text-muted-foreground mb-2">🔊 Audio</p>
        <audio controls src={src} className="w-full h-8" />
      </div>
    );
  }
  if (widget.type === 'image') {
    const src = widget.data.url as string | undefined;
    if (!src) return null;
    return <img src={src} alt={(widget.data.prompt as string) ?? 'generated image'} className="max-w-sm rounded-xl border border-border" />;
  }
  return null;
}

function EmptyState() {
  return (
    <div className="flex flex-col items-center justify-center py-24 text-center">
      <div className="flex h-14 w-14 items-center justify-center rounded-2xl bg-primary/10 text-primary text-2xl mb-4">
        Q
      </div>
      <h2 className="text-lg font-semibold">How can I help you?</h2>
      <p className="mt-1 text-sm text-muted-foreground max-w-xs">
        Ask me anything — I can search the web, write code, run tasks, and more.
      </p>
    </div>
  );
}
