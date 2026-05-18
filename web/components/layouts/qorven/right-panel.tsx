'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

// Right panel — slides in from the right edge
// Four tabs: Chat (Prime), Notifications, Activity, Tasks
// Opened/closed by header icon buttons via Zustand store

import { useStore } from '@/store';
import { useEffect, useRef, useState, useCallback } from 'react';
import { cn } from '@/lib/utils';
import { X, MessageSquare, Bell, Activity, Send, Loader2, Brain, ShieldAlert, CheckCircle2, XCircle, Zap, Infinity } from 'lucide-react';
import { notifications as notifApi, chat as chatApi, sessions, providers as providersApi, permissions } from '@/lib/api';
import { tasks as tasksApi } from '@/lib/api-workspace';
import { agents } from '@/lib/api-agents';
import { modelDisplayName } from '@/lib/model-names';
import { TelemetryLog } from '@/components/chat/telemetry-log';
import { ApprovalCards } from '@/components/chat/approval-card';
import { GitHubPRCards } from '@/components/chat/github-pr-card';
import { CommitApprovalCards } from '@/components/chat/commit-approval-card';

const PANEL_W = 320;

export function RightPanel() {
  const open = useStore((s) => s.rightPanelOpen);
  const tab = useStore((s) => s.rightPanelTab);
  const closeRightPanel = useStore((s) => s.closeRightPanel);
  const openRightPanel = useStore((s) => s.openRightPanel);

  return (
    <div
      className={cn(
        'right-panel fixed top-[var(--header-height)] right-0 bottom-0 z-20 flex flex-col border-l border-border bg-background shadow-xl',
        'transition-transform duration-[var(--sidebar-transition-duration)]',
      )}
      style={{
        width: PANEL_W,
        transform: open ? 'translateX(0)' : `translateX(${PANEL_W}px)`,
      }}>

      {/* Tab bar */}
      <div className="flex items-center border-b border-border shrink-0 bg-muted/20">
        {(['chat', 'notifications', 'activity', 'tasks'] as const).map((t) => {
          const Icon =
            t === 'chat' ? MessageSquare :
            t === 'notifications' ? Bell :
            t === 'tasks' ? Zap :
            Activity;
          const label =
            t === 'chat' ? 'Chat' :
            t === 'notifications' ? 'Alerts' :
            t === 'tasks' ? 'Tasks' :
            'Activity';
          return (
            <button key={t} onClick={() => openRightPanel(t)}
              className={cn(
                'flex flex-1 items-center justify-center gap-1.5 py-2.5 text-xs font-medium border-b-2 transition-colors',
                tab === t ? 'border-primary text-foreground' : 'border-transparent text-muted-foreground hover:text-foreground'
              )}>
              <Icon className="h-3.5 w-3.5" />
              {label}
            </button>
          );
        })}
        <button onClick={closeRightPanel} className="px-2 text-muted-foreground hover:text-foreground transition-colors">
          <X className="h-4 w-4" />
        </button>
      </div>

      {/* Tab content */}
      <div className="flex-1 overflow-hidden">
        {tab === 'chat' && <ChatTab />}
        {tab === 'notifications' && <NotificationsTab />}
        {tab === 'activity' && <ActivityTab />}
        {tab === 'tasks' && <TasksTab />}
      </div>
    </div>
  );
}

// ─── Chat Tab — Prime AI assistant ──────────────────────────────────────────

interface ChatMsg { role: 'user' | 'assistant'; content: string }

