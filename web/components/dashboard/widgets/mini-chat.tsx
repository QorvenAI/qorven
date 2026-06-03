'use client';
// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).
import { useRouter } from 'next/navigation';
import { registerWidget } from '@/components/dashboard/widget-registry';
import type { WidgetConfig } from '@/components/dashboard/widget-registry';
import { MessageSquare, ArrowUpRight } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { useStore } from '@/store';

function MiniChatWidget({ config }: { config: WidgetConfig }) {
  const router = useRouter();
  const souls = useStore(s => s.souls);
  // Use dataSource as agent ID hint, or just show first agent
  const agent = souls.find(s => s.id === config.dataSource || s.agent_key === config.dataSource) ?? souls[0];

  return (
    <div className="flex flex-col h-full p-1">
      <div className="flex items-center justify-between mb-2">
        <p className="text-xs font-medium text-muted-foreground truncate">{config.title}</p>
        {agent && (
          <Button variant="ghost" size="sm" className="h-6 px-2 text-[10px]"
            onClick={() => router.push(`/qors/${agent.id}`)}>
            Open <ArrowUpRight className="h-2.5 w-2.5 ml-0.5" />
          </Button>
        )}
      </div>
      <div className="flex-1 flex flex-col items-center justify-center gap-2">
        <MessageSquare className="h-8 w-8 text-muted-foreground/30" />
        {agent ? (
          <>
            <p className="text-xs text-muted-foreground">Chat with {agent.display_name}</p>
            <Button size="sm" className="h-7 text-xs"
              onClick={() => router.push(`/qors/${agent.id}`)}>
              Start conversation
            </Button>
          </>
        ) : (
          <p className="text-xs text-muted-foreground">No agent configured</p>
        )}
      </div>
    </div>
  );
}

registerWidget('chat' as never, MiniChatWidget as never);
export { MiniChatWidget };
