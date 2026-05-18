'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState } from 'react';
import { Search, Globe, FileText, Loader2, Check, Zap } from 'lucide-react';
import { cn } from '@/lib/utils';

interface Step {
  name: string;
  args?: string;
  result?: string;
  status: 'running' | 'done';
}

const toolIcons: Record<string, typeof Search> = {
  web_search: Search, web_fetch: Globe, file_read: FileText,
};

const toolLabels: Record<string, (args?: string) => string> = {
  web_search: (a) => `Searching${a ? ': ' + tryParse(a, 'query') : ''}`,
  web_fetch: (a) => `Reading ${tryParse(a, 'url')?.replace(/https?:\/\/(www\.)?/, '').split('/')[0] || 'page'}`,
  file_read: (a) => `Reading ${tryParse(a, 'path')?.split('/').pop() || 'file'}`,
  shell_exec: () => 'Running command',
  memory_search: () => 'Searching memory',
  execute_action: () => 'Calling API',
};

function tryParse(args: string | undefined, key: string): string {
  if (!args) return '';
  try {
    const d = JSON.parse(args);
    return d[key] || '';
  } catch { return ''; }
}

export function LiveStatus({ steps, isThinking }: { steps: Step[]; isThinking: boolean }) {
  if (!isThinking && steps.length === 0) return null;

  return (
    <div className="space-y-1.5 mb-3">
      {steps.map((step, i) => {
        const Icon = toolIcons[step.name] || Zap;
        const label = (toolLabels[step.name] || (() => step.name))(step.args);
        const done = step.status === 'done';

        return (
          <div key={i} className={cn(
            'flex items-center gap-2 text-xs transition-all duration-300',
            done ? 'text-muted-foreground/60' : 'text-foreground',
          )}>
            {done ? (
              <Check className="h-3 w-3 text-green-500 shrink-0" />
            ) : (
              <Loader2 className="h-3 w-3 animate-spin text-primary shrink-0" />
            )}
            <span className={cn(done && 'line-through decoration-muted-foreground/30')}>{label}</span>
          </div>
        );
      })}
      {isThinking && steps.length === 0 && (
        <div className="flex items-center gap-2 text-xs text-foreground">
          <Loader2 className="h-3 w-3 animate-spin text-primary shrink-0" />
          <span>Thinking...</span>
        </div>
      )}
    </div>
  );
}