function ChatTab() {
  const souls = useStore((s) => s.souls);
  const [messages, setMessages] = useState<ChatMsg[]>([
    { role: 'assistant', content: 'Hi! I\'m Prime. Ask me anything about your workspace.' }
  ]);
  const [input, setInput] = useState('');
  const [loading, setLoading] = useState(false);
  const bottomRef = useRef<HTMLDivElement>(null);
  const sessionRef = useRef<string>(`right-panel-${Date.now()}`);

  // Find prime agent
  const prime = souls.find(s => s.agent_key === 'prime' || s.role === 'supervisor');

  useEffect(() => { bottomRef.current?.scrollIntoView({ behavior: 'smooth' }); }, [messages]);

  const send = useCallback(async () => {
    if (!input.trim() || loading || !prime) return;
    const text = input.trim();
    setInput('');
    setMessages(prev => [...prev, { role: 'user', content: text }]);
    setLoading(true);

    try {
      // Create session if needed
      let sessId = sessionRef.current;
      try {
        await sessions.create({ agent_id: prime.id, channel: 'web' }).then(s => { sessId = s.id; sessionRef.current = s.id; });
      } catch { /* use existing */ }

      const res = await chatApi.send({ session_id: sessId, agent_id: prime.id, message: text, stream: true });
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
              const delta = evt.choices?.[0]?.delta?.content || (evt.type === 'text_delta' ? evt.data?.content ?? evt.data : null);
              if (delta) { accumulated += delta; setMessages(prev => { const next = [...prev]; const last = next[next.length - 1]; if (last?.role === 'assistant' && last.content.startsWith('▌')) { last.content = accumulated; } else { next.push({ role: 'assistant', content: accumulated }); } return next; }); }
            } catch { /* ignore */ }
          }
        }
      }
      if (!accumulated) setMessages(prev => [...prev, { role: 'assistant', content: '(no response)' }]);
    } catch (e) {
      setMessages(prev => [...prev, { role: 'assistant', content: `Error: ${e instanceof Error ? e.message : 'Failed'}` }]);
    } finally { setLoading(false); }
  }, [input, loading, prime]);

  if (!prime) return (
    <div className="flex h-full items-center justify-center p-4 text-center">
      <p className="text-sm text-muted-foreground">No Prime agent found. Create one in Qors.</p>
    </div>
  );

  return (
    <div className="flex h-full flex-col">
      <div className="flex-1 overflow-y-auto p-3 space-y-3">
        {messages.map((m, i) => (
          <div key={i} className={cn('flex gap-2', m.role === 'user' && 'flex-row-reverse')}>
            <div className={cn('max-w-[85%] rounded-xl px-3 py-2 text-xs leading-relaxed',
              m.role === 'user' ? 'bg-primary text-primary-foreground' : 'bg-muted text-foreground')}>
              {m.content}
            </div>
          </div>
        ))}
        {loading && (
          <TelemetryLog sessionId={sessionRef.current} active={loading} />
        )}
        {/* Permission approvals — always rendered so a reply can be
            issued even after streaming has stopped. */}
        <ApprovalCards sessionId={sessionRef.current} />
        <GitHubPRCards sessionId={sessionRef.current} />
        <CommitApprovalCards sessionId={sessionRef.current} />
        <div ref={bottomRef} />
      </div>
      <div className="border-t border-border p-2.5 flex gap-2">
        <input value={input} onChange={e => setInput(e.target.value)}
          onKeyDown={e => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); send(); } }}
          placeholder="Ask Prime…"
          className="qr-input text-xs" />
        <button onClick={send} disabled={loading || !input.trim()}
          className="h-8 w-8 flex items-center justify-center rounded-lg bg-primary text-primary-foreground hover:bg-primary/90 disabled:opacity-40 cursor-pointer">
          {loading ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Send className="h-3.5 w-3.5" />}
        </button>
      </div>
    </div>
  );
}

// ─── Notifications Tab ───────────────────────────────────────────────────────

