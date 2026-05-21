'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useState, useEffect, useCallback, useRef } from 'react';
import { projectBriefs as api } from '@/lib/api';
import { InceptionChat } from '@/components/code/inception-chat';
import { ProjectStudio } from '@/components/code/project-studio';
import { useStore } from '@/store';
import type { ProjectBrief } from '@/types';

export function InceptionTab() {
  const activeBriefId = useStore((s) => s.activeBriefId);
  const setActiveBriefId = useStore((s) => s.setActiveBriefId);
  const [active, setActive] = useState<ProjectBrief | null>(null);
  const activeRef = useRef<ProjectBrief | null>(null);

  const loadBrief = useCallback(async (id: string) => {
    try {
      const list = await api.list();
      const found = list.find(b => b.id === id);
      if (found) { setActive(found); activeRef.current = found; }
    } catch { /* non-fatal */ }
  }, []);

  // When store changes brief ID, load it
  useEffect(() => {
    if (activeBriefId) {
      loadBrief(activeBriefId);
    } else {
      setActive(null);
      activeRef.current = null;
    }
  }, [activeBriefId, loadBrief]);

  // Listen for backend project_updated WS events
  useEffect(() => {
    const handler = (e: Event) => {
      const data = (e as CustomEvent<{ id?: string }>).detail;
      if (data?.id && activeRef.current?.id === data.id) {
        loadBrief(data.id);
      }
    };
    window.addEventListener('qorven:project_updated', handler);
    return () => window.removeEventListener('qorven:project_updated', handler);
  }, [loadBrief]);

  const onBriefUpdate = (updated: ProjectBrief) => {
    setActive(updated);
    activeRef.current = updated;
    // If brief was just created from InceptionChat, track it in store
    if (!activeBriefId) setActiveBriefId(updated.id);
  };

  if (!active) {
    return (
      <div className="flex h-full items-center justify-center">
        <div className="text-center space-y-2">
          <p className="text-sm text-muted-foreground">Select a project from the sidebar or create one</p>
          <button
            onClick={async () => {
              const brief = await api.create({ title: 'New Project', idea: '', quality: 'mvp' });
              setActiveBriefId(brief.id);
            }}
            className="text-xs text-primary hover:underline"
          >
            + New project
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="flex h-full overflow-hidden">
      {/* Chat panel — fixed width, left */}
      <div className="flex flex-col w-[360px] shrink-0 border-r border-border">
        <InceptionChat brief={active} onBriefUpdate={onBriefUpdate} />
      </div>
      {/* Studio canvas — fills rest */}
      <div className="flex-1 overflow-hidden min-h-0">
        <ProjectStudio brief={active} onBriefUpdate={onBriefUpdate} />
      </div>
    </div>
  );
}
