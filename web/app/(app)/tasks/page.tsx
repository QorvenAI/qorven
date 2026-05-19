'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState, useCallback, useRef } from 'react';
import {
  Plus, Loader2, Filter, X, Send, MessageSquare, Bot, User,
  Circle, CircleDot, CircleCheck, CircleDashed, GitCommit, File, Ticket,
} from 'lucide-react';
import { CanvasHeader } from '@/components/layouts/canvas-header';
import { cn } from '@/lib/utils';
import { tasks as tasksApi } from '@/lib/api';
import { useStore } from '@/store';
import { toast } from 'sonner';
import { EmptyState, emptyStates } from '@/components/empty-state';
import { request } from '@/lib/api-core';

type Task = {
  id: string; title: string; description: string; state: string;
  assigned_agent_id: string | null; priority?: string;
  ticket_id?: string | null;
  iteration_count?: number;
  scratchpad?: string;
  created_at: string; started_at: string | null; completed_at: string | null;
};

type TaskFile = { id: string; path: string; operation: 'created' | 'modified' | 'deleted' };
type TaskComment = { id: string; author_type: 'user' | 'agent'; author_id: string; body: string; created_at: string };

const STATE_OPTS = [
  { value: '', label: 'All states' },
  { value: 'backlog', label: 'Backlog' },
  { value: 'todo', label: 'To Do' },
  { value: 'in_progress', label: 'In Progress' },
  { value: 'review', label: 'Review' },
  { value: 'done', label: 'Done' },
];

const PRIORITY_OPTS = [
  { value: '', label: 'All priorities' },
  { value: 'high', label: 'High' },
  { value: 'medium', label: 'Medium' },
  { value: 'low', label: 'Low' },
];

const STATE_DOT: Record<string, { icon: typeof Circle; cls: string }> = {
  backlog:     { icon: CircleDashed, cls: 'text-muted-foreground/40' },
  todo:        { icon: Circle,       cls: 'text-muted-foreground' },
  in_progress: { icon: CircleDot,   cls: 'text-blue-500' },
  review:      { icon: CircleDot,   cls: 'text-amber-500' },
  done:        { icon: CircleCheck, cls: 'text-emerald-500' },
};

const PRIORITY_CLS: Record<string, string> = {
  high:   'bg-destructive/10 text-destructive',
  medium: 'bg-amber-500/10 text-amber-500',
  low:    'bg-muted text-muted-foreground/60',
};

const OP_COLOR: Record<string, { text: string; symbol: string }> = {
  created:  { text: 'text-emerald-500', symbol: '+' },
  modified: { text: 'text-amber-500',   symbol: '±' },
  deleted:  { text: 'text-destructive', symbol: '−' },
};

