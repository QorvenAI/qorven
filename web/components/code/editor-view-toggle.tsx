'use client';
// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { Code, GitCompare, Monitor, Package } from 'lucide-react';
import { cn } from '@/lib/utils';

export type EditorView = 'code' | 'diff' | 'preview' | 'app';

interface EditorViewToggleProps {
  view: EditorView;
  onChange: (view: EditorView) => void;
  hasDiffs?: boolean;
  hasPreview?: boolean;
  isQorvenApp?: boolean;
}

const VIEWS: { id: EditorView; label: string; Icon: React.ComponentType<{ className?: string }> }[] = [
  { id: 'code',    label: 'Code',    Icon: Code },
  { id: 'diff',    label: 'Diff',    Icon: GitCompare },
  { id: 'preview', label: 'Preview', Icon: Monitor },
  { id: 'app',     label: 'App',     Icon: Package },
];

export function EditorViewToggle({ view, onChange, hasDiffs, hasPreview, isQorvenApp }: EditorViewToggleProps) {
  return (
    <div className="flex items-center rounded-lg border border-border bg-muted/30 p-0.5 gap-0.5">
      {VIEWS.map(({ id, label, Icon }) => {
        const active = view === id;
        const disabled =
          (id === 'diff' && !hasDiffs) ||
          (id === 'preview' && !hasPreview) ||
          (id === 'app' && !isQorvenApp);
        if (id === 'app' && !isQorvenApp) return null;
        return (
          <button
            key={id}
            type="button"
            disabled={disabled}
            onClick={() => onChange(id)}
            title={disabled ? (id === 'diff' ? 'No pending changes' : 'No preview available') : label}
            className={cn(
              'flex items-center gap-1.5 rounded-md px-2.5 py-1 text-xs font-medium transition-all',
              active
                ? 'bg-background text-foreground shadow-sm'
                : 'text-muted-foreground hover:text-foreground',
              disabled && 'pointer-events-none opacity-40',
            )}
          >
            <Icon className="h-3.5 w-3.5 shrink-0" />
            {label}
          </button>
        );
      })}
    </div>
  );
}
