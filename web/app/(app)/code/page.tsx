'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useState, useEffect, useRef, useCallback, useMemo } from 'react';
import { useSearchParams, useRouter } from 'next/navigation';
import { useToolbarContent } from '@/hooks/use-toolbar-content';
import { useStore } from '@/store';
import { cn } from '@/lib/utils';
import { TicketsTab } from './tabs/tickets';
import { OrgTab } from './tabs/org';
import { GoalsTab } from './tabs/goals';
import { InceptionTab } from './tabs/inception';
import GitHubTab from '@/app/(app)/github/page';
import TasksTab from '@/app/(app)/tasks/page';
import WorkflowsTab from '@/app/(app)/workflows/page';
import PlansTab from '@/app/(app)/plans/page';
import { InboxTab } from './tabs/inbox';
import { ActivityFeed } from './tabs/activity';
import { BottomDrawerTab } from '@/components/layouts/qorven/bottom-drawer';
import { AgentDashboard } from '@/components/agents/AgentDashboard';
import { TaskFeed } from '@/components/agents/TaskFeed';
import { useAgentsStream } from '@/hooks/use-agents-stream';
import { ensureCanonicalSessionId } from '@/lib/session';
import {
  File, Search, Terminal, GitBranch,
  Loader2, X, Code, GitCommit,
  Play, CheckCircle2, AlertCircle, Zap,
  Ticket, Target, Network, Lightbulb, FolderOpen, Plus,
  GitPullRequest, CheckSquare, Workflow, ShieldCheck,
} from 'lucide-react';

import { apiBase, wsBase } from '@/lib/api-url';
import { getToken, request as apiRequest, fetchWithRetry, BASE as API_BASE } from '@/lib/api-core';
import { detectLang, fileColor } from '@/components/code/code-utils';
import { TreeNode } from '@/components/code/tree-node';
import { EditorTabBar } from '@/components/code/editor-tab-bar';
import { CodeEditor } from '@/components/code/code-editor';
import { TerminalPane } from '@/components/code/terminal-pane';
import { BuildLog } from '@/components/code/build-log';
import { ProjectDiscoveryChat } from '@/components/code/project-discovery-chat';
import { CodeChatSidebar } from '@/components/code/code-chat-sidebar';
import type { FileNode, FileTab, ChatMsg, CodeProject, BuildEntry } from '@/components/code/code-types';