function relativeTime(iso: string): string {
  const diff = Date.now() - new Date(iso).getTime();
  const m = Math.floor(diff / 60000);
  if (m < 1) return 'just now';
  if (m < 60) return `${m}m ago`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h}h ago`;
  return `${Math.floor(h / 24)}d ago`;
}


// ─── Task Drawer ──────────────────────────────────────────────────────────────

function TaskDrawer({ task, onClose, souls }: {
  task: Task;
  onClose: () => void;
  souls: import('@/types').Soul[];
}) {
  const [files, setFiles] = useState<TaskFile[]>([]);
  const [comments, setComments] = useState<TaskComment[]>([]);
  const [commentBody, setCommentBody] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [currentState, setCurrentState] = useState(task.state);
  const bottomRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    request<TaskFile[]>(`/tasks/${task.id}/files`).then(setFiles).catch(() => {});
    request<TaskComment[]>(`/tasks/${task.id}/comments`).then(setComments).catch(() => {});
  }, [task.id]);

  useEffect(() => { bottomRef.current?.scrollIntoView({ behavior: 'smooth' }); }, [comments]);

  const changeState = async (newState: string) => {
    setCurrentState(newState);
    await tasksApi.updateStatus(task.id, newState).catch(() => toast.error('Failed to update state'));
  };

  const submitComment = async () => {
    if (!commentBody.trim()) return;
    setSubmitting(true);
    try {
      await request<any>(`/tasks/${task.id}/comments`, {
        method: 'POST',
        body: JSON.stringify({ body: commentBody.trim() }),
      });
      setCommentBody('');
      request<TaskComment[]>(`/tasks/${task.id}/comments`).then(setComments).catch(() => {});
    } finally {
      setSubmitting(false);
    }
  };

  const { icon: DotIcon, cls: dotCls } = STATE_DOT[currentState] ?? STATE_DOT.todo!;
  const agent = souls.find(s => s.id === task.assigned_agent_id);

  return (
    <div className="fixed inset-y-0 right-0 z-40 flex w-[520px] flex-col border-l border-border bg-background shadow-2xl">
      {/* Header */}
      <div className="flex shrink-0 items-center gap-3 border-b border-border px-4 py-3">
        <DotIcon className={cn('h-4 w-4 shrink-0', dotCls)} />
        <h2 className="flex-1 text-sm font-semibold leading-snug">{task.title}</h2>
        <button onClick={onClose} className="rounded p-1 hover:bg-accent transition-colors">
          <X className="h-4 w-4" />
        </button>
      </div>

      {/* Meta row */}
      <div className="flex shrink-0 flex-wrap items-center gap-2 border-b border-border px-4 py-2.5">
        <select
          value={currentState}
          onChange={e => changeState(e.target.value)}
          className="rounded-full border border-border bg-transparent px-2.5 py-0.5 text-xs font-medium outline-none cursor-pointer capitalize"
        >
          {STATE_OPTS.filter(o => o.value).map(o => (
            <option key={o.value} value={o.value}>{o.label}</option>
          ))}
        </select>
        {task.priority && (
          <span className={cn('rounded-full px-2 py-0.5 text-xs font-medium capitalize', PRIORITY_CLS[task.priority] ?? PRIORITY_CLS.low)}>
            {task.priority}
          </span>
        )}
        {agent && (
          <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
            <div className="h-4 w-4 rounded-full bg-primary/20 flex items-center justify-center text-xs font-semibold text-primary">
              {agent.display_name[0]}
            </div>
            <span>{agent.display_name}</span>
          </div>
        )}
        {task.ticket_id && (
          <a href={`/code?tab=tickets`}
            className="flex items-center gap-1 text-xs text-primary/80 hover:text-primary transition-colors font-mono">
            <Ticket className="h-3.5 w-3.5" />
            {task.ticket_id.slice(0, 8)}
          </a>
        )}
        <span className="ml-auto text-xs text-muted-foreground/50">{relativeTime(task.created_at)}</span>
      </div>

      {/* Description */}
      {task.description && (
        <div className="shrink-0 border-b border-border px-4 py-3">
          <p className="text-xs text-muted-foreground leading-relaxed whitespace-pre-wrap">{task.description}</p>
        </div>
      )}

      {/* Scratchpad / working notes */}
      {task.scratchpad && task.scratchpad.trim() && (
        <div className="shrink-0 border-b border-border px-4 py-3">
          <p className="text-xs font-semibold uppercase tracking-wider text-muted-foreground mb-2">
            Working notes{task.iteration_count ? ` · Iteration ${task.iteration_count}` : ''}
          </p>
          <pre className="text-xs text-muted-foreground/80 leading-relaxed whitespace-pre-wrap max-h-48 overflow-y-auto rounded-lg bg-muted/30 border border-border p-2">
            {task.scratchpad}
          </pre>
        </div>
      )}

      {/* Changed files */}
      {files.length > 0 && (
        <div className="shrink-0 border-b border-border px-4 py-3">
          <div className="flex items-center gap-1.5 mb-2">
            <GitCommit className="h-3.5 w-3.5 text-muted-foreground/60" />
            <p className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">Changed files ({files.length})</p>
          </div>
          <div className="space-y-0.5 max-h-40 overflow-y-auto rounded-lg border border-border bg-muted/20 p-1.5">
            {files.map(f => {
              const op = OP_COLOR[f.operation] ?? OP_COLOR.modified!;
              return (
                <div key={f.id} className="flex items-center gap-2 rounded px-1.5 py-1 hover:bg-accent/50 transition-colors group cursor-pointer">
                  <span className={cn('shrink-0 w-3.5 text-center text-xs font-bold', op.text)}>{op.symbol}</span>
                  <File className="h-3 w-3 shrink-0 text-muted-foreground/50" />
                  <span className="flex-1 truncate font-mono text-xs">{f.path}</span>
                  <span className={cn('shrink-0 text-xs font-mono opacity-0 group-hover:opacity-100 transition-opacity', op.text)}>
                    {f.operation}
                  </span>
                </div>
              );
            })}
          </div>
        </div>
      )}

      {/* Comments */}
      <div className="flex-1 overflow-y-auto px-4 py-3 space-y-3">
        {comments.length === 0 ? (
          <div className="flex flex-col items-center justify-center h-full gap-2 text-center">
            <MessageSquare className="h-8 w-8 text-muted-foreground/20" />
            <p className="text-xs text-muted-foreground/60">No comments yet</p>
          </div>
        ) : (
          comments.map(c => (
            <div key={c.id} className={cn('flex gap-2.5', c.author_type === 'user' ? 'flex-row-reverse' : '')}>
              <div className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-muted mt-0.5">
                {c.author_type === 'agent'
                  ? <Bot className="h-3.5 w-3.5 text-primary" />
                  : <User className="h-3.5 w-3.5 text-muted-foreground" />}
              </div>
              <div className={cn('max-w-[85%] rounded-xl px-3 py-2 text-xs leading-relaxed whitespace-pre-wrap',
                c.author_type === 'user' ? 'bg-primary text-primary-foreground rounded-tr-sm' : 'bg-muted rounded-tl-sm')}>
                {c.body}
              </div>
            </div>
          ))
        )}
        <div ref={bottomRef} />
      </div>

      {/* Comment input */}
      <div className="shrink-0 border-t border-border px-3 py-2.5 flex items-end gap-2">
        <textarea
          value={commentBody}
          onChange={e => setCommentBody(e.target.value)}
          onKeyDown={e => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); submitComment(); } }}
          placeholder="Add a comment…"
          rows={2}
          className="qr-textarea flex-1 resize-none text-xs" />
        <button
          onClick={submitComment}
          disabled={!commentBody.trim() || submitting}
          className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-primary text-primary-foreground hover:bg-primary/90 transition-colors disabled:opacity-50"
        >
          {submitting ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Send className="h-3.5 w-3.5" />}
        </button>
      </div>
    </div>
  );
}

// ─── Main component ───────────────────────────────────────────────────────────

export default function TasksPage() {
  const [tasks, setTasks] = useState<Task[]>([]);
  const [loading, setLoading] = useState(true);
  const [stateFilter, setStateFilter] = useState('');
  const [priorityFilter, setPriorityFilter] = useState('');
  const [selected, setSelected] = useState<Task | null>(null);
  const [showCreate, setShowCreate] = useState(false);
  const [createTitle, setCreateTitle] = useState('');
  const [createDesc, setCreateDesc] = useState('');
  const [createPriority, setCreatePriority] = useState('medium');
  const [creating, setCreating] = useState(false);
  const taskAgentFilter = useStore(s => s.taskAgentFilter);
  const souls = useStore(s => s.souls);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const r: any = await tasksApi.list(taskAgentFilter ?? undefined).catch(() => []);
      let all: Task[] = Array.isArray(r) ? r : (r?.tasks ?? []);
      if (stateFilter) all = all.filter(t => t.state === stateFilter);
      if (priorityFilter) all = all.filter(t => t.priority === priorityFilter);
      const seen = new Set<string>();
      setTasks(all.filter(t => { if (seen.has(t.id)) return false; seen.add(t.id); return true; }));
    } finally {
      setLoading(false);
    }
  }, [taskAgentFilter, stateFilter, priorityFilter]);

  useEffect(() => { load(); }, [load]);

  // Real-time task updates via WebSocket
  useEffect(() => {
    const proto = window.location.protocol === 'https:' ? 'wss' : 'ws';
    const ws = new WebSocket(`${proto}://${window.location.host}/ws/realtime`);
    ws.onmessage = (e) => {
      try {
        const evt = JSON.parse(e.data);
        const d = evt.data ?? {};
        if (!d.task_id) return;
        if (['task_progress','task_iteration_start','task_tool_call'].includes(evt.type)) {
          setTasks(prev => prev.map(t => t.id !== d.task_id ? t : {
            ...t,
            state: 'in_progress',
            ...(d.iteration !== undefined ? { iteration_count: d.iteration } : {}),
            ...(d.scratchpad ? { scratchpad: d.scratchpad } : {}),
          }));
        } else if (evt.type === 'task_done') {
          setTasks(prev => prev.map(t => t.id !== d.task_id ? t : { ...t, state: 'done' }));
        } else if (evt.type === 'task_blocked') {
          setTasks(prev => prev.map(t => t.id !== d.task_id ? t : { ...t, state: 'blocked' }));
        }
      } catch {}
    };
    return () => ws.close();
  }, []);

  const createTask = async () => {
    if (!createTitle.trim()) return;
    setCreating(true);
    try {
      await tasksApi.create({ title: createTitle.trim(), description: createDesc.trim() || undefined, priority: createPriority });
      setCreateTitle(''); setCreateDesc(''); setCreatePriority('medium');
      setShowCreate(false);
      toast.success('Task created');
      load();
    } finally {
      setCreating(false);
    }
  };

  const counts = STATE_OPTS.filter(o => o.value).map(o => ({
    ...o, count: tasks.filter(t => t.state === o.value).length,
  }));

  return (
    <div className="flex flex-col h-full">
      <CanvasHeader title="Tasks" description="Track work across all your Qors" />
      {/* Toolbar */}
      <div className="flex shrink-0 items-center gap-2 border-b border-border px-4 py-2.5">
        <Filter className="h-4 w-4 text-muted-foreground shrink-0" />
        <select
          value={stateFilter} onChange={e => setStateFilter(e.target.value)}
          className="bg-transparent text-sm text-muted-foreground outline-none cursor-pointer"
        >
          {STATE_OPTS.map(o => <option key={o.value} value={o.value}>{o.label}</option>)}
        </select>
        <select
          value={priorityFilter} onChange={e => setPriorityFilter(e.target.value)}
          className="bg-transparent text-sm text-muted-foreground outline-none cursor-pointer"
        >
          {PRIORITY_OPTS.map(o => <option key={o.value} value={o.value}>{o.label}</option>)}
        </select>
        <div className="ml-auto">
          <button
            onClick={() => setShowCreate(!showCreate)}
            className="flex items-center gap-1.5 rounded-lg bg-primary px-3 py-1.5 text-xs font-semibold text-primary-foreground hover:bg-primary/90 transition-colors"
          >
            <Plus className="h-3.5 w-3.5" />New task
          </button>
        </div>
      </div>

      {/* Inline create form */}
      {showCreate && (
        <div className="shrink-0 border-b border-border bg-muted/20 px-4 py-3 space-y-2">
          <input
            autoFocus
            value={createTitle} onChange={e => setCreateTitle(e.target.value)}
            onKeyDown={e => { if (e.key === 'Enter' && !e.shiftKey) createTask(); if (e.key === 'Escape') setShowCreate(false); }}
            placeholder="Task title…"
            className="qr-input" />
          <textarea
            value={createDesc} onChange={e => setCreateDesc(e.target.value)}
            placeholder="Description (optional)…" rows={2}
            className="qr-textarea resize-none" />
          <div className="flex items-center gap-2">
            <select
              value={createPriority} onChange={e => setCreatePriority(e.target.value)}
              className="flex-1 rounded-lg border border-border bg-background px-2 py-1.5 text-xs outline-none"
            >
              {PRIORITY_OPTS.filter(p => p.value).map(p => <option key={p.value} value={p.value}>{p.label}</option>)}
            </select>
            <button onClick={createTask} disabled={!createTitle.trim() || creating}
              className="flex items-center gap-1.5 rounded-lg bg-primary px-3 py-1.5 text-xs font-semibold text-primary-foreground hover:bg-primary/90 disabled:opacity-50">
              {creating ? <Loader2 className="h-3 w-3 animate-spin" /> : <Plus className="h-3 w-3" />}Create
            </button>
            <button onClick={() => setShowCreate(false)} className="text-xs text-muted-foreground hover:text-foreground">Cancel</button>
          </div>
        </div>
      )}

      {/* Issues list */}
      <div className="flex-1 overflow-auto">
        {loading ? (
          <div className="flex items-center justify-center py-20">
            <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
          </div>
        ) : tasks.length === 0 ? (
          <EmptyState {...emptyStates.tasks} onAction={() => setShowCreate(true)} />
        ) : (
          <div>
            {/* State group tabs */}
            <div className="flex items-center gap-0 border-b border-border text-xs">
              {counts.map(c => {
                const { icon: Dot, cls } = STATE_DOT[c.value] ?? STATE_DOT.todo!;
                const active = stateFilter === c.value;
                return (
                  <button key={c.value}
                    onClick={() => setStateFilter(active ? '' : c.value)}
                    className={cn('flex items-center gap-1.5 px-3 py-2 border-b-2 transition-colors',
                      active
                        ? 'border-foreground text-foreground font-medium'
                        : 'border-transparent text-muted-foreground hover:text-foreground')}>
                    <Dot className={cn('h-3 w-3', cls)} />
                    {c.label}
                    {c.count > 0 && (
                      <span className="ml-0.5 rounded-full bg-muted px-1.5 py-px text-muted-foreground text-[10px]">{c.count}</span>
                    )}
                  </button>
                );
              })}
            </div>

            {/* Issue rows */}
            <div className="divide-y divide-border/50">
              {tasks.map(task => {
                const { icon: DotIcon, cls: dotCls } = STATE_DOT[task.state] ?? STATE_DOT.todo!;
                const agent = souls.find(s => s.id === task.assigned_agent_id);
                return (
                  <div
                    key={task.id}
                    onClick={() => setSelected(task)}
                    className="flex items-start gap-3 px-4 py-3 hover:bg-accent/30 transition-colors cursor-pointer group"
                  >
                    <DotIcon className={cn('h-4 w-4 mt-0.5 shrink-0', dotCls)} />

                    <div className="flex-1 min-w-0">
                      <div className="flex items-baseline gap-2">
                        <span className="text-sm font-medium leading-snug group-hover:text-primary transition-colors">
                          {task.title}
                        </span>
                        {task.priority && task.priority !== 'medium' && (
                          <span className={cn('rounded-full px-1.5 py-px text-[10px] font-medium capitalize shrink-0', PRIORITY_CLS[task.priority] ?? PRIORITY_CLS.low)}>
                            {task.priority}
                          </span>
                        )}
                        {task.state === 'in_progress' && (
                          <span className="relative flex h-2 w-2 ml-1 inline-flex">
                            <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-blue-400 opacity-75" />
                            <span className="relative inline-flex rounded-full h-2 w-2 bg-blue-500" />
                          </span>
                        )}
                      </div>
                      {task.description && (
                        <p className="mt-0.5 text-xs text-muted-foreground/70 line-clamp-1">{task.description}</p>
                      )}
                    </div>

                    <div className="flex items-center gap-3 shrink-0 ml-auto">
                      {agent && (
                        <div className="flex items-center gap-1.5">
                          <div className="h-5 w-5 rounded-full bg-primary/20 flex items-center justify-center text-xs font-semibold text-primary">
                            {agent.display_name[0]}
                          </div>
                          <span className="text-xs text-muted-foreground hidden sm:block">{agent.display_name}</span>
                        </div>
                      )}
                      <span className="text-xs text-muted-foreground/50 tabular-nums">
                        {relativeTime(task.created_at)}
                      </span>
                    </div>
                  </div>
                );
              })}
            </div>
          </div>
        )}
      </div>

      {/* Drawer */}
      {selected && (
        <>
          <div className="fixed inset-0 z-30 bg-black/20" onClick={() => setSelected(null)} />
          <TaskDrawer task={selected} souls={souls} onClose={() => setSelected(null)} />
        </>
      )}
    </div>
  );
}
