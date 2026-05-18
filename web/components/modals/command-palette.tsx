'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useState, useEffect, useRef, useCallback } from 'react';
import { useRouter } from 'next/navigation';
import { useStore } from '@/store';
import {
  Search, Home, MessageSquare, Code, Mail, CalendarDays, CheckSquare,
  HardDrive, Link2, GitBranch, Users, Sparkles, Plug, Share2, Activity,
  Cpu, Settings, Bot, BarChart3, Bell, Layers, ShieldCheck, Database,
  Wand2, Mic, ListTodo, FlaskConical, BookOpen, Ticket, File, Brain,
  ShieldAlert, Network, Beaker, Terminal, ListChecks, Waypoints, Radar,
  ClipboardList, Goal, FolderOpen,
} from 'lucide-react';
import { brand, placeholders } from '@/lib/branding';
import { request } from '@/lib/api-core';

type Result = { label: string; sub: string; href: string; icon: React.ElementType };

const allPages: Result[] = [
  { label: 'Dashboard', sub: 'Page', href: '/', icon: Home },
  { label: brand.agentNamePlural, sub: 'Page', href: '/qors', icon: Bot },
  { label: 'Code', sub: 'Page', href: '/code', icon: Code },
  { label: 'Chat / Mail', sub: 'Page', href: '/mail', icon: Mail },
  { label: 'Calendar', sub: 'Page', href: '/schedule', icon: CalendarDays },
  { label: 'Tasks', sub: 'Page', href: '/tasks', icon: CheckSquare },
  { label: 'Drive', sub: 'Page', href: '/drive', icon: HardDrive },
  { label: 'Channels', sub: 'Page', href: '/channels', icon: Link2 },
  { label: 'Workflows', sub: 'Page', href: '/workflows', icon: GitBranch },
  { label: 'Teams', sub: 'Page', href: '/teams', icon: Users },
  { label: 'Skills', sub: 'Apps', href: '/apps', icon: Sparkles },
  { label: 'MCP Servers', sub: 'Page', href: '/mcp', icon: Plug },
  { label: 'Knowledge Graph', sub: 'Page', href: '/knowledge-graph', icon: Share2 },
  { label: 'Health', sub: 'Page', href: '/heartbeat', icon: Activity },
  { label: 'Models Hub', sub: 'Page', href: '/models-hub', icon: Cpu },
  { label: 'Settings', sub: 'Page', href: '/settings', icon: Settings },
  { label: 'Analytics', sub: 'Page', href: '/analytics', icon: BarChart3 },
  { label: 'Notifications', sub: 'Page', href: '/notifications', icon: Bell },
  { label: 'Blueprints', sub: 'Apps', href: '/apps', icon: Layers },
  { label: 'Audit Log', sub: 'Page', href: '/audit', icon: ShieldCheck },
  { label: 'Provider Keys', sub: 'Page', href: '/provider-keys', icon: Database },
  { label: 'Memories', sub: 'Page', href: '/memories', icon: BookOpen },
  { label: 'Research', sub: 'Page', href: '/research', icon: Wand2 },
  { label: 'Hubs', sub: 'Chat', href: '/qors', icon: MessageSquare },
  { label: 'Voice', sub: 'Page', href: '/voice', icon: Mic },
  { label: 'Tasks (Global)', sub: 'Page', href: '/tasks', icon: ListTodo },
  { label: 'Sandbox', sub: 'Page', href: '/sandbox', icon: FlaskConical },
  { label: 'Labs', sub: 'Page', href: '/labs', icon: Beaker },
  { label: 'Scenarios', sub: 'Page', href: '/scenarios', icon: FlaskConical },
  { label: 'Org Chart', sub: 'Page', href: '/org-chart', icon: Users },
  { label: 'Supervisor', sub: 'Page', href: '/supervisor', icon: ShieldAlert },
  { label: 'Traces', sub: 'Page', href: '/traces', icon: Radar },
  { label: 'Terminal', sub: 'Page', href: '/terminal', icon: Terminal },
  { label: 'Plans', sub: 'Page', href: '/plans', icon: ClipboardList },
  { label: 'Approvals', sub: 'Page', href: '/approvals', icon: ListChecks },
  { label: 'Pairing', sub: 'Page', href: '/pairing', icon: Network },
  { label: 'Training', sub: 'Page', href: '/training', icon: Goal },
  { label: 'A2A', sub: 'Page', href: '/a2a', icon: Waypoints },
];

interface SessionResult { id: string; agent_name?: string; preview?: string; channel?: string }
interface TicketResult  { id: string; title: string; slug?: string; status?: string }
interface TaskResult    { id: string; title: string; status?: string; priority?: string }
interface MemoryResult  { id: string; content: string; scope?: string; agent_id?: string }
interface DriveResult   { id: string; name: string; mime_type?: string; is_folder?: boolean }

