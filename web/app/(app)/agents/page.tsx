'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useAgentsStream } from '@/hooks/use-agents-stream';
import { AgentDashboard } from '@/components/agents/AgentDashboard';
import { TaskFeed } from '@/components/agents/TaskFeed';
import { PlanApproval } from '@/components/agents/PlanApproval';
import { OrgChart } from '@/components/agents/OrgChart';
import { useStore } from '@/store';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/qor/tabs';
import { Bot, ListChecks, ClipboardCheck, Network } from 'lucide-react';
import { PageShell } from '@/components/layouts/page-shell';

export default function AgentsPage() {
  useAgentsStream();
  const pendingPlans = useStore(s =>
    Object.values(s.daemonPlans).filter(p => p.status === 'pending').length
  );

  return (
    <PageShell
      title="Agents"
      description="Monitor connected agents, track tasks, and approve plans"
      contentClassName="flex flex-col overflow-hidden px-0 py-0 sm:px-0"
    >
      <Tabs defaultValue="agents" className="flex flex-col h-full overflow-hidden">
        {/* Tab bar */}
        <div className="px-6 py-3 border-b border-border">
          <TabsList variant="default" size="sm">
            <TabsTrigger value="agents" className="gap-1.5">
              <Bot className="h-3.5 w-3.5" />
              Agents
            </TabsTrigger>
            <TabsTrigger value="tasks" className="gap-1.5">
              <ListChecks className="h-3.5 w-3.5" />
              Tasks
            </TabsTrigger>
            <TabsTrigger value="approvals" className="gap-1.5">
              <ClipboardCheck className="h-3.5 w-3.5" />
              Approvals
              {pendingPlans > 0 && (
                <span className="ml-0.5 rounded-full bg-amber-500/20 px-1.5 py-0.5 text-2xs font-semibold text-amber-400">
                  {pendingPlans}
                </span>
              )}
            </TabsTrigger>
            <TabsTrigger value="org-chart" className="gap-1.5">
              <Network className="h-3.5 w-3.5" />
              Org Chart
            </TabsTrigger>
          </TabsList>
        </div>

        {/* Tab content */}
        <div className="flex-1 overflow-y-auto px-6 py-5">
          <TabsContent value="agents" className="mt-0"><AgentDashboard /></TabsContent>
          <TabsContent value="tasks" className="mt-0"><TaskFeed /></TabsContent>
          <TabsContent value="approvals" className="mt-0"><PlanApproval /></TabsContent>
          <TabsContent value="org-chart" className="mt-0"><OrgChart /></TabsContent>
        </div>
      </Tabs>
    </PageShell>
  );
}
