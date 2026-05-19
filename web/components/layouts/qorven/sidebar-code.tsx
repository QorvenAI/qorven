'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useRef, useState } from 'react';
import { useRouter } from 'next/navigation';
import { useStore } from '@/store';
import { cn } from '@/lib/utils';
import { apiBase } from '@/lib/api-url';
import {
  FolderOpen, Search, X, ChevronDown, ChevronRight, GitBranch, Loader2,
  Code, Plus, Globe, Check, FilePlus, FolderPlus,
} from 'lucide-react';
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover';
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList } from '@/components/ui/command';

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

      {(codeProjectName || activeProject) && (
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
      )}

      {(tab === 'explorer' || !(codeProjectName || activeProject)) && (
        <>
          <div className="flex items-center gap-1.5 px-3 pt-3 pb-2">
            <ProjectCombobox
              projects={projects}
              activeProjectId={activeProjectId}
              onSelect={id => useStore.getState().setCodeActiveProjectId(id || null)}
            />
            <NewItemButton activeProjectId={activeProjectId} />
          </div>

          {(codeProjectName || activeProject) && (
            <div className="px-3 pb-2 relative">
              <Search className="absolute left-5 top-1/2 -translate-y-1/2 h-3 w-3 text-muted-foreground pointer-events-none" />
              <input
                value={search}
                onChange={e => setSearch(e.target.value)}
                placeholder="Search files…"
                className="qr-input text-xs w-full pl-7 pr-7"
              />
              {search && (
                <button onClick={() => setSearch('')} className="absolute right-5 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground">
                  <X className="h-3 w-3" />
                </button>
              )}
            </div>
          )}

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

      {tab === 'github' && (codeProjectName || activeProject) && (
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

function ProjectCombobox({ projects, activeProjectId, onSelect }: {
  projects: any[];
  activeProjectId: string | null;
  onSelect: (id: string | null) => void;
}) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState('');
  const active = projects.find((p: any) => p.id === activeProjectId);
  const label = active?.display_name || active?.name || 'Select project…';

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <button
          className="flex flex-1 min-w-0 items-center justify-between gap-1.5 rounded-md border border-border bg-background px-2.5 py-1.5 text-xs hover:bg-accent transition-colors"
        >
          <span className="truncate text-left">{label}</span>
          <ChevronDown className="h-3 w-3 shrink-0 text-muted-foreground" />
        </button>
      </PopoverTrigger>
      <PopoverContent className="w-64 p-0" align="start" sideOffset={4}>
        <Command>
          <CommandInput
            placeholder="Search projects…"
            value={query}
            onValueChange={setQuery}
            className="h-8 text-xs"
          />
          <CommandList>
            <CommandEmpty className="py-4 text-xs text-muted-foreground text-center">No projects found</CommandEmpty>
            <CommandGroup>
              {projects.map((p: any) => (
                <CommandItem
                  key={p.id}
                  value={p.display_name || p.name}
                  onSelect={() => { onSelect(p.id); setOpen(false); setQuery(''); }}
                  className="text-xs"
                >
                  <FolderOpen className="h-3.5 w-3.5 text-amber-500/80 shrink-0" />
                  <span className="truncate">{p.display_name || p.name}</span>
                  {p.id === activeProjectId && <Check className="h-3.5 w-3.5 ml-auto text-primary shrink-0" />}
                </CommandItem>
              ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}

function NewItemButton({ activeProjectId }: { activeProjectId: string | null }) {
  const router = useRouter();
  const [menuOpen, setMenuOpen] = useState(false);
  const [mode, setMode] = useState<'file' | 'folder' | null>(null);
  const [inputVal, setInputVal] = useState('');
  const [creating, setCreating] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);
  const projectPath = useStore((s) => s.codeProjectPath);
  const tok = typeof window !== 'undefined' ? (localStorage.getItem('qorven_token') || '') : '';

  useEffect(() => {
    if (mode) setTimeout(() => inputRef.current?.focus(), 50);
  }, [mode]);

  const create = async () => {
    const name = inputVal.trim();
    if (!name || !activeProjectId) return;
    setCreating(true);
    try {
      const path = name.startsWith('/') ? name : `${projectPath || ''}/${name}`;
      const content = mode === 'folder' ? '' : '';
      const isFolder = mode === 'folder';
      if (isFolder) {
        // Create a .gitkeep inside the folder so it exists
        await fetch(`${apiBase()}/v1/projects/${activeProjectId}/file`, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${tok}` },
          body: JSON.stringify({ path: `${path}/.gitkeep`, content: '' }),
        });
      } else {
        await fetch(`${apiBase()}/v1/projects/${activeProjectId}/file`, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${tok}` },
          body: JSON.stringify({ path, content }),
        });
        router.push(`/code?open=${encodeURIComponent(path)}`);
      }
      setMode(null);
      setInputVal('');
    } finally {
      setCreating(false);
    }
  };

  if (mode) {
    return (
      <div className="flex items-center gap-1">
        <input
          ref={inputRef}
          value={inputVal}
          onChange={e => setInputVal(e.target.value)}
          onKeyDown={e => { if (e.key === 'Enter') create(); if (e.key === 'Escape') { setMode(null); setInputVal(''); } }}
          placeholder={mode === 'file' ? 'filename.ts' : 'folder-name'}
          className="qr-input text-xs flex-1 w-24"
        />
        <button
          onClick={create}
          disabled={creating || !inputVal.trim()}
          className="flex h-6 w-6 items-center justify-center rounded bg-primary text-primary-foreground disabled:opacity-40"
        >
          {creating ? <Loader2 className="h-3 w-3 animate-spin" /> : <Check className="h-3 w-3" />}
        </button>
        <button onClick={() => { setMode(null); setInputVal(''); }} className="flex h-6 w-6 items-center justify-center rounded text-muted-foreground hover:bg-accent">
          <X className="h-3 w-3" />
        </button>
      </div>
    );
  }

  return (
    <Popover open={menuOpen} onOpenChange={setMenuOpen}>
      <PopoverTrigger asChild>
        <button
          className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md border border-border bg-background text-muted-foreground hover:bg-accent hover:text-foreground transition-colors"
          title="New…"
        >
          <Plus className="h-3.5 w-3.5" />
        </button>
      </PopoverTrigger>
      <PopoverContent className="w-44 p-1" align="end" sideOffset={4}>
        <button
          onClick={() => { setMenuOpen(false); useStore.getState().setCodeActiveProjectId(null); router.push('/code'); }}
          className="flex w-full items-center gap-2 rounded-sm px-2 py-1.5 text-xs text-foreground hover:bg-accent transition-colors"
        >
          <FolderPlus className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
          New project
        </button>
        {activeProjectId && (
          <>
            <button
              onClick={() => { setMenuOpen(false); setMode('file'); }}
              className="flex w-full items-center gap-2 rounded-sm px-2 py-1.5 text-xs text-foreground hover:bg-accent transition-colors"
            >
              <FilePlus className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
              New file
            </button>
            <button
              onClick={() => { setMenuOpen(false); setMode('folder'); }}
              className="flex w-full items-center gap-2 rounded-sm px-2 py-1.5 text-xs text-foreground hover:bg-accent transition-colors"
            >
              <FolderPlus className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
              New folder
            </button>
          </>
        )}
      </PopoverContent>
    </Popover>
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