// All 5 search categories run in parallel with a single AbortController so
// navigating away or typing new chars kills all in-flight fetches at once.
interface MultiResults {
  sessions: SessionResult[];
  tickets:  TicketResult[];
  tasks:    TaskResult[];
  memories: MemoryResult[];
  drive:    DriveResult[];
}

const EMPTY_MULTI: MultiResults = { sessions: [], tickets: [], tasks: [], memories: [], drive: [] };

export function CommandPalette() {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState('');
  const [activeIdx, setActiveIdx] = useState(0);
  const [multi, setMulti] = useState<MultiResults>(EMPTY_MULTI);
  const inputRef = useRef<HTMLInputElement>(null);
  const searchTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const abortRef = useRef<AbortController | null>(null);
  const router = useRouter();
  const souls = useStore((s) => s.souls);

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 'k') { e.preventDefault(); setOpen(o => !o); }
      if (e.key === 'Escape') setOpen(false);
    };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, []);

  useEffect(() => {
    if (open) { setTimeout(() => inputRef.current?.focus(), 50); setQuery(''); setActiveIdx(0); setMulti(EMPTY_MULTI); }
  }, [open]);

  // Debounced parallel search across all 5 categories.
  const runSearch = useCallback((q: string) => {
    const enc = encodeURIComponent(q);
    const ctrl = new AbortController();
    abortRef.current = ctrl;
    const sig = { signal: ctrl.signal };

    Promise.allSettled([
      request<{ sessions: SessionResult[] }>(`/sessions/search?q=${enc}`, sig),
      request<TicketResult[] | { tickets?: TicketResult[] }>(`/tickets?q=${enc}`, sig),
      request<{ tasks: TaskResult[] }>(`/tasks?q=${enc}`, sig),
      request<{ memories: MemoryResult[] }>(`/memory/search?q=${enc}&max_results=5`, sig),
      request<DriveResult[]>(`/drive/files?q=${enc}`, sig),
    ]).then((results) => {
      if (abortRef.current !== ctrl) return;
      const [sessRes, tickRes, taskRes, memRes, driveRes] = results;

      const sessions = sessRes.status === 'fulfilled' ? (sessRes.value.sessions ?? []).slice(0, 3) : [];
      const tickData = tickRes.status === 'fulfilled' ? tickRes.value : null;
      const tickets  = Array.isArray(tickData) ? tickData.slice(0, 3) : ((tickData as any)?.tickets ?? []).slice(0, 3);
      const tasks    = taskRes.status === 'fulfilled' ? (taskRes.value.tasks ?? []).slice(0, 3) : [];
      const memories = memRes.status === 'fulfilled' ? (memRes.value.memories ?? []).slice(0, 3) : [];
      const drive    = driveRes.status === 'fulfilled' ? (Array.isArray(driveRes.value) ? driveRes.value : []).slice(0, 3) : [];

      setMulti({ sessions, tickets, tasks, memories, drive });
    });
  }, []);

  useEffect(() => {
    if (searchTimer.current) clearTimeout(searchTimer.current);
    abortRef.current?.abort();
    if (query.trim().length < 2) { setMulti(EMPTY_MULTI); return; }
    searchTimer.current = setTimeout(() => runSearch(query.trim()), 300);
    return () => {
      if (searchTimer.current) clearTimeout(searchTimer.current);
      abortRef.current?.abort();
    };
  }, [query, runSearch]);

  const q = query.toLowerCase();

  const soulResults: Result[] = souls
    .filter((s) => s.display_name.toLowerCase().includes(q) || s.role?.toLowerCase().includes(q))
    .slice(0, 5)
    .map((s) => ({ label: s.display_name, sub: s.role || brand.agentName, href: `/qors/${s.id}`, icon: Bot }));

  const pageResults: Result[] = q
    ? allPages.filter((p) => p.label.toLowerCase().includes(q) || p.href.includes(q)).slice(0, 8)
    : allPages.slice(0, 6);

  const sessionResults: Result[] = multi.sessions.map((s) => ({
    label: s.preview?.slice(0, 60) || `Session ${s.id.slice(0, 8)}`,
    sub: s.agent_name || s.channel || 'chat',
    href: `/qors?session=${s.id}`,
    icon: MessageSquare,
  }));

  const ticketResults: Result[] = multi.tickets.map((t) => ({
    label: t.title,
    sub: t.status || 'ticket',
    href: `/code?ticket=${t.id}`,
    icon: Ticket,
  }));

  const taskResults: Result[] = multi.tasks.map((t) => ({
    label: t.title,
    sub: t.status || 'task',
    href: `/tasks?id=${t.id}`,
    icon: CheckSquare,
  }));

  const memoryResults: Result[] = multi.memories.map((m) => ({
    label: m.content.slice(0, 60),
    sub: m.scope || 'memory',
    href: `/memories`,
    icon: Brain,
  }));

  const driveResults: Result[] = multi.drive.map((f) => ({
    label: f.name,
    sub: f.is_folder ? 'folder' : (f.mime_type?.split('/')[1] || 'file'),
    href: `/drive`,
    icon: f.is_folder ? FolderOpen : File,
  }));

  const results: Result[] = q
    ? [...soulResults, ...sessionResults, ...ticketResults, ...taskResults, ...memoryResults, ...driveResults, ...pageResults].slice(0, 18)
    : [...soulResults.slice(0, 4), ...pageResults];

  const navigate = (r: Result) => { router.push(r.href); setOpen(false); setQuery(''); };

  const handleKey = (e: React.KeyboardEvent) => {
    if (e.key === 'ArrowDown') { e.preventDefault(); setActiveIdx(i => Math.min(i + 1, results.length - 1)); }
    if (e.key === 'ArrowUp') { e.preventDefault(); setActiveIdx(i => Math.max(i - 1, 0)); }
    if (e.key === 'Enter' && results[activeIdx]) navigate(results[activeIdx]);
  };

  if (!open) return null;

  // Render each section with its own header, calculating the global index offset.
  const sections: Array<{ label: string; items: Result[] }> = [];
  if (soulResults.length > 0)   sections.push({ label: brand.agentNamePlural, items: soulResults });
  if (sessionResults.length > 0) sections.push({ label: 'Sessions', items: sessionResults });
  if (ticketResults.length > 0)  sections.push({ label: 'Tickets', items: ticketResults });
  if (taskResults.length > 0)    sections.push({ label: 'Tasks', items: taskResults });
  if (memoryResults.length > 0)  sections.push({ label: 'Memories', items: memoryResults });
  if (driveResults.length > 0)   sections.push({ label: 'Drive', items: driveResults });
  if (pageResults.length > 0)    sections.push({ label: q ? 'Pages' : 'Quick Nav', items: pageResults });

  let globalIdx = 0;

  return (
    <div className="fixed inset-0 z-[100] flex items-start justify-center pt-[18vh] bg-black/60 backdrop-blur-sm" onClick={() => setOpen(false)}>
      <div className="w-full max-w-lg rounded-xl border border-border bg-popover shadow-2xl overflow-hidden" onClick={(e) => e.stopPropagation()}>
        {/* Input */}
        <div className="flex items-center gap-2.5 border-b border-border px-4 py-3">
          <Search className="h-4 w-4 text-muted-foreground shrink-0" />
          <input
            ref={inputRef}
            value={query}
            onChange={(e) => { setQuery(e.target.value); setActiveIdx(0); }}
            onKeyDown={handleKey}
            placeholder={placeholders.globalSearch}
            className="flex-1 bg-transparent text-sm outline-none placeholder:text-muted-foreground"
          />
          <kbd className="rounded border border-border bg-muted px-1.5 py-0.5 text-xs text-muted-foreground">ESC</kbd>
        </div>

        {/* Results */}
        <div className="max-h-80 overflow-y-auto py-1">
          {sections.map(({ label, items }) => (
            <div key={label}>
              <p className="px-4 py-1.5 text-xs font-semibold uppercase tracking-wider text-muted-foreground">{label}</p>
              {items.map((r) => {
                const idx = globalIdx++;
                return <ResultItem key={`${label}-${r.href}-${idx}`} r={r} active={idx === activeIdx} onSelect={() => navigate(r)} />;
              })}
            </div>
          ))}

          {query && results.length === 0 && (
            <p className="py-6 text-center text-sm text-muted-foreground">No results for &quot;{query}&quot;</p>
          )}
        </div>

        {/* Footer */}
        <div className="flex items-center gap-3 border-t border-border px-4 py-2 text-xs text-muted-foreground">
          <span><kbd className="font-mono">↑↓</kbd> navigate</span>
          <span><kbd className="font-mono">↵</kbd> open</span>
          <span><kbd className="font-mono">ESC</kbd> close</span>
          <span className="ml-auto"><kbd className="font-mono">⌘K</kbd> toggle</span>
        </div>
      </div>
    </div>
  );
}

function ResultItem({ r, active, onSelect }: { r: Result; active: boolean; onSelect: () => void }) {
  const Icon = r.icon;
  return (
    <button
      onClick={onSelect}
      className={`flex w-full items-center gap-3 px-4 py-2 text-left text-sm transition-colors ${active ? 'bg-accent text-accent-foreground' : 'hover:bg-accent/50'}`}
    >
      <Icon className="h-4 w-4 text-muted-foreground shrink-0" />
      <span className="font-medium flex-1">{r.label}</span>
      <span className="text-xs text-muted-foreground rounded border border-border px-1.5 py-0.5">{r.sub}</span>
    </button>
  );
}
