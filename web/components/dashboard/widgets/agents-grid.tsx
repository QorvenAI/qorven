'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useStore } from '@/store';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { cn } from '@/lib/utils';
import { registerWidget, type WidgetConfig } from '../widget-registry';
import type { SoulActivity } from '@/types';

const ACTIVITY_COLORS: Record<SoulActivity, string> = {
  idle:     'bg-muted-foreground',
  thinking: 'bg-amber-400',
  running:  'bg-emerald-500 animate-pulse',
  offline:  'bg-zinc-400',
  error:    'bg-rose-500',
};

function AgentsGridWidget({ config }: { config: WidgetConfig }) {
  const souls = useStore((s) => s.souls);
  const soulStates = useStore((s) => s.soulStates);

  const visible = souls.slice(0, 6);
  const overflow = souls.length - visible.length;

  return (
    <Card className="h-full flex flex-col">
      <CardHeader className="min-h-10 px-4 py-2">
        <CardTitle className="text-sm font-medium text-muted-foreground">{config.title}</CardTitle>
      </CardHeader>
      <CardContent className="flex-1 px-4 pb-3 pt-0">
        {souls.length === 0 ? (
          <p className="text-xs text-muted-foreground py-2">No agents found</p>
        ) : (
          <div className="grid grid-cols-2 gap-2">
            {visible.map((soul) => {
              const state = soulStates[soul.id];
              const activity: SoulActivity = state?.activity ?? 'idle';
              return (
                <div
                  key={soul.id}
                  className="flex items-center gap-2 rounded-lg border border-border bg-muted/40 px-3 py-2"
                >
                  <span className={cn('shrink-0 size-2 rounded-full', ACTIVITY_COLORS[activity])} />
                  <div className="min-w-0">
                    <p className="text-xs font-medium truncate">{soul.display_name}</p>
                    {state?.lastEvent && (
                      <p className="text-[10px] text-muted-foreground truncate">{state.lastEvent}</p>
                    )}
                  </div>
                </div>
              );
            })}
            {overflow > 0 && (
              <div className="flex items-center justify-center rounded-lg border border-dashed border-border px-3 py-2">
                <span className="text-xs text-muted-foreground">+{overflow} more</span>
              </div>
            )}
          </div>
        )}
      </CardContent>
    </Card>
  );
}

registerWidget('agents', AgentsGridWidget);
export { AgentsGridWidget };
