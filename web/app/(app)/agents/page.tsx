'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useAgentsStream } from '@/hooks/use-agents-stream';
import { AgentDashboard } from '@/components/agents/AgentDashboard';
import { TaskFeed } from '@/components/agents/TaskFeed';
import { PlanApproval } from '@/components/agents/PlanApproval';
import { useStore } from '@/store';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/qor/tabs';
import { Bot, ListChecks, ClipboardCheck } from 'lucide-react';

export default function AgentsPage() {
  useAgentsStream();
  const pendingPlans = useStore(s =>
    Object.values(s.daemonPlans).filter(p => p.status === 'pending').length
  );

  return (
    <div className="flex flex-col h-full">
      {/* Header */}
      <div className="border-b border-border px-6 py-4">
        <h1 className="text-base font-semibold">Agents</h1>
        <p className="text-xs text-muted-foreground mt-0.5">
          Monitor connected agents, track tasks, and approve plans
        </p>
      </div>

      <Tabs defaultValue="agents" className="flex flex-col flex-1 overflow-hidden">
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
          </TabsList>
        </div>

        {/* Tab content */}
        <div className="flex-1 overflow-y-auto px-6 py-5">
          <TabsContent value="agents" className="mt-0"><AgentDashboard /></TabsContent>
          <TabsContent value="tasks" className="mt-0"><TaskFeed /></TabsContent>
          <TabsContent value="approvals" className="mt-0"><PlanApproval /></TabsContent>
        </div>
      </Tabs>
    </div>
  );
}