function NotificationsTab() {
  const [items, setItems] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [discovered, setDiscovered] = useState<any[]>([]);
  const approvals = useStore((s) => s.approvals);
  const markApprovalResolved = useStore((s) => s.markApprovalResolved);
  const [submitting, setSubmitting] = useState<Record<string, 'allow' | 'allow_always' | 'deny'>>({});

  const pendingApprovals = Object.values(approvals).filter((a) => !a.resolved);

  const toolLabel = (tool: string) => {
    const labels: Record<string, string> = {
      gh_push_file: 'Write a file to GitHub', gh_open_pr: 'Open a Pull Request',
      gh_merge_pr: 'Merge a Pull Request', gh_create_repo: 'Create a GitHub repository',
      exec: 'Run a shell command', write_file: 'Write a file to disk',
      delete_file: 'Delete a file', cron: 'Schedule a recurring task',
    };
    return labels[tool] ?? tool.replace(/_/g, ' ');
  };

  const sendDecision = async (requestId: string, decision: 'allow' | 'allow_always' | 'deny') => {
    setSubmitting((prev) => ({ ...prev, [requestId]: decision }));
    try {
      await permissions.reply(requestId, { decision });
      markApprovalResolved(requestId, decision === 'deny' ? 'deny' : 'allow', { actor: 'me' });
    } catch { /* optimistic already handled */ } finally {
      setSubmitting((prev) => { const n = { ...prev }; delete n[requestId]; return n; });
    }
  };

  useEffect(() => {
    notifApi.list().then(d => { setItems(Array.isArray(d) ? d : []); setLoading(false); }).catch(() => setLoading(false));
    providersApi.discoveredModels(true).then(d => setDiscovered(Array.isArray(d) ? d : [])).catch(() => {});
  }, []);

  const dismissDiscovered = async (id: string, action: 'enable' | 'dismiss') => {
    await providersApi.actionDiscoveredModel(id, action).catch(() => {});
    setDiscovered(prev => prev.filter(d => d.id !== id));
  };

  const markRead = async (id: string) => {
    await notifApi.markRead(id).catch(() => {});
    setItems(prev => prev.map(n => n.id === id ? { ...n, read_at: new Date().toISOString() } : n));
  };

  const markAllRead = async () => {
    await notifApi.markAllRead().catch(() => {});
    setItems(prev => prev.map(n => ({ ...n, read_at: new Date().toISOString() })));
  };

  return (
    <div className="flex h-full flex-col">
      {/* Pending permission-gate approvals — shown first, highest urgency */}
      {pendingApprovals.length > 0 && (
        <div className="border-b border-amber-500/20 bg-amber-500/5 px-3 py-2.5 space-y-2">
          <div className="flex items-center gap-2">
            <ShieldAlert className="h-3.5 w-3.5 text-amber-500 shrink-0" />
            <span className="text-xs font-semibold text-amber-500">
              {pendingApprovals.length} agent tool request{pendingApprovals.length !== 1 ? 's' : ''} waiting
            </span>
          </div>
          {pendingApprovals.map((entry) => {
            const { request } = entry;
            const busy = submitting[request.request_id];
            return (
              <div key={request.request_id} className="rounded-lg border border-amber-500/30 bg-background/60 p-2 space-y-1.5">
                <div className="flex items-center gap-2">
                  <span className="text-2xs font-semibold text-amber-400 truncate flex-1">{toolLabel(request.tool)}</span>
                  {request.agent_key && (
                    <span className="text-2xs text-muted-foreground shrink-0">{request.agent_key}</span>
                  )}
                </div>
                {request.reason && (
                  <p className="text-2xs text-muted-foreground leading-relaxed">{request.reason}</p>
                )}
                <div className="flex gap-1">
                  <button
                    onClick={() => sendDecision(request.request_id, 'allow')}
                    disabled={!!busy}
                    className="flex-1 flex items-center justify-center gap-1 rounded-md bg-emerald-500/10 border border-emerald-500/30 text-emerald-600 text-2xs font-medium py-1 hover:bg-emerald-500/20 disabled:opacity-50 transition-colors"
                  >
                    {busy === 'allow' ? <Loader2 className="h-2.5 w-2.5 animate-spin" /> : <CheckCircle2 className="h-2.5 w-2.5" />}
                    Once
                  </button>
                  <button
                    onClick={() => sendDecision(request.request_id, 'allow_always')}
                    disabled={!!busy}
                    title="Allow always — no more prompts for this tool"
                    className="flex-1 flex items-center justify-center gap-1 rounded-md bg-blue-500/10 border border-blue-500/30 text-blue-500 text-2xs font-medium py-1 hover:bg-blue-500/20 disabled:opacity-50 transition-colors"
                  >
                    {busy === 'allow_always' ? <Loader2 className="h-2.5 w-2.5 animate-spin" /> : <Infinity className="h-2.5 w-2.5" />}
                    Always
                  </button>
                  <button
                    onClick={() => sendDecision(request.request_id, 'deny')}
                    disabled={!!busy}
                    className="flex-1 flex items-center justify-center gap-1 rounded-md bg-destructive/10 border border-destructive/30 text-destructive text-2xs font-medium py-1 hover:bg-destructive/20 disabled:opacity-50 transition-colors"
                  >
                    {busy === 'deny' ? <Loader2 className="h-2.5 w-2.5 animate-spin" /> : <XCircle className="h-2.5 w-2.5" />}
                    Deny
                  </button>
                </div>
              </div>
            );
          })}
        </div>
      )}

      {/* New models discovered banner */}
      {discovered.length > 0 && (
        <div className="border-b border-amber-400/20 bg-amber-400/5 px-3 py-2.5">
          <div className="flex items-center gap-2 mb-2">
            <Brain className="h-3.5 w-3.5 text-amber-400 shrink-0" />
            <span className="text-xs font-semibold text-amber-400">{discovered.length} new model{discovered.length !== 1 ? 's' : ''} discovered</span>
          </div>
          <div className="space-y-1.5">
            {discovered.slice(0, 5).map((d: any) => (
              <div key={d.id} className="flex items-center gap-2 rounded-md bg-amber-400/10 px-2 py-1.5">
                <span className="text-xs font-mono flex-1 truncate text-amber-300">{modelDisplayName(d.model_id)}</span>
                <span className="text-2xs text-muted-foreground shrink-0">{d.provider_id}</span>
                <button onClick={() => dismissDiscovered(d.id, 'enable')} className="text-2xs text-emerald-400 hover:underline cursor-pointer shrink-0">Enable</button>
                <button onClick={() => dismissDiscovered(d.id, 'dismiss')} className="text-2xs text-muted-foreground hover:text-foreground cursor-pointer shrink-0">✕</button>
              </div>
            ))}
            {discovered.length > 5 && (
              <a href="/models-hub?tab=generative" className="block text-2xs text-amber-400 hover:underline text-center pt-0.5">
                +{discovered.length - 5} more — open Models Hub
              </a>
            )}
          </div>
        </div>
      )}
      {items.length > 0 && (
        <div className="flex items-center justify-between px-3 py-2 border-b border-border">
          <span className="text-xs text-muted-foreground">{items.filter(n => !n.read_at).length} unread</span>
          <button onClick={markAllRead} className="text-xs text-primary hover:underline cursor-pointer">Mark all read</button>
        </div>
      )}
      <div className="flex-1 overflow-y-auto">
        {loading ? (
          <div className="flex items-center justify-center py-8"><Loader2 className="h-4 w-4 animate-spin text-muted-foreground" /></div>
        ) : items.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-12 text-center px-4">
            <Bell className="h-8 w-8 text-muted-foreground/30 mb-2" />
            <p className="text-sm text-muted-foreground">No notifications</p>
          </div>
        ) : items.map((n, i) => (
          <button key={i} onClick={() => markRead(n.id)}
            className={cn('w-full text-left px-3 py-2.5 border-b border-border/50 hover:bg-accent/50 transition-colors', !n.read_at && 'bg-primary/5')}>
            <div className="flex items-start gap-2">
              {!n.read_at && <span className="mt-1.5 h-1.5 w-1.5 rounded-full bg-primary shrink-0" />}
              <div className={cn('min-w-0', n.read_at && 'pl-3.5')}>
                <p className="text-xs font-medium truncate">{n.title || n.type}</p>
                <p className="text-2xs text-muted-foreground mt-0.5 line-clamp-2">{n.highlight || n.body || n.detail}</p>
                <p className="text-2xs text-muted-foreground/60 mt-0.5">{n.created_at ? new Date(n.created_at).toLocaleTimeString() : ''}</p>
              </div>
            </div>
          </button>
        ))}
      </div>
    </div>
  );
}

