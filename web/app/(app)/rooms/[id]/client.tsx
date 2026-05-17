'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useEffect, useState, useRef, useCallback } from 'react';
import { useParams } from 'next/navigation';
import Link from 'next/link';
import { rooms as roomsApi } from '@/lib/api';
import { useStore } from '@/store';
import {
  ArrowLeft, Send, Pin, CheckSquare, FileText, Users, MessageSquare,
  Loader2, Sparkles, Circle, Crown, ShieldCheck, Clock, UserPlus, UserMinus,
  RefreshCw,
} from 'lucide-react';
import { cn } from '@/lib/utils';

// ─── Types ────────────────────────────────────────────────────────────────────

interface Member {
  id: string;
  agent_key: string;
  display_name: string;
  role?: string;
  agent_role?: string;
  room_role?: string;
  speaking_order?: number;
  can_decide?: boolean;
  last_active_at?: string;
  avatar?: string;
}

interface RoomMsg {
  id: string;
  sender_id: string;
  sender_type: string;
  content: string;
  message_type?: string;
  created_at: string;
}

interface Decision {
  id: string;
  content: string;
  decided_by: string;
  status: string;
  created_at: string;
}

interface Task {
  id: string;
  title: string;
  description?: string;
  assigned_by: string;
  assigned_to: string;
  status: string;
  due_at?: string;
  created_at: string;
}

interface Minutes {
  id: string;
  summary: string;
  decisions: Array<{ text: string; decided_by: string }>;
  action_items: Array<{ task: string; assigned_to: string; due?: string }>;
  blockers: Array<{ issue: string; raised_by: string }>;
  msg_count: number;
  generated_at: string;
}

// ─── Message bubble ───────────────────────────────────────────────────────────

function ChatBubble({ msg, getMemberName, getMemberInitial }: {
  msg: RoomMsg;
  getMemberName: (id: string) => string;
  getMemberInitial: (id: string) => string;
}) {
  const isUser = msg.sender_type === 'user';
  const isSystem = msg.sender_id === 'system';
  const name = isUser ? 'You' : getMemberName(msg.sender_id);
  const initial = isUser ? 'U' : getMemberInitial(msg.sender_id);

  const msgTypeStyle = {
    decision: 'border-l-4 border-amber-500 bg-amber-500/10',
    task: 'border-l-4 border-blue-500 bg-blue-500/10',
    summary: 'border-l-4 border-violet-500 bg-violet-500/10',
    alert: 'border-l-4 border-red-500 bg-red-500/10',
  }[msg.message_type || ''] ?? '';

  if (isSystem) {
    return (
      <div className="text-center py-1">
        <span className={cn('inline-block rounded-lg px-3 py-1.5 text-xs text-muted-foreground bg-muted/60', msgTypeStyle)}>
          {msg.content}
        </span>
      </div>
    );
  }

  return (
    <div className={cn('flex gap-2', isUser && 'flex-row-reverse')}>
      <div className={cn(
        'h-7 w-7 shrink-0 rounded-full flex items-center justify-center text-xs font-semibold',
        isUser ? 'bg-primary text-primary-foreground' : 'bg-gradient-to-br from-primary to-primary/60 text-white'
      )}>
        {initial}
      </div>
      <div className="max-w-[75%]">
        {!isUser && <p className="text-xs font-medium text-muted-foreground mb-0.5">{name}</p>}
        <div className={cn(
          'rounded-xl px-3 py-2 text-sm whitespace-pre-wrap break-words',
          isUser ? 'rounded-tr-sm bg-primary text-primary-foreground' : 'rounded-tl-sm bg-muted',
          msgTypeStyle && !isUser && msgTypeStyle
        )}>
          {msg.content}
        </div>
        <p className="text-xs text-muted-foreground/50 mt-0.5">
          {msg.created_at ? new Date(msg.created_at).toLocaleTimeString() : ''}
        </p>
      </div>
    </div>
  );
}

// ─── Main page ────────────────────────────────────────────────────────────────

