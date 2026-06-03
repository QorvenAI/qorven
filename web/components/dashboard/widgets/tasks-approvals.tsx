'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useState } from 'react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { useDashboardData } from '@/contexts/dashboard-data';
import { registerWidget, type WidgetConfig } from '../widget-registry';
import { request } from '@/lib/api-core';

interface ApprovalTask {
  id: string;
  description: string;
  agent?: string;
}

function TasksApprovalsWidget({ config }: { config: WidgetConfig }) {
  const { data } = useDashboardData();
  const raw = data['pending_approvals'];
  const items: ApprovalTask[] = Array.isArray(raw) ? (raw as ApprovalTask[]) : [];
  const [acting, setActing] = useState<string | null>(null);

  async function handleAction(id: string, decision: 'allow' | 'deny') {
    setActing(id);
    try {
      await request(`/approvals/${id}/respond`, {
        method: 'POST',
        body: JSON.stringify({ decision }),
      });
    } catch {
      // silently fail — WS will update state
    } finally {
      setActing(null);
    }
  }

  return (
    <Card className="h-full flex flex-col">
      <CardHeader className="min-h-10 px-4 py-2">
        <CardTitle className="text-sm font-medium text-muted-foreground">{config.title}</CardTitle>
      </CardHeader>
      <CardContent className="flex-1 overflow-y-auto px-4 pb-3 pt-0">
        {items.length === 0 ? (
          <p className="text-xs text-muted-foreground py-2">No pending approvals</p>
        ) : (
          <ul className="space-y-2">
            {items.map((item) => (
              <li key={item.id} className="rounded-lg border border-border bg-muted/40 px-3 py-2">
                <p className="text-xs text-foreground leading-relaxed mb-2">{item.description}</p>
                {item.agent && (
                  <p className="text-[10px] text-muted-foreground mb-2">Agent: {item.agent}</p>
                )}
                <div className="flex gap-2">
                  <Button
                    variant="primary"
                    size="sm"
                    disabled={acting === item.id}
                    onClick={() => handleAction(item.id, 'allow')}
                    className="h-6 text-xs px-2"
                  >
                    Approve
                  </Button>
                  <Button
                    variant="destructive"
                    size="sm"
                    disabled={acting === item.id}
                    onClick={() => handleAction(item.id, 'deny')}
                    className="h-6 text-xs px-2"
                  >
                    Reject
                  </Button>
                </div>
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  );
}

registerWidget('tasks', TasksApprovalsWidget);
export { TasksApprovalsWidget };
