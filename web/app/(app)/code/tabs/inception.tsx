'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useState, useEffect, useCallback, useRef } from 'react';
import { Plus, Loader2, FolderOpen } from 'lucide-react';
import { projectBriefs as api } from '@/lib/api';
import { InceptionChat } from '@/components/code/inception-chat';
import { ProjectStudio } from '@/components/code/project-studio';
import type { ProjectBrief } from '@/types';
import { cn } from '@/lib/utils';

export function InceptionTab() {
  const [briefs, setBriefs] = useState<ProjectBrief[]>([]);
  const [active, setActive] = useState<ProjectBrief | null>(null);
  const [loading, setLoading] = useState(true);

  const load = useCallback(async (keepActiveId?: string) => {
    setLoading(true);
    try {
      const list = await api.list();
      setBriefs(list);
      if (keepActiveId) {
        const refreshed = list.find(b => b.id === keepActiveId);
        if (refreshed) setActive(refreshed);
      }
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { load(activeRef.current?.id); }, [load]);

  const activeRef = useRef<ProjectBrief | null>(null);
  useEffect(() => { activeRef.current = active; }, [active]);

  // Listen for backend project_updated WS events
  useEffect(() => {
    const handler = (e: Event) => {
      const data = (e as CustomEvent<{ id?: string }>).detail;
      if (data?.id) load(activeRef.current?.id);
    };
    window.addEventListener('qorven:project_updated', handler);
    return () => window.removeEventListener('qorven:project_updated', handler);
  }, [load]);

  const createNew = async () => {
    const brief = await api.create({ title: 'New Project', idea: '', quality: 'mvp' });
    setBriefs(prev => [brief, ...prev]);
    setActive(brief);
  };

  const onBriefUpdate = (updated: ProjectBrief) => {
    setActive(updated);
    setBriefs(prev => prev.map(b => b.id === updated.id ? updated : b));
  };

  return (
    <div className="flex h-full">
      {/* Left sidebar — brief list */}
      <div className="flex flex-col w-52 shrink-0 border-r border-border bg-muted/10">
        <div className="flex items-center gap-2 px-3 py-2.5 border-b border-border">
          <span className="flex-1 text-xs font-semibold text-muted-foreground uppercase tracking-wider">Projects</span>
          <button
            onClick={createNew}
            className="flex items-center justify-center h-5 w-5 rounded text-muted-foreground hover:text-foreground hover:bg-accent transition-colors"
            title="New project"
          >
            <Plus className="h-3.5 w-3.5" />
          </button>
        </div>
        <div className="flex-1 overflow-y-auto py-1">
          {loading ? (
            <div className="flex justify-center py-8">
              <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
            </div>
          ) : briefs.length === 0 ? (
            <div className="flex flex-col items-center gap-2 py-8 px-3 text-center">
              <FolderOpen className="h-6 w-6 text-muted-foreground/40" />
              <p className="text-xs text-muted-foreground/60">No projects yet</p>
              <button onClick={createNew} className="text-xs text-primary hover:underline">Start one</button>
            </div>
          ) : (
            briefs.map(b => (
              <button
                key={b.id}
                onClick={() => setActive(b)}
                className={cn(
                  'w-full text-left px-3 py-2 text-xs transition-colors',
                  active?.id === b.id
                    ? 'bg-primary/10 text-foreground'
                    : 'text-muted-foreground hover:text-foreground hover:bg-accent'
                )}
              >
                <div className="font-medium truncate">{b.title || 'Untitled'}</div>
                <div className="text-xs mt-0.5 opacity-60 capitalize">{b.status}</div>
              </button>
            ))
          )}
        </div>
      </div>

      {/* Main area */}
      {!active ? (
        <div className="flex flex-1 items-center justify-center">
          <div className="text-center space-y-3">
            <p className="text-sm text-muted-foreground">Select a project or start a new one</p>
            <button
              onClick={createNew}
              className="flex items-center gap-1.5 mx-auto rounded-lg bg-primary px-4 py-2 text-sm font-semibold text-primary-foreground hover:bg-primary/90 transition-colors"
            >
              <Plus className="h-4 w-4" />
              New Project
            </button>
          </div>
        </div>
      ) : (
        <div className="flex flex-1 overflow-hidden">
          {/* Chat panel */}
          <div className="flex flex-col w-[360px] shrink-0 border-r border-border">
            <InceptionChat brief={active} onBriefUpdate={onBriefUpdate} />
          </div>
          {/* Studio panel */}
          <div className="flex-1 overflow-hidden min-h-0">
            <ProjectStudio brief={active} onBriefUpdate={onBriefUpdate} />
          </div>
        </div>
      )}
    </div>
  );
}
