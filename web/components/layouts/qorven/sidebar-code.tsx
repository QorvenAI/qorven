'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useEffect, useState } from 'react';
import { useRouter } from 'next/navigation';
import { useStore } from '@/store';
import { cn } from '@/lib/utils';
import { apiBase } from '@/lib/api-url';
import {
  FolderOpen, Search, X, ChevronDown, ChevronRight, GitBranch, Loader2,
  Code, Plus, Globe,
} from 'lucide-react';

export function CodeSidebar() {
  const router = useRouter();
  const tab = useStore((s) => s.codeSidebarTab);
  const setTab = useStore((s) => s.setCodeSidebarTab);
  const projects = useStore((s) => s.codeProjects);
  const activeProjectId = useStore((s) => s.codeActiveProjectId);
  const codeProjectName = useStore((s) => s.codeProjectName);
  const tree = useStore((s) => s.codeTree);
  const projectPath = useStore((s) => s.codeProjectPath);
  const [search, setSearch] = useState('');
  const activeProject = projects.find((p: any) => p.id === activeProjectId);

  const flatFiles = (nodes: any[]): any[] =>
    nodes.flatMap((n: any) => n.type === 'file' ? [n] : flatFiles(n.children ?? []));
  const searchResults = search.length >= 2
    ? flatFiles(tree).filter((n: any) => n.name.toLowerCase().includes(search.toLowerCase())).slice(0, 30)
    : [];

  // GitHub state
  const [ghConnected, setGhConnected] = useState<boolean | null>(null);
  const [ghRepo, setGhRepo] = useState('');
  const [ghOwner, setGhOwner] = useState('');
  const [ghIssues, setGhIssues] = useState<any[]>([]);
  const [ghPrs, setGhPrs] = useState<any[]>([]);
  const [ghLoading, setGhLoading] = useState(false);
  const [ghInput, setGhInput] = useState('');
  const [ghTab, setGhTab] = useState<'issues' | 'prs'>('issues');
  const ghFetch = (path: string) => {
    const tok = typeof window !== 'undefined' ? (localStorage.getItem('qorven_token') || '') : '';
    return fetch(`${apiBase()}${path}`, { headers: { Authorization: `Bearer ${tok}` } })
      .then(r => r.ok ? r.json() : null).catch(() => null);
  };

  useEffect(() => {
    if (tab === 'github' && ghConnected === null) {
      ghFetch('/connections').then(d => {
        setGhConnected(!!(d?.connections || []).find((c: any) => c.platform_id === 'github'));
      });
    }
  }, [tab, ghConnected]);

  const loadGhRepo = async (ownerRepo: string) => {
    const parts = ownerRepo.trim().split('/');
    if (parts.length !== 2) return;
    const [o, r] = parts as [string, string];
    setGhOwner(o); setGhRepo(r); setGhLoading(true);
    const [issData, prData] = await Promise.all([
      ghFetch(`/github/${o}/${r}/issues?state=open&limit=25`),
      ghFetch(`/github/${o}/${r}/pulls?state=open&limit=15`),
    ]);
    setGhIssues(issData?.issues || []);
    setGhPrs(prData?.pulls || []);
    setGhLoading(false);
  };

  return (
    <>
      {(codeProjectName || activeProject) && (
        <div className="flex items-center gap-2 border-b border-border px-3 py-2 shrink-0 bg-muted/20">
          <div className="flex h-5 w-5 shrink-0 items-center justify-center rounded bg-primary/15">
            <Code className="h-3 w-3 text-primary" />
          </div>
          <div className="flex-1 min-w-0">
            <p className="text-xs font-semibold truncate">{codeProjectName || (activeProject as any)?.name || 'Project'}</p>
            {projectPath && (
              <p className="text-xs text-muted-foreground font-mono truncate">{projectPath.replace(/^.*\/qorven-workspace\//, '~/')}</p>
            )}
          </div>
          {projects.length > 1 && (
            <select
              value={activeProjectId || ''}
              onChange={e => useStore.getState().setCodeActiveProjectId(e.target.value || null)}
              className="h-5 w-5 rounded cursor-pointer opacity-50 hover:opacity-100 transition-opacity"
              title="Switch project"
            >
              <option value="">— switch —</option>
              {projects.map((p: any) => (
                <option key={p.id} value={p.id}>{p.name}</option>
              ))}
            </select>
          )}
        </div>
      )}

      <div className="flex border-b border-border shrink-0">
        {[
          { id: 'explorer' as const, label: 'Explorer', icon: FolderOpen },
          { id: 'github' as const, label: 'GitHub', icon: GitBranch },
        ].map(t => (
          <button key={t.id} onClick={() => setTab(t.id)}
            className={cn(
              'flex flex-1 items-center justify-center gap-1.5 px-2 py-2.5 text-xs font-medium border-b-2 -mb-px transition-colors',
              tab === t.id ? 'border-primary text-foreground' : 'border-transparent text-muted-foreground hover:text-foreground'
            )}>
            <t.icon className="h-3.5 w-3.5" />
            {t.label}
          </button>
        ))}
      </div>

      {tab === 'explorer' && (
        <>
          <div className="flex items-center gap-1.5 px-3 pt-3 pb-2">
            <select
              value={activeProjectId || ''}
              onChange={e => useStore.getState().setCodeActiveProjectId(e.target.value || null)}
              className={cn(
                'qr-select text-xs transition-all',
                search ? 'w-0 overflow-hidden opacity-0 p-0 border-0' : 'flex-1'
              )}
            >
              <option value="">Select project…</option>
              {projects.map((p: any) => <option key={p.id} value={p.id}>{p.name}</option>)}
            </select>
            <div className={cn('relative transition-all', search ? 'flex-1' : 'w-8')}>
              <Search className="absolute left-2 top-1/2 -translate-y-1/2 h-3 w-3 text-muted-foreground" />
              <input
                value={search}
                onChange={e => setSearch(e.target.value)}
                placeholder={search ? 'Search files…' : ''}
                className={cn(
                  'qr-input text-xs transition-all',
                  search ? 'w-full pl-7 pr-7' : 'w-8 pl-7 pr-1 cursor-pointer'
                )}
              />
              {search && (
                <button onClick={() => setSearch('')} className="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground">
                  <X className="h-3 w-3" />
                </button>
              )}
            </div>
          </div>

          <div className="flex-1 overflow-y-auto">
            {search.length >= 2 ? (
              <div className="px-1">
                {searchResults.length === 0 ? (
                  <p className="px-3 py-4 text-center text-xs text-muted-foreground">No files match</p>
                ) : searchResults.map((n: any) => (
                  <button key={n.path} onClick={() => { router.push(`/code?open=${encodeURIComponent(n.path)}`); setSearch(''); }}
                    className="flex w-full items-center gap-2 rounded-md px-2.5 py-1 text-xs text-muted-foreground hover:text-foreground hover:bg-accent transition-colors text-left">
                    <span className="truncate flex-1">{n.name}</span>
                    <span className="text-2xs text-muted-foreground/50 truncate max-w-[60px]">{n.path.split('/').slice(-2, -1)[0]}</span>
                  </button>
                ))}
              </div>
            ) : tree.length > 0 ? (
              <div className="px-1 py-0.5">
                {tree.map((n: any) => (
                  <SidebarTreeNode key={n.path} node={n} depth={0} />
                ))}
              </div>
            ) : (
              <div className="px-3 py-6 text-center">
                <FolderOpen className="h-6 w-6 mx-auto text-muted-foreground/30 mb-2" />
                <p className="text-xs text-muted-foreground">No project open</p>
                <p className="text-2xs text-muted-foreground/50 mt-0.5">Select or create a project above</p>
              </div>
            )}
          </div>

          {projectPath && (
            <div className="border-t border-border px-3 py-2">
              <p className="text-2xs text-muted-foreground/50 font-mono truncate" title={projectPath}>
                {projectPath}
              </p>
            </div>
          )}
        </>
      )}

      {tab === 'github' && (
        <>
          {ghConnected === false && (
            <div className="flex flex-1 flex-col items-center justify-center gap-3 p-5 text-center">
              <GitBranch className="h-8 w-8 text-muted-foreground/30" />
              <p className="text-xs text-muted-foreground">GitHub not connected</p>
              <button onClick={() => router.push('/provider-keys')}
                className="rounded-md bg-primary px-3 py-1.5 text-xs text-primary-foreground hover:bg-primary/90">
                Add Token
              </button>
            </div>
          )}

          {ghConnected !== false && (
            <>
              <div className="px-3 pt-3 pb-2">
                <div className="flex gap-1.5">
                  <input
                    value={ghInput}
                    onChange={e => setGhInput(e.target.value)}
                    onKeyDown={e => e.key === 'Enter' && loadGhRepo(ghInput)}
                    placeholder="owner/repo"
                    className="qr-textarea flex-1 resize-none text-xs font-mono"
                  />
                  <button onClick={() => loadGhRepo(ghInput)}
                    className="rounded-md bg-primary px-2 py-1.5 text-xs text-primary-foreground hover:bg-primary/90 shrink-0">
                    Load
                  </button>
                </div>
                {ghRepo && (
                  <p className="text-2xs text-muted-foreground mt-1.5 font-mono">
                    {ghOwner}/{ghRepo}
                  </p>
                )}
              </div>

              {ghRepo && (
                <>
                  <div className="flex border-b border-border mx-3 mb-1">
                    {(['issues', 'prs'] as const).map(t => (
                      <button key={t} onClick={() => setGhTab(t)}
                        className={cn(
                          'flex-1 py-1.5 text-2xs font-medium border-b-2 -mb-px transition-colors',
                          ghTab === t ? 'border-primary text-foreground' : 'border-transparent text-muted-foreground hover:text-foreground'
                        )}>
                        {t === 'issues' ? `Issues (${ghIssues.length})` : `PRs (${ghPrs.length})`}
                      </button>
                    ))}
                  </div>

                  <div className="flex-1 overflow-y-auto">
                    {ghLoading && (
                      <div className="flex items-center justify-center py-6">
                        <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
                      </div>
                    )}

                    {!ghLoading && ghTab === 'issues' && (
                      <div className="px-1">
                        {ghIssues.length === 0 ? (
                          <p className="px-3 py-4 text-center text-xs text-muted-foreground">No open issues</p>
                        ) : ghIssues.map((iss: any) => (
                          <div key={iss.number} className="group rounded-md mx-1 px-2 py-1.5 hover:bg-accent/50 transition-colors">
                            <div className="flex items-start gap-1.5">
                              <span className="text-2xs text-muted-foreground/50 font-mono mt-0.5 shrink-0">#{iss.number}</span>
                              <div className="flex-1 min-w-0">
                                <p className="text-xs truncate">{iss.title}</p>
                                {iss.labels?.length > 0 && (
                                  <div className="flex flex-wrap gap-1 mt-0.5">
                                    {iss.labels.slice(0, 2).map((l: any) => (
                                      <span key={l.name} className="rounded px-1 py-0.5 text-2xs"
                                        style={{ background: `#${l.color}22`, color: `#${l.color}` }}>
                                        {l.name}
                                      </span>
                                    ))}
                                  </div>
                                )}
                                <div className="flex items-center gap-1.5 mt-1 opacity-0 group-hover:opacity-100 transition-opacity">
                                  <a href={iss.html_url} target="_blank" rel="noopener noreferrer"
                                    className="text-2xs text-primary hover:underline">Open</a>
                                  <span className="text-muted-foreground/30">·</span>
                                  <button className="text-2xs text-muted-foreground hover:text-foreground"
                                    onClick={() => navigator.clipboard.writeText(`Fix issue #${iss.number}: ${iss.title}`)}>
                                    Copy prompt
                                  </button>
                                </div>
                              </div>
                            </div>
                          </div>
                        ))}
                      </div>
                    )}

                    {!ghLoading && ghTab === 'prs' && (
                      <div className="px-1">
                        {ghPrs.length === 0 ? (
                          <p className="px-3 py-4 text-center text-xs text-muted-foreground">No open PRs</p>
                        ) : ghPrs.map((pr: any) => (
                          <div key={pr.number} className="group rounded-md mx-1 px-2 py-1.5 hover:bg-accent/50 transition-colors">
                            <div className="flex items-start gap-1.5">
                              <span className="text-2xs text-muted-foreground/50 font-mono mt-0.5 shrink-0">#{pr.number}</span>
                              <div className="flex-1 min-w-0">
                                <p className="text-xs truncate">{pr.title}</p>
                                <p className="text-2xs text-muted-foreground/50 font-mono mt-0.5">
                                  {pr.head?.ref} → {pr.base?.ref}
                                </p>
                                {pr.draft && <span className="text-2xs bg-muted text-muted-foreground rounded px-1 py-0.5">draft</span>}
                                <div className="flex items-center gap-1.5 mt-1 opacity-0 group-hover:opacity-100 transition-opacity">
                                  <a href={pr.html_url} target="_blank" rel="noopener noreferrer"
                                    className="text-2xs text-primary hover:underline">Open</a>
                                  <span className="text-muted-foreground/30">·</span>
                                  <button className="text-2xs text-muted-foreground hover:text-foreground"
                                    onClick={() => navigator.clipboard.writeText(`Review PR #${pr.number}: ${pr.title}`)}>
                                    Copy prompt
                                  </button>
                                </div>
                              </div>
                            </div>
                          </div>
                        ))}
                      </div>
                    )}
                  </div>

                  <div className="border-t border-border px-3 py-2 space-y-1">
                    <a href={`https://github.com/${ghOwner}/${ghRepo}/issues/new`} target="_blank" rel="noopener noreferrer"
                      className="flex items-center gap-2 rounded-md px-2 py-1.5 text-xs text-muted-foreground hover:text-foreground hover:bg-accent transition-colors">
                      <Plus className="h-3.5 w-3.5" /> New Issue
                    </a>
                    <a href={`https://github.com/${ghOwner}/${ghRepo}`} target="_blank" rel="noopener noreferrer"
                      className="flex items-center gap-2 rounded-md px-2 py-1.5 text-xs text-muted-foreground hover:text-foreground hover:bg-accent transition-colors">
                      <Globe className="h-3.5 w-3.5" /> Open on GitHub
                    </a>
                  </div>
                </>
              )}

              {!ghRepo && ghConnected === true && !ghLoading && (
                <div className="flex flex-1 flex-col items-center justify-center gap-2 p-5 text-center">
                  <GitBranch className="h-7 w-7 text-muted-foreground/30" />
                  <p className="text-xs text-muted-foreground">Enter a repo above</p>
                </div>
              )}
            </>
          )}
        </>
      )}
    </>
  );
}

export function SidebarTreeNode({ node, depth }: { node: any; depth: number }) {
  const [open, setOpen] = useState(depth === 0);
  const router = useRouter();
  const isDir = node.type === 'dir';

  return (
    <>
      <button
        onClick={() => isDir ? setOpen(!open) : router.push(`/code?open=${encodeURIComponent(node.path)}`)}
        className={cn(
          'flex w-full items-center gap-1 rounded-sm py-[2px] text-xs text-muted-foreground hover:text-foreground hover:bg-accent/50 transition-colors',
        )}
        style={{ paddingLeft: `${depth * 12 + 8}px` }}
      >
        {isDir ? (
          <>
            {open ? <ChevronDown className="h-3 w-3 shrink-0" /> : <ChevronRight className="h-3 w-3 shrink-0" />}
            <FolderOpen className="h-3.5 w-3.5 shrink-0 text-amber-500/80" />
          </>
        ) : (
          <>
            <span className="w-3 shrink-0" />
            <span className="h-3.5 w-3.5 shrink-0 text-muted-foreground/60">·</span>
          </>
        )}
        <span className="truncate">{node.name}</span>
      </button>
      {isDir && open && node.children?.map((c: any) => (
        <SidebarTreeNode key={c.path} node={c} depth={depth + 1} />
      ))}
    </>
  );
}