// ─── Activity Tab — real-time agent events ───────────────────────────────────

function ActivityTab() {
  const liveEvents = useStore((s) => s.liveEvents);
  const souls = useStore((s) => s.souls);

  const typeColor: Record<string, string> = {
    soul_activity:  'text-amber-400',   soul_completed: 'text-emerald-400',
    new_message:    'text-blue-400',    task_updated:   'text-purple-400',
    stream_start:   'text-primary',     stream_end:     'text-muted-foreground',
    error:          'text-destructive', tool_start:     'text-cyan-400',
    tool_result:    'text-cyan-400',
  };

  return (
    <div className="flex h-full flex-col">
      <div className="flex-1 overflow-y-auto">
        {liveEvents.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-12 text-center px-4">
            <Activity className="h-8 w-8 text-muted-foreground/30 mb-2" />
            <p className="text-sm text-muted-foreground">No activity yet</p>
            <p className="text-xs text-muted-foreground/60 mt-1">Agent events will appear here in real-time</p>
          </div>
        ) : liveEvents.slice(0, 50).map((e) => {
          const soul = e.agent_id ? souls.find(s => s.id === e.agent_id) : null;
          return (
            <div key={e.id} className="flex items-start gap-2 px-3 py-2 border-b border-border/30 hover:bg-accent/30 transition-colors">
              {soul && (
                <div className="h-5 w-5 rounded-full bg-primary/20 flex items-center justify-center text-xs font-semibold text-primary shrink-0 mt-0.5">
                  {soul.display_name[0]?.toUpperCase()}
                </div>
              )}
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-1.5">
                  {soul && <span className="text-2xs font-medium text-foreground/80 truncate">{soul.display_name}</span>}
                  <span className={cn('text-2xs font-mono', typeColor[e.type] ?? 'text-muted-foreground')}>{e.type}</span>
                </div>
                {e.detail && <p className="text-2xs text-muted-foreground mt-0.5 truncate">{e.detail}</p>}
                <p className="text-2xs text-muted-foreground/50">{new Date(e.timestamp).toLocaleTimeString()}</p>
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}

// ─── Tasks Tab — live task monitor ──────────────────────────────────────────

type LiveTask = {
  id: string;
  title: string;
  status: string;
  iteration: number;
  scratchpad: string;
  last_tool?: string;
  updated_at: string;
};

function TasksTab() {
  const liveTasks = useStore((s) => s.liveTasks);
  const agentId = useStore((s) => s.liveTaskAgentId);
  const [overrideInput, setOverrideInput] = useState('');
  const [busy, setBusy] = useState<string | null>(null);
  const tasks = Object.values(liveTasks ?? {});

  const doAction = async (label: string, fn: () => Promise<unknown>) => {
    setBusy(label);
    try { await fn(); } catch { /* non-fatal */ } finally { setBusy(null); }
  };

  const sendOverride = () => {
    if (!overrideInput.trim() || !agentId) return;
    const msg = overrideInput.trim();
    setOverrideInput('');
    doAction('override', () => agents.runtimeOverride(agentId, msg));
  };

  if (tasks.length === 0) {
    return (
      <div className="flex h-full flex-col">
        <div className="flex flex-col items-center justify-center flex-1 text-center px-4">
          <Zap className="h-8 w-8 text-muted-foreground/30 mb-2" />
          <p className="text-sm text-muted-foreground">No active tasks</p>
          <p className="text-xs text-muted-foreground/60 mt-1">Tasks will appear here when the agent starts working</p>
        </div>
        {agentId && (
          <div className="border-t border-border p-2.5 flex gap-2">
            <input
              value={overrideInput}
              onChange={e => setOverrideInput(e.target.value)}
              onKeyDown={e => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); sendOverride(); } }}
              placeholder="Send instruction to agent"
              className="qr-input text-xs"
            />
            <button
              onClick={sendOverride}
              disabled={!!busy || !overrideInput.trim()}
              className="h-8 w-8 flex items-center justify-center rounded-lg bg-primary text-primary-foreground hover:bg-primary/90 disabled:opacity-40"
            >
              {busy === 'override' ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Send className="h-3.5 w-3.5" />}
            </button>
          </div>
        )}
      </div>
    );
  }

  return (
    <div className="flex h-full flex-col">
      {agentId && (
        <div className="border-b border-border/50 px-3 py-2 flex items-center justify-end">
          <span className="text-2xs text-muted-foreground">{tasks.length} task{tasks.length !== 1 ? 's' : ''}</span>
        </div>
      )}
      <div className="flex-1 overflow-y-auto space-y-2 p-2">
        {tasks.map((task) => (
          <TaskCard key={task.id} task={task} />
        ))}
      </div>
      {agentId && (
        <div className="border-t border-border p-2.5 flex gap-2">
          <input
            value={overrideInput}
            onChange={e => setOverrideInput(e.target.value)}
            onKeyDown={e => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); sendOverride(); } }}
            placeholder="Send instruction to agent"
            className="qr-input text-xs"
          />
          <button
            onClick={sendOverride}
            disabled={!!busy || !overrideInput.trim()}
            className="h-8 w-8 flex items-center justify-center rounded-lg bg-primary text-primary-foreground hover:bg-primary/90 disabled:opacity-40"
          >
            {busy === 'override' ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Send className="h-3.5 w-3.5" />}
          </button>
        </div>
      )}
    </div>
  );
}

