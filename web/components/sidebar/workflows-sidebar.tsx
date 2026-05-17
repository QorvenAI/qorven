'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useEffect, useState } from 'react';
import { workflows as workflowsApi } from '@/lib/api';
import { GitBranch } from 'lucide-react';
import { SidebarMenuItem, SidebarGroupTitle } from './sidebar-primitives';

export function WorkflowsSidebar() {
  const [workflowList, setWorkflowList] = useState<any[]>([]);
  useEffect(() => {
    workflowsApi.list()
      .then((list) => setWorkflowList(Array.isArray(list) ? list : []))
      .catch(() => setWorkflowList([]));
  }, []);
  return (
    <>
      <SidebarGroupTitle>Workflows</SidebarGroupTitle>
      <ul className="flex flex-col gap-px px-2.5">
        {workflowList.map((w: any) => (
          <SidebarMenuItem key={w.id} icon={GitBranch} label={w.name}
            badge={w.enabled ? 'enabled' : 'disabled'}
            badgeColor={w.enabled ? 'bg-emerald-400/10 text-emerald-400' : 'bg-muted text-muted-foreground'} />
        ))}
        {workflowList.length === 0 && <p className="px-2.5 py-4 text-2sm text-muted-foreground">No workflows</p>}
      </ul>
    </>
  );
}