async function apiFetch(endpoint: string, options?: RequestInit) {
  return fetchWithRetry(`${API_BASE}${endpoint}`, {
    ...options,
    headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${getToken()}`, ...options?.headers },
  });
}

async function apiPost(endpoint: string, body: unknown) {
  return apiRequest<unknown>(endpoint, { method: 'POST', body: JSON.stringify(body) });
}

const CODE_TABS = [
  { id: 'editor',    label: 'Editor',    icon: Code },
  { id: 'tickets',   label: 'Tickets',   icon: Ticket },
  { id: 'tasks',     label: 'Tasks',     icon: CheckSquare },
  { id: 'github',    label: 'GitHub',    icon: GitPullRequest },
  { id: 'workflows', label: 'Flows',     icon: Workflow },
  { id: 'plans',     label: 'Plans',     icon: GitBranch },
  { id: 'inbox',     label: 'Inbox',     icon: ShieldCheck },
  { id: 'org',       label: 'Org',       icon: Network },
  { id: 'goals',     label: 'Goals',     icon: Target },
  { id: 'inception', label: 'Inception', icon: Lightbulb },
] as const;
type CodeTabId = (typeof CODE_TABS)[number]['id'];

export default function CodePage() {
  const searchParams = useSearchParams();
  const router = useRouter();
  const activeCodeTab = (searchParams.get('tab') as CodeTabId) || 'editor';

  const [activeProject, setActiveProject] = useState<CodeProject | null>(null);

  const codeTabBar = useMemo(() => (
    <nav className="flex items-stretch gap-0">
      {CODE_TABS.map(t => {
        const Icon = t.icon;
        return (
          <button key={t.id} onClick={() => router.push(`/code?tab=${t.id}`)}
            className={cn('flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium border-b-2 whitespace-nowrap transition-colors',
              activeCodeTab === t.id
                ? 'border-primary text-foreground bg-background'
                : 'border-transparent text-muted-foreground hover:text-foreground')}>
            <Icon className="h-3.5 w-3.5 shrink-0" />
            {t.label}
          </button>
        );
      })}
    </nav>
  ), [activeCodeTab, router]);
  useToolbarContent(activeProject ? codeTabBar : null);

  const [tree, setTree] = useState<FileNode[]>([]);
  const [tabs, setTabs] = useState<FileTab[]>([]);
  const [activeTab, setActiveTab] = useState('');
  const [changes, setChanges] = useState<any[]>([]);
  const [messages, setMessages] = useState<ChatMsg[]>([{
    role: 'assistant', content: "Hey! I'm Prime Coder. Describe your project and I'll build it, or open an existing one from the sidebar.",
  }]);
  const [isLoading, setIsLoading] = useState(false);
  const [projectPath, setProjectPath] = useState('');
  const [projects, setProjects] = useState<CodeProject[]>([]);
  const termOpen = useStore((s) => s.codeTermOpen);
  const setTermOpen = useStore((s) => s.setCodeTermOpen);
  const [quickOpen, setQuickOpen] = useState(false);
  const [quickQuery, setQuickQuery] = useState('');
  const openBottomDrawer = useStore((s) => s.openBottomDrawer);
  useAgentsStream();
  const pendingAgentPlans = useStore(s =>
    Object.values(s.daemonPlans).filter(p => p.status === 'pending').length
  );
  const [buildLog, setBuildLog] = useState<BuildEntry[]>([]);
  const [buildRunning, setBuildRunning] = useState(false);
  const [buildPhase, setBuildPhase] = useState<string>('');
  const [buildPlan, setBuildPlan] = useState<any>(null);
  const [buildPlanEdited, setBuildPlanEdited] = useState<any>(null);
  const [previewUrl, setPreviewUrl] = useState<string>('');
  const [buildPrUrl, setBuildPrUrl] = useState<string>('');
  const [buildStartTime, setBuildStartTime] = useState<number>(0);
  const [buildSummary, setBuildSummary] = useState<{ files: number; agents: number; prUrl?: string; previewUrl?: string; elapsed?: string } | null>(null);
  const [agentStatus, setAgentStatus] = useState<Record<string, 'working' | 'done' | 'error'>>({});
  const [codeThinkingLevel, setCodeThinkingLevel] = useState<'off' | 'medium' | 'high'>('off');
  // Mirror refs kept in sync with the 4 state values that parseSSEStream reads
  // inside an async callback where closure-captured state would be stale.
  const buildStartTimeRef = useRef<number>(0);
  const previewUrlRef = useRef<string>('');
  const buildPrUrlRef = useRef<string>('');
  const agentStatusRef = useRef<Record<string, 'working' | 'done' | 'error'>>({});
  const buildAbortRef = useRef<AbortController | null>(null);
  const chatAbortRef = useRef<AbortController | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  // Holds the cleanup for the per-build WS subscription opened by approveBuild.
  // Stored in a ref so the useEffect below can close it on unmount regardless
  // of whether the build succeeded or failed.
  const rtHubCleanupRef = useRef<(() => void) | null>(null);

  // Wrapped setters that keep the mirror refs current so parseSSEStream /
  // subscribeToRtHub can read the latest values without stale closure issues.
  const setPreviewUrlSync = useCallback((url: string) => {
    previewUrlRef.current = url;
    setPreviewUrl(url);
  }, []);
  const setBuildPrUrlSync = useCallback((url: string) => {
    buildPrUrlRef.current = url;
    setBuildPrUrl(url);
  }, []);
  const setBuildStartTimeSync = useCallback((t: number) => {
    buildStartTimeRef.current = t;
    setBuildStartTime(t);
  }, []);
  const setAgentStatusSync = useCallback((fn: (prev: Record<string, 'working' | 'done' | 'error'>) => Record<string, 'working' | 'done' | 'error'>) => {
    setAgentStatus(prev => {
      const next = fn(prev);
      agentStatusRef.current = next;
      return next;
    });
  }, []);

  const sessionId = useRef('');
  const getSessionId = useCallback(
    () => ensureCanonicalSessionId(sessionId, { agentId: 'prime', channel: 'code' }),
    [],
  );

  const setCodeProjectName = useStore(s => s.setCodeProjectName);
  const setStoreProjects = useStore(s => s.setCodeProjects);
  const setStoreTree = useStore(s => s.setCodeTree);
  const setStoreProjectPath = useStore(s => s.setCodeProjectPath);
  const setStoreActiveProjectId = useStore(s => s.setCodeActiveProjectId);
  const storeActiveProjectId = useStore(s => s.codeActiveProjectId);

  const loadProjects = useCallback(async () => {
    const d = await apiRequest<unknown>('/projects').catch(() => []);
    if (Array.isArray(d)) { setProjects(d); setStoreProjects(d); }
  }, [setStoreProjects]);

  useEffect(() => { loadProjects(); }, [loadProjects]);

  // Close the per-build WS subscription and any in-flight chat stream on unmount.
  useEffect(() => () => {
    rtHubCleanupRef.current?.();
    chatAbortRef.current?.abort();
  }, []);

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key === '`') { e.preventDefault(); setTermOpen(!useStore.getState().codeTermOpen); }
      if ((e.ctrlKey || e.metaKey) && e.key === 'p') { e.preventDefault(); setQuickOpen(v => !v); setQuickQuery(''); }
      if (e.key === 'Escape') setQuickOpen(false);
      if ((e.ctrlKey || e.metaKey) && e.key === 's') {
        e.preventDefault();
        setTabs(prev => {
          const tab = prev.find(t => t.path === activeTab);
          if (!tab || !tab.dirty) return prev;
          const pid = useStore.getState().codeActiveProjectId;
          const save = async () => {
            if (pid) {
              const res = await apiFetch(`/projects/${pid}/file`, {
                method: 'PUT',
                body: JSON.stringify({ path: tab.path, content: tab.content }),
              });
              if (res.ok) {
                setTabs(ts => ts.map(t => t.path === tab.path ? { ...t, dirty: false } : t));
                return;
              }
            }
            const sid = await getSessionId();
            await apiPost('/chat/completions', {
              agent_id: 'prime', session_id: sid, stream: false,
              message: `Use write_file to save the file at path "${tab.path}" with this exact content:\n\n${tab.content}`,
            });
            setTabs(ts => ts.map(t => t.path === tab.path ? { ...t, dirty: false } : t));
          };
          save().catch(() => {});
          return prev;
        });
      }
    };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [activeTab]);

  const parseTree = (text: string, base: string): FileNode[] => {
    const nodes: FileNode[] = [];
    for (const line of text.split('\n').filter(l => l.trim())) {
      const name = line.trim().replace(/\/$/, '');
      if (!name || name.startsWith('.') || name === 'node_modules' || name === 'vendor') continue;
      const isDir = line.trim().endsWith('/');
      nodes.push({ name, path: `${base}/${name}`, type: isDir ? 'dir' : 'file' });
    }
    return nodes.sort((a, b) => a.type === b.type ? a.name.localeCompare(b.name) : a.type === 'dir' ? -1 : 1);
  };

  const loadProjectTree = useCallback(async (path: string, projectId?: string) => {
    // Prefer explicit id; fall back to store (not closure) to avoid stale activeProject.
    const id = projectId || useStore.getState().codeActiveProjectId;
    if (!id) return;
    try {
      const res = await apiFetch(`/projects/${id}/tree`);
      if (res.ok) {
        const data = await res.json();
        const nodes = data.tree || [];
        setTree(nodes); setStoreTree(nodes); setStoreProjectPath(path);
        return;
      }
    } catch {}
    try {
      const sid = await getSessionId();
      const data = await apiPost('/chat/completions', {
        agent_id: 'prime', session_id: sid, stream: false,
        message: `Use list_files to list the contents of ${path}. Return ONLY file/directory names one per line, directories end with /.`,
      }) as any;
      const parsed = parseTree(data?.choices?.[0]?.message?.content || '', path);
      setTree(parsed); setStoreTree(parsed); setStoreProjectPath(path);
    } catch {}
  }, [setStoreTree, setStoreProjectPath]);

  const switchProject = useCallback((p: CodeProject) => {
    setActiveProject(p); setProjectPath(p.path);
    setCodeProjectName(p.name); sessionId.current = p.session_id;
    setStoreActiveProjectId(p.id);
    setMessages([{ role: 'assistant', content: `Switched to **${p.name}**. The workspace is at \`${p.path}\`.` }]);
    setTabs([]); setActiveTab(''); setChanges([]);
    setBuildLog([]); setBuildRunning(false);
    loadProjectTree(p.path, p.id);
  }, [loadProjectTree, setCodeProjectName, setStoreActiveProjectId]);

  useEffect(() => {
    if (storeActiveProjectId && storeActiveProjectId !== activeProject?.id) {
      const p = projects.find(p => p.id === storeActiveProjectId);
      if (p) switchProject(p);
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [storeActiveProjectId]);

  const parseSSEStream = useCallback(async (
    reader: ReadableStreamDefaultReader<Uint8Array>,
    projectPath: string,
    onPlanReady?: (plan: any) => void,
  ) => {
    const decoder = new TextDecoder();
    const seen = new Set<string>();
    const fingerprint = (evt: any): string => {
      if (evt.id) return String(evt.id);
      const p = evt.properties ?? evt.data ?? {};
      try { return (evt.type ?? '') + '|' + JSON.stringify(p).slice(0, 128); } catch { return evt.type ?? ''; }
    };
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      for (const line of decoder.decode(value, { stream: true }).split('\n')) {
        const raw = line.replace(/^data: /, '').trim();
        if (!raw || raw === '[DONE]') continue;
        try {
          const evt = JSON.parse(raw);
          const fp = fingerprint(evt);
          if (seen.has(fp)) continue;
          seen.add(fp);
          // All events are canonical envelopes: { type, properties }.
          const t: string = evt.type ?? '';
          const d: any = evt.properties ?? {};
          switch (t) {
            case 'build.phase':
              setBuildPhase(d.phase || '');
              if (d.preview_url) setPreviewUrlSync(d.preview_url);
              break;
            case 'message.part.updated': {
              const delta: string = (() => { try { return JSON.parse(d.payload ?? '{}').delta ?? ''; } catch { return ''; } })();
              if (!delta) break;
              setBuildLog(prev => {
                const last = prev[prev.length - 1];
                if (last?.type === 'text') return [...prev.slice(0, -1), { ...last, content: last.content + delta }];
                return [...prev, { type: 'text', content: delta }];
              });
              break;
            }
            case 'plan.proposed':
              setBuildRunning(false);
              setBuildPhase('pending_approval');
              if (d.plan) { setBuildPlan(d.plan); onPlanReady?.(d.plan); }
              break;
            case 'agent.progress': {
              const detail = d.detail ?? {};
              if (d.kind === 'tool_start') {
                setBuildLog(prev => [...prev, { type: 'tool_start', tool: detail.name || detail.tool || '', content: detail.input ? JSON.stringify(detail.input).slice(0, 100) + '…' : '', ts: Date.now() }]);
              } else if (d.kind === 'file_created') {
                const fp2 = detail.path || '';
                setBuildLog(prev => [...prev, {
                  type: 'file_chip',
                  path: fp2,
                  content: fp2,
                  linesAdded: detail.lines_added ?? 0,
                  linesRemoved: detail.lines_removed ?? 0,
                  totalLines: detail.total_lines,
                }]);
                setChanges(prev => prev.some(c => c.path === fp2) ? prev : [...prev, { path: fp2, action: 'created' }]);
                if (fp2) {
                  setTree(prev => {
                    const name = fp2.split('/').pop() || fp2;
                    if (prev.some(n => n.path === fp2)) return prev;
                    return [...prev, { name, path: fp2, type: 'file' as const }].sort((a, b) =>
                      a.type === b.type ? a.name.localeCompare(b.name) : a.type === 'dir' ? -1 : 1
                    );
                  });
                }
              } else if (d.kind === 'text') {
                const label = `[${d.agent_key}] `;
                setBuildLog(prev => {
                  const last = prev[prev.length - 1];
                  if (last?.type === 'text' && last.content.startsWith(label)) {
                    return [...prev.slice(0, -1), { ...last, content: last.content + (detail.delta || '') }];
                  }
                  return [...prev, { type: 'text', content: label + (detail.delta || '') }];
                });
              }
              break;
            }
            case 'agent.started':
              setAgentStatusSync(prev => ({ ...prev, [d.agent_key || '']: 'working' }));
              setBuildLog(prev => [...prev, { type: 'tool_start', tool: d.agent_key || '', content: `${d.role || ''} started`, ts: Date.now() }]);
              break;
            case 'agent.completed':
              setAgentStatusSync(prev => ({ ...prev, [d.agent_key || '']: 'done' }));
              setBuildLog(prev => [...prev, { type: 'file_created', path: '', content: `✓ ${d.agent_key || ''} finished` }]);
              break;
            case 'github.pr_opened': {
              const url = d.html_url || '';
              if (url) setBuildPrUrlSync(url);
              setBuildLog(prev => [...prev, {
                type: 'pr_card',
                content: '',
                prUrl: url || undefined,
                prTitle: d.title || undefined,
                prNumber: d.number || undefined,
                prRepo: d.repo || undefined,
              }]);
              break;
            }
            case 'github.ci_status':
              if (d.status === 'completed' && d.conclusion === 'failure') {
                setBuildLog(prev => [...prev, { type: 'error', content: `❌ CI failed on PR #${d.pr_number} — check GitHub` }]);
              } else {
                setBuildLog(prev => [...prev, { type: 'tool_start', tool: '🔄 CI', content: `Watching PR #${d.pr_number} in ${d.repo}` }]);
              }
              break;
            case 'preview.ready':
              setPreviewUrlSync(d.url || '');
              setBuildLog(prev => [...prev, { type: 'done', content: `🌐 Preview: ${d.url}` }]);
              break;
            case 'session.idle': {
              setBuildRunning(false);
              setBuildPhase('done');
              const elapsed = buildStartTimeRef.current ? `${Math.round((Date.now() - buildStartTimeRef.current) / 1000)}s` : '';
              const pUrl = previewUrlRef.current;
              setBuildSummary({
                files: 0,
                agents: Object.keys(agentStatusRef.current).length || 1,
                prUrl: buildPrUrlRef.current || undefined,
                previewUrl: pUrl || undefined,
                elapsed,
              });
              loadProjectTree(projectPath);
              break;
            }
            case 'graph.node_completed': {
              // Graph runtime completes — treat as done when no more nodes pending.
              setBuildLog(prev => [...prev, { type: 'file_created', path: '', content: `✓ node ${d.title || d.node_id || ''} done` }]);
              break;
            }
            case 'session.error':
              setBuildLog(prev => [...prev, { type: 'error', content: d.message || 'Unknown error' }]);
              break;
          }
        } catch {}
      }
    }
  }, [loadProjectTree, setPreviewUrlSync, setBuildPrUrlSync, setAgentStatusSync]);

  const subscribeToRtHub = useCallback((projectId: string, projectPath: string) => {
    const ws = new WebSocket(wsBase('/ws/realtime'));
    wsRef.current = ws;
    ws.onmessage = (e) => {
      try {
        const msg = JSON.parse(e.data);
        if (msg.type === 'build_event' && msg.data?.project_id === projectId) {
          const { event, data } = msg.data;
          switch (event) {
            case 'phase': setBuildPhase(data.phase || ''); if (data.preview_url) setPreviewUrlSync(data.preview_url); break;
            case 'file_created':
              setBuildLog(prev => [...prev, {
                type: 'file_chip',
                path: data.path || '',
                content: data.path || '',
                linesAdded: data.lines_added ?? 0,
                linesRemoved: data.lines_removed ?? 0,
                totalLines: data.total_lines,
              }]);
              setChanges(prev => prev.some(c => c.path === data.path) ? prev : [...prev, { path: data.path, action: 'created' }]);
              setTimeout(() => loadProjectTree(projectPath), 400);
              break;
            case 'agent_started': setAgentStatusSync(prev => ({ ...prev, [data.agent || '']: 'working' })); break;
            case 'agent_completed': setAgentStatusSync(prev => ({ ...prev, [data.agent || '']: 'done' })); break;
            case 'agent_text':
              setBuildLog(prev => {
                const label = `[${data.agent}] `;
                const last = prev[prev.length - 1];
                if (last?.type === 'text' && last.content.startsWith(label)) return [...prev.slice(0, -1), { ...last, content: last.content + (data.delta || '') }];
                return [...prev, { type: 'text', content: label + (data.delta || '') }];
              });
              break;
            case 'ci_polling':
              setBuildLog(prev => [...prev, { type: 'tool_start', tool: '🔄 CI', content: `Watching PR #${data.pr}` }]);
              break;
            case 'ci_failed':
              setBuildLog(prev => [...prev, { type: 'error', content: `❌ CI failed on PR #${data.pr}` }]);
              break;
            case 'preview_ready': setPreviewUrlSync(data.url || ''); break;
            case 'done':
              setBuildRunning(false); setBuildPhase('done');
              if (data.preview_url) setPreviewUrlSync(data.preview_url);
              loadProjectTree(projectPath, projectId);
              ws.close();
              wsRef.current = null;
              rtHubCleanupRef.current = null;
              break;
          }
        }
      } catch {}
    };
    ws.onclose = () => { wsRef.current = null; };
    return () => ws.close();
  }, [loadProjectTree, setPreviewUrlSync, setAgentStatusSync]);

  const startBuild = useCallback(async (project: CodeProject, description: string, stack: string) => {
    setActiveProject(project); setProjectPath(project.path);
    setCodeProjectName(project.name); sessionId.current = project.session_id;
    setStoreActiveProjectId(project.id);
    setBuildLog([]); setBuildRunning(true); setBuildPhase('planning');
    setBuildPlan(null); setBuildPlanEdited(null); setPreviewUrlSync(''); setBuildPrUrlSync('');
    setBuildSummary(null); setBuildStartTimeSync(Date.now()); setAgentStatus({}); agentStatusRef.current = {};
    openBottomDrawer('build');
    setTabs([]); setActiveTab(''); setChanges([]);
    await loadProjects();
    setProjects(prev => prev.some(p => p.id === project.id) ? prev : [project, ...prev]);

    const abort = new AbortController();
    buildAbortRef.current = abort;
    try {
      const res = await fetchWithRetry(`${API_BASE}/projects/${project.id}/build`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${getToken()}` },
        body: JSON.stringify({ description, stack: stack === 'auto' ? '' : stack }),
        signal: abort.signal,
      });
      if (!res.ok) { setBuildLog([{ type: 'error', content: `Failed: ${res.status}` }]); setBuildRunning(false); return; }
      const reader = res.body?.getReader();
      if (!reader) { setBuildRunning(false); return; }
      await parseSSEStream(reader, project.path);
    } catch (e: any) {
      if (e?.name !== 'AbortError') setBuildLog(prev => [...prev, { type: 'error', content: String(e) }]);
    } finally {
      setBuildRunning(false);
    }
  }, [loadProjects, parseSSEStream, setCodeProjectName, setStoreActiveProjectId, setPreviewUrlSync, setBuildPrUrlSync, setBuildStartTimeSync]);

  const approveBuild = useCallback(async (project: CodeProject) => {
    setBuildRunning(true); setBuildPhase('spawning');
    setBuildLog(prev => [...prev, { type: 'tool_start', tool: '🚀', content: 'Plan approved — spawning agents…', ts: Date.now() }]);

    // Close any previous subscription before opening a new one.
    rtHubCleanupRef.current?.();
    rtHubCleanupRef.current = subscribeToRtHub(project.id, project.path);
    try {
      const res = await fetchWithRetry(`${API_BASE}/projects/${project.id}/approve`, {
        method: 'POST',
        headers: { Authorization: `Bearer ${getToken()}` },
      });
      if (!res.ok) {
        setBuildLog(prev => [...prev, { type: 'error', content: `Approve failed: ${res.status}` }]);
        setBuildRunning(false);
        rtHubCleanupRef.current?.();
        rtHubCleanupRef.current = null;
      }
      // On success the subscription stays open — `subscribeToRtHub` handles
      // the 'done' event internally and the useEffect below closes it on unmount.
    } catch (e) {
      setBuildLog(prev => [...prev, { type: 'error', content: String(e) }]);
      setBuildRunning(false);
      rtHubCleanupRef.current?.();
      rtHubCleanupRef.current = null;
    }
  }, [subscribeToRtHub]);

  const stopBuild = () => {
    buildAbortRef.current?.abort();
    wsRef.current?.close();
    setBuildRunning(false);
    setBuildPhase('');
    setBuildLog(prev => [...prev, { type: 'error', content: 'Build stopped.' }]);
  };

  const openFile = useCallback(async (path: string) => {
    if (tabs.find(t => t.path === path)) { setActiveTab(path); return; }
    if (activeProject?.id) {
      try {
        const res = await apiFetch(`/projects/${activeProject.id}/file?path=${encodeURIComponent(path)}`);
        if (res.ok) {
          const data = await res.json();
          setTabs(prev => [...prev, { path, name: path.split('/').pop() || path, content: data.content || '', dirty: false }]);
          setActiveTab(path);
          return;
        }
      } catch {}
    }
    try {
      const sid = await getSessionId();
      const data = await apiPost('/chat/completions', {
        agent_id: 'prime', session_id: sid, stream: false,
        message: `Use read_file to read ${path}. Return ONLY the raw file contents, no formatting or explanation.`,
      }) as any;
      const content = (data?.choices?.[0]?.message?.content || '').replace(/^```\w*\n?/, '').replace(/\n?```$/, '');
      setTabs(prev => [...prev, { path, name: path.split('/').pop() || path, content, dirty: false }]);
      setActiveTab(path);
    } catch {}
  }, [tabs, activeProject?.id]);

  const closeTab = (path: string) => {
    setTabs(prev => {
      const next = prev.filter(t => t.path !== path);
      if (activeTab === path) setActiveTab(next[0]?.path || '');
      return next;
    });
  };

  const handleChat = useCallback(async (msg: string) => {
    // Cancel any in-flight chat stream before starting a new one.
    chatAbortRef.current?.abort();
    const abort = new AbortController();
    chatAbortRef.current = abort;

    setMessages(prev => [...prev, { role: 'user', content: msg }]);
    setIsLoading(true);
    setMessages(prev => [...prev, { role: 'assistant', content: '', streaming: true, tools: [] }]);

    try {
      const sid = await getSessionId();
      const res = await apiFetch('/chat/completions', {
        method: 'POST',
        body: JSON.stringify({ agent_id: 'prime', message: msg, session_id: sid, stream: true, ...(codeThinkingLevel !== 'off' ? { thinking_level: codeThinkingLevel } : {}) }),
        signal: abort.signal,
      });

      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const reader = res.body?.getReader();
      const decoder = new TextDecoder();
      if (!reader) throw new Error('No stream reader');

      let fullContent = '';
      const toolCalls: ChatMsg['tools'] = [];
      let currentTool: { name: string; args: string } | null = null;

      while (!abort.signal.aborted) {
        const { done, value } = await reader.read();
        if (done) break;
        for (const line of decoder.decode(value, { stream: true }).split('\n')) {
          const raw = line.replace(/^data: /, '').trim();
          if (!raw || raw === '[DONE]') continue;
          try {
            const evt = JSON.parse(raw);
            if (evt.choices?.[0]?.delta?.content) {
              fullContent += evt.choices[0].delta.content;
              setMessages(prev => {
                const copy = [...prev];
                copy[copy.length - 1] = { ...copy[copy.length - 1]!, content: fullContent, streaming: true };
                return copy;
              });
            }
            if (evt.type === 'tool_start') {
              const name = evt.data?.name || evt.data?.tool || '';
              currentTool = { name, args: JSON.stringify(evt.data?.input || {}) };
            }
            if (evt.type === 'tool_result') {
              const name = evt.data?.name || currentTool?.name || '';
              const result = typeof evt.data?.result === 'string' ? evt.data.result : JSON.stringify(evt.data?.result || '');
              toolCalls.push({ name, args: currentTool?.args, result: result.slice(0, 500) });
              currentTool = null;
              if (['write_file', 'edit', 'apply_patch'].includes(name)) {
                const input = evt.data?.input || {};
                const fp = input.path || input.file_path || '';
                if (fp) {
                  setChanges(prev => prev.some(c => c.path === fp) ? prev : [...prev, { path: fp, action: 'modified' }]);
                  setTimeout(() => loadProjectTree(activeProject?.path || projectPath), 500);
                }
              }
              // exec tool output is streamed directly inside the PTY terminal.
              setMessages(prev => {
                const copy = [...prev];
                copy[copy.length - 1] = { ...copy[copy.length - 1]!, tools: [...toolCalls], streaming: true };
                return copy;
              });
            }
          } catch {}
        }
      }

      setMessages(prev => {
        const copy = [...prev];
        copy[copy.length - 1] = { ...copy[copy.length - 1]!, content: fullContent, streaming: false, tools: toolCalls };
        return copy;
      });
    } catch (err: any) {
      if (err?.name !== 'AbortError') {
        setMessages(prev => {
          const copy = [...prev];
          copy[copy.length - 1] = { role: 'assistant', content: `Error: ${err}`, streaming: false };
          return copy;
        });
      }
    } finally {
      setIsLoading(false);
    }
  }, [activeProject, projectPath, loadProjectTree, codeThinkingLevel]);


  const currentTab = tabs.find(t => t.path === activeTab);
  const allFiles = useCallback((nodes: FileNode[]): FileNode[] =>
    nodes.flatMap(n => n.type === 'file' ? [n] : allFiles(n.children ?? [])), []);

  if (activeCodeTab !== 'editor') {
    return (
      <div className="full-bleed flex flex-col" style={{ height: 'calc(100vh - var(--header-height) - var(--toolbar-height, 0px))' }}>
        <div className="flex-1 overflow-hidden">
          {activeCodeTab === 'tickets'   && <TicketsTab />}
          {activeCodeTab === 'tasks'     && <TasksTab />}
          {activeCodeTab === 'github'    && <GitHubTab />}
          {activeCodeTab === 'workflows' && <WorkflowsTab />}
          {activeCodeTab === 'plans'     && <PlansTab />}
          {activeCodeTab === 'inbox'     && <InboxTab />}
          {activeCodeTab === 'org'       && <OrgTab />}
          {activeCodeTab === 'goals'     && <GoalsTab />}
          {activeCodeTab === 'inception' && <InceptionTab />}
        </div>
      </div>
    );
  }

  if (!activeProject) {
    return (
      <div className="full-bleed flex flex-col overflow-hidden" style={{ height: 'calc(100vh - var(--header-height) - var(--toolbar-height, 0px))' }}>
        <div className="flex flex-1 overflow-hidden">
          <div className="flex-1 min-w-0 overflow-y-auto bg-background">
            <div className="mx-auto max-w-3xl px-6 py-10 space-y-8">
              <div>
                <div className="flex items-center gap-2 mb-2">
                  <div className="flex h-9 w-9 items-center justify-center rounded-xl bg-primary/10">
                    <Code className="h-5 w-5 text-primary" />
                  </div>
                  <h1 className="text-lg font-semibold">Code</h1>
                </div>
                <p className="text-sm text-muted-foreground max-w-xl">
                  Describe your project in the chat — Prime will prepare a detailed plan, team, and budget estimate for your approval before any work starts.
                </p>
              </div>

              {projects.length > 0 && (
                <div>
                  <h2 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground mb-2">Recent projects</h2>
                  <div className="grid gap-2 sm:grid-cols-2">
                    {projects.slice(0, 6).map(p => (
                      <button key={p.id} onClick={() => switchProject(p)}
                        className="flex items-center gap-3 rounded-lg border border-border bg-card p-3 text-left hover:border-primary/30 hover:bg-accent transition-colors">
                        <FolderOpen className="h-4 w-4 text-amber-500/80 shrink-0" />
                        <div className="min-w-0 flex-1">
                          <p className="text-sm font-medium truncate">{p.display_name || p.name}</p>
                          <p className="text-xs text-muted-foreground font-mono truncate">{p.path}</p>
                        </div>
                        {p.build_phase === 'building' && <Loader2 className="h-3.5 w-3.5 animate-spin text-primary shrink-0" />}
                      </button>
                    ))}
                  </div>
                </div>
              )}
            </div>
          </div>

          <aside className="w-[380px] shrink-0 border-l border-border bg-muted/10 flex flex-col overflow-hidden">
            <ProjectDiscoveryChat
              onReady={() => {}}
              onNameGenerated={(name) => setCodeProjectName(name)}
            />
          </aside>
        </div>
      </div>
    );
  }

  return (
    <div className="full-bleed flex flex-col overflow-hidden" style={{ height: 'calc(100vh - var(--header-height) - var(--toolbar-height, 0px))' }}>
      <div className="flex flex-1 overflow-hidden">
        <div className="flex flex-1 flex-col min-w-0 overflow-hidden">

          <EditorTabBar tabs={tabs} active={activeTab} onSelect={setActiveTab} onClose={closeTab} />

          {currentTab && (
            <div className="flex h-[22px] shrink-0 items-center border-b border-border bg-muted/20 px-3 gap-2">
              <span className="text-xs text-muted-foreground font-mono truncate">{currentTab.path}</span>
            </div>
          )}

          <div className={cn('flex-1 overflow-hidden min-h-0', termOpen && 'border-b border-border')}>
            {currentTab ? (
              <CodeEditor content={currentTab.content} path={currentTab.path}
                onChange={value => setTabs(prev => prev.map(t =>
                  t.path === currentTab.path ? { ...t, content: value, dirty: true } : t
                ))}
              />
            ) : (
              <div className="flex h-full flex-col items-center justify-center gap-4">
                <div className="flex h-14 w-14 items-center justify-center rounded-xl bg-muted">
                  <Code className="h-7 w-7 text-muted-foreground/40" />
                </div>
                <div className="text-center">
                  <p className="font-medium text-sm">{activeProject.name}</p>
                  <p className="text-xs text-muted-foreground mt-1">Select a file from the explorer</p>
                  <div className="flex items-center justify-center gap-3 mt-2">
                    <span className="text-xs text-muted-foreground/50">
                      <kbd className="bg-muted px-1 rounded text-xs">⌘P</kbd> open file
                    </span>
                    <span className="text-xs text-muted-foreground/50">
                      <kbd className="bg-muted px-1 rounded text-xs">⌘`</kbd> terminal
                    </span>
                  </div>
                </div>
                {buildLog.length > 0 && (
                  <button onClick={() => openBottomDrawer('build')}
                    className="flex items-center gap-2 rounded-lg border border-primary/30 bg-primary/5 px-4 py-2 text-xs text-primary hover:bg-primary/10 transition-colors">
                    {buildRunning ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <CheckCircle2 className="h-3.5 w-3.5" />}
                    {buildRunning ? 'Building…' : 'View build log'}
                  </button>
                )}
              </div>
            )}
          </div>

          {termOpen && (
            <div className="h-[200px] shrink-0 border-t border-border">
              <TerminalPane />
            </div>
          )}

          <div className="flex h-[22px] shrink-0 items-center border-t border-border bg-primary px-3 gap-0 text-primary-foreground text-xs">
            <button onClick={() => setTermOpen(!termOpen)}
              className="flex items-center gap-1 px-1.5 hover:bg-white/20 rounded-sm h-full transition-colors">
              <Terminal className="h-3 w-3" /><span>Terminal</span>
            </button>
            {buildRunning && (
              <>
                <div className="w-px h-3 bg-white/20 mx-0.5" />
                <button onClick={() => openBottomDrawer('build')}
                  className="flex items-center gap-1 px-1.5 hover:bg-white/20 rounded-sm h-full transition-colors">
                  <Loader2 className="h-3 w-3 animate-spin" /><span>Building…</span>
                </button>
              </>
            )}
            {changes.length > 0 && <>
              <div className="w-px h-3 bg-white/20 mx-0.5" />
              <div className="flex items-center gap-1 px-1.5"><GitCommit className="h-3 w-3" /><span>{changes.length}</span></div>
            </>}
            {currentTab && <>
              <div className="w-px h-3 bg-white/20 mx-0.5" />
              <div className="flex items-center gap-1 px-1.5"><GitBranch className="h-3 w-3" /><span>main</span></div>
              <div className="w-px h-3 bg-white/20 mx-0.5" />
              <div className="px-1.5">{detectLang(currentTab.path).toUpperCase()}</div>
              <div className="w-px h-3 bg-white/20 mx-0.5" />
              <div className="px-1.5">Ln {currentTab.content.split('\n').length}</div>
            </>}
            <div className="ml-auto flex items-center gap-2">
              <span className="font-semibold">{activeProject.name}</span>
              <div className="w-px h-3 bg-white/20" />
              <button onClick={() => setActiveProject(null)}
                className="flex items-center gap-1 hover:bg-white/20 px-1.5 rounded-sm h-full transition-colors">
                <Plus className="h-3 w-3" />New
              </button>
            </div>
          </div>

        </div>

        {/* Persistent Prime chat sidebar — sibling to editor column */}
        <aside className="w-[340px] shrink-0 border-l border-border bg-muted/10 flex flex-col overflow-hidden">
          <CodeChatSidebar
            messages={messages}
            isLoading={isLoading}
            onSend={handleChat}
            thinkingLevel={codeThinkingLevel}
            onThinkingLevelChange={setCodeThinkingLevel}
          />
        </aside>

        <BottomDrawerTab id="build" label="Build" iconName="Zap" order={20} badge={buildRunning ? '…' : undefined}>
          <div className="flex h-full flex-col overflow-hidden">
            {buildPhase && (
              <div className="flex shrink-0 items-center gap-1 overflow-x-auto border-b border-border bg-muted/20 px-3 py-1.5 scrollbar-none">
                {['planning','pending_approval','spawning','building','testing','pushing','preview','done'].map((phase, i, arr) => {
                  const phases = ['planning','pending_approval','spawning','building','testing','pushing','preview','done'];
                  const idx = phases.indexOf(buildPhase);
                  const thisIdx = phases.indexOf(phase);
                  const done = thisIdx < idx;
                  const active = thisIdx === idx;
                  return (
                    <div key={phase} className="flex items-center gap-1 shrink-0">
                      <span className={cn('text-xs font-medium capitalize',
                        active ? 'text-primary' : done ? 'text-emerald-500' : 'text-muted-foreground/40')}>
                        {done ? '✓' : active ? '›' : '·'} {phase.replace('_', ' ')}
                      </span>
                      {i < arr.length - 1 && <span className="text-muted-foreground/20 text-xs">→</span>}
                    </div>
                  );
                })}
              </div>
            )}

            {buildPhase === 'pending_approval' && buildPlan && (
              <div className="shrink-0 border-b border-border overflow-y-auto max-h-[60%]">
                <div className="p-4 space-y-3">
                  <div className="flex items-start justify-between gap-2">
                    <div>
                      <p className="text-xs font-semibold text-primary">Plan ready</p>
                      <p className="text-xs text-muted-foreground mt-0.5 leading-relaxed">{buildPlan.summary}</p>
                    </div>
                    <span className="shrink-0 rounded border border-border px-2 py-0.5 text-xs text-muted-foreground font-mono">{buildPlan.stack}</span>
                  </div>

                  <div className="flex flex-wrap gap-1">
                    {(buildPlan.agents || []).map((a: any) => (
                      <span key={a.role} className="rounded-full bg-primary/10 text-primary px-2 py-0.5 text-xs">{a.role}</span>
                    ))}
                  </div>

                  <div>
                    <div className="flex items-center justify-between mb-1">
                      <p className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                        {(buildPlanEdited?.files ?? buildPlan.files ?? []).length} files to create
                      </p>
                      {buildPlanEdited && (
                        <button onClick={() => setBuildPlanEdited(null)} className="text-xs text-muted-foreground hover:text-foreground">reset</button>
                      )}
                    </div>
                    <div className="space-y-0.5 max-h-40 overflow-y-auto rounded border border-border bg-muted/20 p-1.5">
                      {(buildPlanEdited?.files ?? buildPlan.files ?? []).map((f: string, i: number) => (
                        <div key={i} className="group flex items-center gap-1.5 rounded px-1 py-0.5 hover:bg-accent/50">
                          <File className={cn('h-3 w-3 shrink-0', fileColor(f))} />
                          <span className="flex-1 truncate text-xs font-mono">{f}</span>
                          <button
                            onClick={() => {
                              const current = buildPlanEdited?.files ?? buildPlan.files ?? [];
                              setBuildPlanEdited({ ...buildPlan, ...buildPlanEdited, files: current.filter((_: string, j: number) => j !== i) });
                            }}
                            className="opacity-0 group-hover:opacity-100 text-muted-foreground hover:text-destructive transition-all"
                          >
                            <X className="h-3 w-3" />
                          </button>
                        </div>
                      ))}
                    </div>
                  </div>

                  <button
                    onClick={() => {
                      const effectivePlan = buildPlanEdited || buildPlan;
                      if (buildPlanEdited && activeProject) {
                        apiRequest(`/projects/${activeProject.id}/notes`, {
                          method: 'PUT',
                          body: JSON.stringify({ notes: JSON.stringify(effectivePlan) }),
                        }).catch(() => {});
                      }
                      approveBuild(activeProject!);
                    }}
                    className="w-full flex items-center justify-center gap-2 rounded-lg bg-primary py-2 text-xs font-semibold text-primary-foreground hover:bg-primary/90 transition-colors"
                  >
                    <Play className="h-3.5 w-3.5" />
                    Approve & Start Building
                    {buildPlanEdited && <span className="text-xs opacity-70">(edited)</span>}
                  </button>
                </div>
              </div>
            )}

            {Object.keys(agentStatus).length > 0 && (
              <div className="flex shrink-0 flex-wrap gap-1.5 border-b border-border px-3 py-2">
                {Object.entries(agentStatus).map(([agent, status]) => (
                  <div key={agent} className={cn('flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium',
                    status === 'done' ? 'bg-emerald-500/10 text-emerald-600' :
                    status === 'error' ? 'bg-destructive/10 text-destructive' :
                    'bg-primary/10 text-primary')}>
                    {status === 'working' ? <Loader2 className="h-2.5 w-2.5 animate-spin" /> : status === 'done' ? <CheckCircle2 className="h-2.5 w-2.5" /> : <AlertCircle className="h-2.5 w-2.5" />}
                    {agent.split('-').pop()}
                  </div>
                ))}
              </div>
            )}

            {buildLog.length === 0 ? (
              <div className="flex flex-1 flex-col items-center justify-center gap-3 p-6 text-center">
                <Zap className="h-8 w-8 text-muted-foreground/30" />
                <p className="text-sm text-muted-foreground">No build yet</p>
                <p className="text-xs text-muted-foreground/60">Create a new project to start building with AI</p>
              </div>
            ) : (
              <div className="flex-1 overflow-hidden">
                <BuildLog
                  entries={buildLog}
                  running={buildRunning}
                  onStop={stopBuild}
                  onFileClick={openFile}
                  onOpenSession={() => openBottomDrawer('build')}
                  summary={buildSummary || undefined}
                />
              </div>
            )}

            {previewUrl && (
              <div className="shrink-0 border-t border-border bg-muted/20 px-3 py-2 flex items-center gap-2">
                <span className="text-xs text-muted-foreground">Preview:</span>
                <a href={previewUrl} target="_blank" rel="noopener noreferrer" className="text-xs text-primary hover:underline truncate flex-1">{previewUrl}</a>
                <button onClick={() => window.open(previewUrl, '_blank')}
                  className="rounded bg-primary px-2 py-0.5 text-xs text-primary-foreground hover:bg-primary/90">Open</button>
              </div>
            )}
          </div>
        </BottomDrawerTab>

        <BottomDrawerTab
          id="agents"
          label="Agents"
          iconName="Bot"
          order={30}
          badge={pendingAgentPlans > 0 ? pendingAgentPlans : undefined}
        >
          <div className="flex h-full overflow-hidden">
            {/* Agent grid — left half */}
            <div className="w-64 shrink-0 border-r border-border overflow-y-auto p-3">
              <AgentDashboard />
            </div>
            {/* Task feed — right */}
            <div className="flex-1 overflow-y-auto p-3">
              <TaskFeed compact />
            </div>
          </div>
        </BottomDrawerTab>

        <BottomDrawerTab id="activity" label="Activity" iconName="Activity" order={35}>
          <div className="h-full overflow-y-auto">
            <ActivityFeed />
          </div>
        </BottomDrawerTab>

        {quickOpen && (
          <div className="fixed inset-0 z-50 flex items-start justify-center pt-[15vh]" onClick={() => setQuickOpen(false)}>
            <div className="w-full max-w-xl overflow-hidden rounded-xl border border-border bg-card shadow-2xl"
              onClick={e => e.stopPropagation()}>
              <div className="flex items-center gap-2 border-b border-border px-4 py-2.5">
                <Search className="h-4 w-4 text-muted-foreground shrink-0" />
                <input autoFocus value={quickQuery} onChange={e => setQuickQuery(e.target.value)}
                  placeholder="Go to file…"
                  className="flex-1 bg-transparent text-sm outline-none placeholder:text-muted-foreground/50" />
                <kbd className="text-xs text-muted-foreground bg-muted px-1.5 py-0.5 rounded">ESC</kbd>
              </div>
              <div className="max-h-80 overflow-y-auto py-1">
                {(() => {
                  const q = quickQuery.toLowerCase();
                  const files = allFiles(tree);
                  const matches = q ? files.filter(n => n.path.toLowerCase().includes(q)) : files.slice(0, 20);
                  if (matches.length === 0) return <p className="py-8 text-center text-sm text-muted-foreground">No files match</p>;
                  return matches.map(n => (
                    <button key={n.path} onClick={() => { openFile(n.path); setQuickOpen(false); }}
                      className="flex w-full items-center gap-3 px-4 py-1.5 text-sm hover:bg-accent transition-colors text-left">
                      <File className={cn('h-4 w-4 shrink-0', fileColor(n.name))} />
                      <span className="flex-1 truncate">{n.name}</span>
                      <span className="text-xs text-muted-foreground/50 truncate max-w-[200px] text-right">
                        {n.path.split('/').slice(0, -1).join('/')}
                      </span>
                    </button>
                  ));
                })()}
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