type Tab = 'chat' | 'decisions' | 'tasks' | 'minutes' | 'org';

const TABS: { id: Tab; label: string; icon: React.ElementType }[] = [
  { id: 'chat', label: 'Chat', icon: MessageSquare },
  { id: 'decisions', label: 'Decisions', icon: Pin },
  { id: 'tasks', label: 'Tasks', icon: CheckSquare },
  { id: 'minutes', label: 'Minutes', icon: FileText },
  { id: 'org', label: 'Org', icon: Users },
];

export function RoomDetail({ roomId, showBack = true }: { roomId: string; showBack?: boolean }) {
  const souls = useStore((s) => s.souls);
  const [room, setRoom] = useState<{ id: string; name: string; display_name: string; description: string; members: Member[] } | null>(null);
  const [messages, setMessages] = useState<RoomMsg[]>([]);
  const [decisions, setDecisions] = useState<Decision[]>([]);
  const [tasks, setTasks] = useState<Task[]>([]);
  const [minutesList, setMinutesList] = useState<Minutes[]>([]);
  const [orgMembers, setOrgMembers] = useState<Member[]>([]);
  const [tab, setTab] = useState<Tab>('chat');
  const [input, setInput] = useState('');
  const [sending, setSending] = useState(false);
  const [generatingMinutes, setGeneratingMinutes] = useState(false);
  const [loadError, setLoadError] = useState(false);
  const bottomRef = useRef<HTMLDivElement>(null);

  // Consume live room state from the global WS hub (websocket.ts routes
  // room_message / room_typing_start/stop / stream_start / stream_delta /
  // stream_end into these store slices). No local WebSocket needed.
  const storeIncoming = useStore((s) => s.roomIncomingMessages[roomId] ?? []);
  const storeTyping = useStore((s) => s.roomTyping[roomId] ?? []);
  const clearRoomIncoming = useStore((s) => s.clearRoomIncoming);

  const memberMap = new Map((room?.members || []).map(m => [m.agent_key, m]));
  const getMemberName = (s: string) => {
    // Try by agent_key first, then by id (store messages use both)
    return memberMap.get(s)?.display_name || souls.find(a => a.id === s)?.display_name || s;
  };
  const getMemberInitial = (s: string) => getMemberName(s).charAt(0).toUpperCase();

  // Merge HTTP-loaded history with live WS messages deduplicated by id
  const allMessages: RoomMsg[] = (() => {
    const seen = new Set(messages.map(m => m.id));
    const incoming = storeIncoming.filter((m: any) => !seen.has(m.id)).map((m: any) => ({
      id: m.id ?? String(Date.now()),
      sender_id: m.sender ?? m.sender_id ?? 'agent',
      sender_type: (m.sender === 'user' || m.sender_id === 'user' ? 'user' : 'soul') as 'user' | 'soul',
      content: m.content ?? '',
      message_type: m.message_type,
      streaming: m.streaming,
      created_at: m.created_at ?? new Date().toISOString(),
    }));
    return [...messages, ...incoming];
  })();

  // Typing: set of agent_keys/ids currently typing
  const typingAgentIds = new Set(storeTyping);

  // Load all data. Each callback is a plain async function — NOT responsible
  // for managing `cancelled`. The useEffect wrappers below own the cancel flag
  // and return the cleanup directly (not as a Promise resolved value), ensuring
  // in-flight requests are silenced when `id` changes or the component unmounts.
  const load = useCallback(async (cancelled: { current: boolean }) => {
    try {
      const [r, m] = await Promise.all([roomsApi.get(roomId), roomsApi.messages(roomId)]);
      if (!cancelled.current) {
        setLoadError(false);
        setRoom(r as any);
        setMessages(((m as any)?.messages as RoomMsg[] || []).reverse());
        clearRoomIncoming(roomId);
      }
    } catch {
      if (!cancelled.current) setLoadError(true);
    }
  }, [roomId, clearRoomIncoming]);

  const loadDecisions = useCallback(async (cancelled: { current: boolean }) => {
    try {
      const data = await roomsApi.decisions(roomId);
      if (!cancelled.current) setDecisions((data as any)?.decisions || []);
    } catch { /* swallow — tab shows empty state */ }
  }, [roomId]);

  const loadTasks = useCallback(async (cancelled: { current: boolean }) => {
    try {
      const data = await roomsApi.tasks(roomId);
      if (!cancelled.current) setTasks((data as any)?.tasks || []);
    } catch { /* swallow */ }
  }, [roomId]);

  const loadMinutes = useCallback(async (cancelled: { current: boolean }) => {
    try {
      const data = await roomsApi.minutes(roomId);
      if (!cancelled.current) setMinutesList((data as any)?.minutes || []);
    } catch { /* swallow */ }
  }, [roomId]);

  const loadOrg = useCallback(async (cancelled: { current: boolean }) => {
    try {
      const data = await roomsApi.org(roomId);
      if (!cancelled.current) setOrgMembers((data as any)?.members || []);
    } catch { /* swallow */ }
  }, [roomId]);

  useEffect(() => {
    const cancelled = { current: false };
    load(cancelled);
    return () => { cancelled.current = true; };
  }, [load]);

  useEffect(() => {
    const cancelled = { current: false };
    if (tab === 'decisions') loadDecisions(cancelled);
    else if (tab === 'tasks') loadTasks(cancelled);
    else if (tab === 'minutes') loadMinutes(cancelled);
    else if (tab === 'org') loadOrg(cancelled);
    return () => { cancelled.current = true; };
  }, [tab, loadDecisions, loadTasks, loadMinutes, loadOrg]);

  // Auto-scroll when new messages arrive
  useEffect(() => {
    if (tab === 'chat') bottomRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [allMessages.length, tab]);

  // Reload history after a streaming turn completes (stream_end)
  useEffect(() => {
    const hasCompleted = storeIncoming.some((m: any) => m.streaming === false);
    if (hasCompleted) {
      const t = setTimeout(() => load({ current: false }), 500);
      return () => clearTimeout(t);
    }
  }, [storeIncoming, load]);

  const handleSend = async () => {
    if (!input.trim() || sending) return;
    const text = input.trim();
    setInput('');
    setSending(true);
    // Optimistic user bubble — real server echo arrives via room_message WS event
    setMessages(prev => [...prev, {
      id: `optimistic-${Date.now()}`, sender_id: 'user', sender_type: 'user',
      content: text, created_at: new Date().toISOString(),
    }]);
    try {
      await roomsApi.sendMessage(roomId, text);
    } catch {
      // Non-fatal — message shows optimistically; WS will deliver actual state
    } finally {
      setSending(false);
    }
  };

  const handleGenerateMinutes = async () => {
    setGeneratingMinutes(true);
    try {
      await roomsApi.generateMinutes(roomId);
      await loadMinutes({ current: false });
      setTab('minutes');
    } finally {
      setGeneratingMinutes(false);
    }
  };

  const handleUpdateTask = async (taskId: string, status: string) => {
    try {
      await roomsApi.updateTask(roomId, taskId, status);
      await loadTasks({ current: false });
    } catch { /* UI will re-fetch on next tab visit */ }
  };

  if (loadError) return (
    <div className="flex h-full flex-col items-center justify-center gap-3">
      <p className="text-sm text-muted-foreground">Failed to load this hub.</p>
      <button
        onClick={() => { setLoadError(false); load({ current: false }); }}
        className="flex items-center gap-1.5 rounded-lg border border-border px-3 py-1.5 text-xs hover:bg-accent"
      >
        <RefreshCw className="h-3.5 w-3.5" /> Retry
      </button>
    </div>
  );

  if (!room) return (
    <div className="flex h-full items-center justify-center">
      <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
    </div>
  );

  const typingList = [...typingAgentIds].map(k => getMemberName(k)).join(', ');

  return (
    <div className="flex h-[calc(100vh-var(--header-height)-1px)] flex-col overflow-hidden">
      {/* Header */}
      <div className="flex items-center gap-3 border-b border-border px-4 py-2 shrink-0">
        {showBack && (
          <Link href="/rooms" className="flex h-8 w-8 items-center justify-center rounded-lg text-muted-foreground hover:bg-accent">
            <ArrowLeft className="h-4 w-4" />
          </Link>
        )}
        <div className="flex-1 min-w-0">
          <p className="text-sm font-semibold truncate">{room.display_name || room.name}</p>
          <p className="text-xs text-muted-foreground truncate">
            {room.members?.length || 0} agents · {room.description || 'Autonomous hub collaboration'}
          </p>
        </div>
        <button
          onClick={handleGenerateMinutes}
          disabled={generatingMinutes}
          title="Generate meeting minutes"
          className="flex items-center gap-1.5 rounded-lg border border-border px-2.5 py-1.5 text-xs text-muted-foreground hover:bg-accent disabled:opacity-50"
        >
          {generatingMinutes ? <Loader2 className="h-3 w-3 animate-spin" /> : <Sparkles className="h-3 w-3" />}
          Minutes
        </button>
      </div>

      {/* Tabs */}
      <div className="flex border-b border-border shrink-0">
        {TABS.map(({ id: tabId, label, icon: Icon }) => (
          <button
            key={tabId}
            onClick={() => setTab(tabId)}
            className={cn(
              'flex items-center gap-1.5 px-4 py-2.5 text-xs font-medium border-b-2 transition-colors',
              tab === tabId
                ? 'border-primary text-primary'
                : 'border-transparent text-muted-foreground hover:text-foreground'
            )}
          >
            <Icon className="h-3.5 w-3.5" />
            {label}
          </button>
        ))}
      </div>

      {/* Tab content */}
      <div className="flex flex-1 overflow-hidden">
        {/* ── Chat ── */}
        {tab === 'chat' && (
          <div className="flex flex-1 flex-col overflow-hidden">
            <div className="flex-1 overflow-y-auto px-4 py-3 space-y-3">
              {allMessages.length === 0 && (
                <div className="py-16 text-center text-sm text-muted-foreground">
                  No messages yet. Start the conversation — agents will respond.
                </div>
              )}
              {allMessages.map(msg => (
                <ChatBubble key={msg.id} msg={msg} getMemberName={getMemberName} getMemberInitial={getMemberInitial} />
              ))}
              {typingAgentIds.size > 0 && (
                <div className="flex items-center gap-2 text-xs text-muted-foreground">
                  <div className="flex gap-0.5">
                    {[0, 1, 2].map(i => (
                      <span key={i} className="inline-block h-1.5 w-1.5 rounded-full bg-muted-foreground animate-bounce"
                        style={{ animationDelay: `${i * 0.15}s` }} />
                    ))}
                  </div>
                  {typingList} {typingAgentIds.size === 1 ? 'is' : 'are'} thinking…
                </div>
              )}
              <div ref={bottomRef} />
            </div>

            {/* Input */}
            <div className="border-t border-border p-3 shrink-0">
              <p className="text-xs text-muted-foreground mb-1.5">
                Tip: start with <code className="bg-muted px-1 rounded">DECISION:</code> to pin · <code className="bg-muted px-1 rounded">@agent_key</code> to mention
              </p>
              <div className="flex items-end gap-2">
                <textarea
                  value={input}
                  onChange={e => setInput(e.target.value)}
                  onKeyDown={e => e.key === 'Enter' && !e.shiftKey && (e.preventDefault(), handleSend())}
                  rows={1}
                  placeholder="Message this hub… agents will respond"
                  className="qr-textarea flex-1 resize-none max-h-32"
                />
                <button
                  onClick={handleSend}
                  disabled={!input.trim() || sending}
                  className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-primary text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
                >
                  {sending ? <Loader2 className="h-4 w-4 animate-spin" /> : <Send className="h-4 w-4" />}
                </button>
              </div>
            </div>
          </div>
        )}

        {/* ── Decisions ── */}
        {tab === 'decisions' && (
          <div className="flex-1 overflow-y-auto p-4 space-y-3">
            {/* Human pin-a-decision form. Agents pin via room_decide;
                this is how a user weighs in without running the tool. */}
            <PinDecisionForm
              roomId={roomId}
              onPinned={() => loadDecisions({ current: false })}
            />

            {decisions.length === 0 && (
              <div className="py-10 text-center text-sm text-muted-foreground">
                No decisions yet. Pin one above, or agents will pin via <code className="bg-muted px-1 rounded">DECISION:</code> / <code className="bg-muted px-1 rounded">room_decide</code>.
              </div>
            )}
            {decisions.map(d => (
              <div key={d.id} className="rounded-lg border border-amber-500/30 bg-amber-500/5 p-4">
                <div className="flex items-start gap-2">
                  <Pin className="h-4 w-4 text-amber-500 mt-0.5 shrink-0" />
                  <div className="flex-1">
                    <p className="text-sm">{d.content}</p>
                    <div className="mt-1 flex items-center gap-2 text-xs text-muted-foreground">
                      <span>
                        Decided by <span className="font-medium">{getMemberName(d.decided_by)}</span>
                      </span>
                      <span>·</span>
                      <span>{new Date(d.created_at).toLocaleString()}</span>
                      {d.status && d.status !== 'active' && (
                        <span className={cn(
                          'ml-auto rounded-sm px-1.5 py-0.5 font-mono uppercase',
                          d.status === 'reversed' && 'bg-destructive/15 text-destructive',
                          d.status === 'superseded' && 'bg-muted text-muted-foreground',
                          d.status === 'pending' && 'bg-amber-400/15 text-amber-400',
                        )}>
                          {d.status}
                        </span>
                      )}
                    </div>
                  </div>
                </div>
              </div>
            ))}
          </div>
        )}

        {/* ── Tasks ── */}
        {tab === 'tasks' && (
          <div className="flex-1 overflow-y-auto p-4 space-y-3">
            <AssignTaskForm roomId={roomId} members={room?.members || []} onAssigned={() => loadTasks({ current: false })} />
            {tasks.length === 0 && (
              <div className="py-8 text-center text-sm text-muted-foreground">
                No tasks yet. Assign one above, or agents assign via <code className="bg-muted px-1 rounded">room_assign</code> tool.
              </div>
            )}
            {tasks.map(t => (
              <div key={t.id} className="rounded-lg border border-border bg-card p-4">
                <div className="flex items-start justify-between gap-3">
                  <div className="flex-1">
                    <p className="text-sm font-medium">{t.title}</p>
                    {t.description && <p className="text-xs text-muted-foreground mt-0.5">{t.description}</p>}
                    <p className="text-xs text-muted-foreground mt-1.5">
                      <span className="font-medium">{getMemberName(t.assigned_to)}</span>
                      {t.due_at && <> · due {new Date(t.due_at).toLocaleDateString()}</>}
                      {' · '}assigned by {getMemberName(t.assigned_by)}
                    </p>
                  </div>
                  <select
                    value={t.status}
                    onChange={e => handleUpdateTask(t.id, e.target.value)}
                    className={cn(
                      'qr-select w-auto text-xs cursor-pointer',
                      t.status === 'done' && 'border-green-500/50 text-green-600',
                      t.status === 'in_progress' && 'border-blue-500/50 text-blue-600',
                      t.status === 'blocked' && 'border-red-500/50 text-red-600',
                      t.status === 'pending' && 'border-border text-muted-foreground'
                    )}
                  >
                    <option value="pending">Pending</option>
                    <option value="in_progress">In Progress</option>
                    <option value="done">Done</option>
                    <option value="blocked">Blocked</option>
                  </select>
                </div>
              </div>
            ))}
          </div>
        )}

        {/* ── Minutes ── */}
        {tab === 'minutes' && (
          <div className="flex-1 overflow-y-auto p-4 space-y-4">
            {minutesList.length === 0 && (
              <div className="py-16 text-center">
                <FileText className="mx-auto h-8 w-8 text-muted-foreground/50 mb-3" />
                <p className="text-sm text-muted-foreground">No meeting minutes yet.</p>
                <button
                  onClick={handleGenerateMinutes}
                  disabled={generatingMinutes}
                  className="mt-3 inline-flex items-center gap-2 rounded-lg bg-primary px-4 py-2 text-sm text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
                >
                  {generatingMinutes ? <Loader2 className="h-4 w-4 animate-spin" /> : <Sparkles className="h-4 w-4" />}
                  Generate from last 100 messages
                </button>
              </div>
            )}
            {minutesList.map(m => (
              <div key={m.id} className="rounded-lg border border-border bg-card p-4 space-y-3">
                <div className="flex items-center justify-between">
                  <p className="text-xs font-medium text-muted-foreground">
                    <Clock className="inline h-3 w-3 mr-1" />
                    {new Date(m.generated_at).toLocaleString()} · {m.msg_count} messages
                  </p>
                </div>
                <p className="text-sm">{m.summary}</p>

                {m.decisions?.length > 0 && (
                  <div>
                    <p className="text-xs font-semibold text-amber-600 mb-1.5">Decisions</p>
                    <ul className="space-y-1">
                      {m.decisions.map((d, i) => (
                        <li key={i} className="text-xs flex gap-2">
                          <Pin className="h-3 w-3 text-amber-500 mt-0.5 shrink-0" />
                          <span>{d.text} <span className="text-muted-foreground">— {getMemberName(d.decided_by)}</span></span>
                        </li>
                      ))}
                    </ul>
                  </div>
                )}

                {m.action_items?.length > 0 && (
                  <div>
                    <p className="text-xs font-semibold text-blue-600 mb-1.5">Action Items</p>
                    <ul className="space-y-1">
                      {m.action_items.map((a, i) => (
                        <li key={i} className="text-xs flex gap-2">
                          <CheckSquare className="h-3 w-3 text-blue-500 mt-0.5 shrink-0" />
                          <span>{a.task} <span className="text-muted-foreground">→ {getMemberName(a.assigned_to)}{a.due ? ` (${a.due})` : ''}</span></span>
                        </li>
                      ))}
                    </ul>
                  </div>
                )}

                {m.blockers?.length > 0 && (
                  <div>
                    <p className="text-xs font-semibold text-red-600 mb-1.5">Blockers</p>
                    <ul className="space-y-1">
                      {m.blockers.map((b, i) => (
                        <li key={i} className="text-xs flex gap-2">
                          <Circle className="h-3 w-3 text-red-500 mt-0.5 shrink-0" />
                          <span>{b.issue} <span className="text-muted-foreground">raised by {getMemberName(b.raised_by)}</span></span>
                        </li>
                      ))}
                    </ul>
                  </div>
                )}
              </div>
            ))}
          </div>
        )}

        {/* ── Org Chart ── */}
        {tab === 'org' && (
          <div className="flex-1 overflow-y-auto p-4 space-y-5">
            {/* Member management */}
            <MembersManagement
              roomId={roomId}
              currentMembers={room?.members || []}
              allSouls={souls}
              onChanged={() => { load({ current: false }); loadOrg({ current: false }); }}
            />

            {orgMembers.length === 0 && (
              <div className="py-8 text-center text-sm text-muted-foreground">No org data available.</div>
            )}
            {orgMembers.length > 0 && (
              <div className="space-y-4">
                {/* Coordinator layer */}
                {orgMembers.filter(m => m.room_role === 'coordinator' || m.can_decide).length > 0 && (
                  <div>
                    <p className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2">Coordinators</p>
                    <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
                      {orgMembers
                        .filter(m => m.room_role === 'coordinator' || m.can_decide)
                        .map(m => <OrgCard key={m.id} member={m} onRemove={() => { load({ current: false }); loadOrg({ current: false }); }} roomId={roomId} />)}
                    </div>
                  </div>
                )}

                {/* Divider */}
                {orgMembers.filter(m => m.room_role === 'coordinator' || m.can_decide).length > 0 &&
                  orgMembers.filter(m => m.room_role !== 'coordinator' && !m.can_decide).length > 0 && (
                    <div className="flex items-center gap-2">
                      <div className="h-px flex-1 bg-border" />
                      <span className="text-xs text-muted-foreground">reports to</span>
                      <div className="h-px flex-1 bg-border" />
                    </div>
                  )}

                {/* Specialists layer */}
                {orgMembers.filter(m => m.room_role !== 'coordinator' && !m.can_decide).length > 0 && (
                  <div>
                    <p className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2">Specialists</p>
                    <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
                      {orgMembers
                        .filter(m => m.room_role !== 'coordinator' && !m.can_decide)
                        .sort((a, b) => (a.speaking_order || 99) - (b.speaking_order || 99))
                        .map(m => <OrgCard key={m.id} member={m} onRemove={() => { load({ current: false }); loadOrg({ current: false }); }} roomId={roomId} />)}
                    </div>
                  </div>
                )}
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  );
}

function OrgCard({ member, roomId, onRemove }: { member: Member; roomId: string; onRemove: () => void }) {
  const [busy, setBusy] = useState(false);
  const initial = member.display_name.charAt(0).toUpperCase();
  const isCoordinator = member.room_role === 'coordinator' || member.can_decide;

  const handleRemove = async () => {
    setBusy(true);
    try { await roomsApi.removeMember(roomId, member.id); onRemove(); }
    finally { setBusy(false); }
  };

  return (
    <div className={cn(
      'flex items-center gap-3 rounded-lg border p-3',
      isCoordinator ? 'border-amber-500/30 bg-amber-500/5' : 'border-border bg-card'
    )}>
      <div className="h-9 w-9 shrink-0 rounded-full bg-gradient-to-br from-primary to-primary/60 flex items-center justify-center text-sm font-semibold text-white">
        {initial}
      </div>
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-1.5">
          <p className="text-sm font-medium truncate">{member.display_name}</p>
          {isCoordinator && <Crown className="h-3 w-3 text-amber-500 shrink-0" />}
          {member.can_decide && !isCoordinator && <ShieldCheck className="h-3 w-3 text-blue-500 shrink-0" />}
        </div>
        <p className="text-xs text-muted-foreground truncate">
          {member.agent_role || member.room_role || 'member'}
          {member.speaking_order != null && ` · order ${member.speaking_order}`}
        </p>
      </div>
      {member.last_active_at && (
        <div className="h-2 w-2 rounded-full bg-green-500 shrink-0" title="Recently active" />
      )}
      <button
        onClick={handleRemove}
        disabled={busy}
        title="Remove from hub"
        className="flex h-7 w-7 items-center justify-center rounded-md text-muted-foreground hover:bg-destructive/10 hover:text-destructive disabled:opacity-50 shrink-0"
      >
        {busy ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <UserMinus className="h-3.5 w-3.5" />}
      </button>
    </div>
  );
}

function MembersManagement({
  roomId, currentMembers, allSouls, onChanged,
}: {
  roomId: string;
  currentMembers: Member[];
  allSouls: { id: string; display_name: string; agent_key?: string }[];
  onChanged: () => void;
}) {
  const [selectedId, setSelectedId] = useState('');
  const [busy, setBusy] = useState(false);

  const memberIds = new Set(currentMembers.map(m => m.id));
  const available = allSouls.filter(s => !memberIds.has(s.id));

  const handleAdd = async () => {
    if (!selectedId || busy) return;
    setBusy(true);
    try { await roomsApi.addMember(roomId, selectedId); setSelectedId(''); onChanged(); }
    finally { setBusy(false); }
  };

  return (
    <div className="rounded-lg border border-dashed border-border/80 bg-card/40 p-3">
      <p className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2 flex items-center gap-1.5">
        <Users className="h-3.5 w-3.5" /> Manage Members
      </p>
      <div className="flex items-center gap-2">
        <select
          value={selectedId}
          onChange={e => setSelectedId(e.target.value)}
          className="qr-select flex-1 text-xs"
        >
          <option value="">Add an agent…</option>
          {available.map(s => (
            <option key={s.id} value={s.id}>{s.display_name || s.agent_key}</option>
          ))}
        </select>
        <button
          onClick={handleAdd}
          disabled={!selectedId || busy}
          className="qr-btn qr-btn-primary qr-btn-sm"
        >
          {busy ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <UserPlus className="h-3.5 w-3.5" />}
          Add
        </button>
      </div>
      {currentMembers.length > 0 && (
        <p className="mt-2 text-2xs text-muted-foreground">
          {currentMembers.length} member{currentMembers.length !== 1 ? 's' : ''} · remove via × on each card below
        </p>
      )}
    </div>
  );
}


// Human assign-a-task form. Sits at the top of the Tasks tab so users can
// create tracked tasks without going through a tool. Mirrors PinDecisionForm.
function AssignTaskForm({ roomId, members, onAssigned }: { roomId: string; members: Member[]; onAssigned: () => void }) {
  const [title, setTitle] = useState('');
  const [assignedTo, setAssignedTo] = useState('');
  const [busy, setBusy] = useState(false);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!title.trim() || busy) return;
    setBusy(true);
    try {
      await roomsApi.createTask(roomId, { title: title.trim(), assigned_to: assignedTo || 'unassigned' });
      setTitle('');
      setAssignedTo('');
      onAssigned();
    } finally {
      setBusy(false);
    }
  };

  return (
    <form onSubmit={submit} className="flex gap-2 rounded-lg border border-dashed border-border/80 bg-card/40 p-3">
      <CheckSquare className="h-4 w-4 shrink-0 self-center text-blue-400" />
      <input
        value={title}
        onChange={(e) => setTitle(e.target.value)}
        placeholder="Task — e.g. &quot;Review the API spec by Friday&quot;"
        className="qr-input flex-1 text-xs"
      />
      <select
        value={assignedTo}
        onChange={(e) => setAssignedTo(e.target.value)}
        className="qr-select w-auto text-xs"
      >
        <option value="">Assign to…</option>
        {members.map(m => (
          <option key={m.id} value={m.agent_key}>{m.display_name || m.agent_key}</option>
        ))}
      </select>
      <button
        type="submit"
        disabled={busy || !title.trim()}
        className="qr-btn qr-btn-primary qr-btn-sm"
      >
        {busy ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <CheckSquare className="h-3.5 w-3.5" />}
        Assign
      </button>
    </form>
  );
}

export default function RoomDetailPage() {
  const { id } = useParams<{ id: string }>();
  return <RoomDetail roomId={id} showBack />;
}

// Human pin-a-decision form. Sits at the top of the Decisions tab so
// the user has a first-class way to say "this is the call" without
// going through a tool. Passes decided_by: 'user' so the list shows
// "Decided by user" — matches the agent-pinned format.
function PinDecisionForm({ roomId, onPinned }: { roomId: string; onPinned: () => void }) {
  const [content, setContent] = useState('');
  const [busy, setBusy] = useState(false);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!content.trim() || busy) return;
    setBusy(true);
    try {
      await roomsApi.pinDecision(roomId, content.trim(), 'user');
      setContent('');
      onPinned();
    } finally {
      setBusy(false);
    }
  };

  return (
    <form onSubmit={submit} className="flex gap-2 rounded-lg border border-dashed border-border/80 bg-card/40 p-3">
      <Pin className="h-4 w-4 shrink-0 self-center text-amber-400" />
      <input
        value={content}
        onChange={(e) => setContent(e.target.value)}
        placeholder="Pin a decision — e.g. &quot;ship the migration behind a flag first&quot;"
        className="qr-input flex-1 text-xs"
      />
      <button
        type="submit"
        disabled={busy || !content.trim()}
        className="inline-flex items-center gap-1.5 rounded-md bg-primary px-3 py-1 text-xs font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
      >
        {busy ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Pin className="h-3.5 w-3.5" />}
        Pin
      </button>
    </form>
  );
}