function TaskCard({ task }: { task: LiveTask }) {
  const [expanded, setExpanded] = useState(false);
  const [msgInput, setMsgInput] = useState('');
  const [busy, setBusy] = useState<string | null>(null);
  const setLiveTask = useStore((s) => s.setLiveTask);

  const statusColor =
    task.status === 'done' ? 'border-emerald-500/30 bg-emerald-500/5' :
    task.status === 'blocked' ? 'border-amber-500/30 bg-amber-500/5' :
    task.status === 'paused' ? 'border-yellow-500/30 bg-yellow-500/5' :
    'border-blue-500/30 bg-blue-500/5';

  const statusIcon =
    task.status === 'done' ? '✓' :
    task.status === 'blocked' ? '⚠' :
    task.status === 'paused' ? '⏸' : '⚡';

  const lastLine = task.scratchpad?.split('\n').filter(Boolean).pop() ?? '';

  const doAction = async (label: string, fn: () => Promise<unknown>) => {
    setBusy(label);
    try { await fn(); } catch { /* non-fatal */ } finally { setBusy(null); }
  };

  const sendMessage = () => {
    if (!msgInput.trim()) return;
    const msg = msgInput.trim();
    setMsgInput('');
    doAction('msg', () => tasksApi.message(task.id, msg));
  };

  const isPausable = task.status === 'in_progress';
  const isResumable = task.status === 'paused';
  const isTerminal = task.status === 'done' || task.status === 'cancelled';

  return (
    <div className={cn('rounded-xl border text-sm', statusColor)}>
      {/* Header row — always visible, click to expand */}
      <button
        type="button"
        onClick={() => !isTerminal && setExpanded(!expanded)}
        className={cn('w-full px-3 py-2.5 flex items-center justify-between gap-2 text-left', !isTerminal && 'cursor-pointer')}
      >
        <span className="font-medium text-xs leading-snug">
          {statusIcon} {task.title}
        </span>
        <div className="flex items-center gap-1.5 shrink-0">
          {task.status === 'in_progress' && (
            <span className="text-2xs text-muted-foreground tabular-nums">iter {task.iteration}</span>
          )}
          {!isTerminal && (
            <span className="text-2xs text-muted-foreground/50">{expanded ? '▲' : '▼'}</span>
          )}
        </div>
      </button>

      {/* Expanded detail */}
      {expanded && !isTerminal && (
        <div className="px-3 pb-3 space-y-2 border-t border-border/30 pt-2">
          {/* Current tool / last scratchpad line */}
          {task.last_tool && (
            <p className="text-2xs font-mono text-primary/60 truncate">▸ {task.last_tool}</p>
          )}
          {lastLine && !task.last_tool && (
            <p className="text-2xs text-muted-foreground/70 truncate">▸ {lastLine}</p>
          )}

          {/* Per-task pause / resume */}
          {(isPausable || isResumable) && (
            <div className="flex gap-1.5">
              {isPausable && (
                <button
                  onClick={() => doAction('pause', async () => {
                    await tasksApi.pause(task.id);
                    setLiveTask({ ...task, status: 'paused' });
                  })}
                  disabled={!!busy}
                  className="flex-1 rounded-md bg-yellow-500/10 px-2 py-1 text-2xs font-medium text-yellow-400 hover:bg-yellow-500/20 disabled:opacity-50"
                >
                  {busy === 'pause' ? <Loader2 className="h-3 w-3 animate-spin inline" /> : '⏸ Pause task'}
                </button>
              )}
              {isResumable && (
                <button
                  onClick={() => doAction('resume', async () => {
                    await tasksApi.resume(task.id);
                    setLiveTask({ ...task, status: 'in_progress' });
                  })}
                  disabled={!!busy}
                  className="flex-1 rounded-md bg-green-500/10 px-2 py-1 text-2xs font-medium text-green-400 hover:bg-green-500/20 disabled:opacity-50"
                >
                  {busy === 'resume' ? <Loader2 className="h-3 w-3 animate-spin inline" /> : '▶ Resume task'}
                </button>
              )}
            </div>
          )}

          {/* Per-task message */}
          <div className="flex gap-1.5">
            <input
              value={msgInput}
              onChange={e => setMsgInput(e.target.value)}
              onKeyDown={e => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); sendMessage(); } }}
              placeholder="Add context to this task"
              className="qr-input flex-1 text-xs h-6 py-0"
            />
            <button
              onClick={sendMessage}
              disabled={!!busy || !msgInput.trim()}
              className="h-6 w-6 flex items-center justify-center rounded-md bg-primary text-primary-foreground hover:bg-primary/90 disabled:opacity-40"
            >
              {busy === 'msg' ? <Loader2 className="h-3 w-3 animate-spin" /> : <Send className="h-3 w-3" />}
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
