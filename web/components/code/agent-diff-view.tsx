'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import dynamic from 'next/dynamic';
import { Loader2, GitCompare, RotateCcw } from 'lucide-react';
import { cn } from '@/lib/utils';
import { detectLang } from './code-utils';

// Load DiffEditor client-side only — Monaco is browser-only.
const MonacoDiffEditor = dynamic(
  () => import('@monaco-editor/react').then(m => m.DiffEditor),
  {
    ssr: false,
    loading: () => (
      <div className="flex h-full items-center justify-center">
        <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
      </div>
    ),
  },
);

interface AgentDiffViewProps {
  /** Workspace-relative or absolute file path — used for language detection and display. */
  path: string;
  /** File content BEFORE the agent edit (the "original" / left side). */
  original: string;
  /** File content AFTER the agent edit (the "modified" / right side). */
  modified: string;
  /** Called when the user clicks "Revert agent changes". */
  onRevert?: () => void;
  /** Whether the revert operation is in-flight. */
  reverting?: boolean;
  className?: string;
}

/** AgentDiffView — shows a read-only Monaco side-by-side diff of an agent edit.
 *
 * Design: agent writes, user SEES the diff — no streaming, no accept-gate.
 * One-click revert delegates to the parent (projectUndo POST /projects/{id}/undo).
 */
export function AgentDiffView({
  path,
  original,
  modified,
  onRevert,
  reverting,
  className,
}: AgentDiffViewProps) {
  const name = path.split('/').pop() || path;
  const language = detectLang(path);

  return (
    <div className={cn('flex flex-col h-full overflow-hidden', className)}>
      {/* Header bar */}
      <div className="flex shrink-0 items-center gap-2 border-b border-border bg-muted/20 px-3 py-1.5">
        <GitCompare className="h-3.5 w-3.5 shrink-0 text-primary" />
        <span className="flex-1 truncate font-mono text-xs font-medium text-foreground">
          {name}
        </span>
        <span className="truncate text-xs text-muted-foreground max-w-xs hidden sm:block">
          {path}
        </span>
        <span className="rounded border border-border bg-muted px-1.5 py-0.5 text-xs text-muted-foreground">
          agent edit
        </span>
        {onRevert && (
          <button
            type="button"
            disabled={reverting}
            onClick={onRevert}
            className={cn(
              'flex items-center gap-1 rounded-md border border-border bg-muted/30 px-2 py-0.5',
              'text-xs font-medium text-destructive hover:bg-destructive/10',
              'transition-colors disabled:opacity-50 disabled:pointer-events-none',
            )}
          >
            {reverting ? (
              <Loader2 className="h-3 w-3 animate-spin" />
            ) : (
              <RotateCcw className="h-3 w-3" />
            )}
            Revert agent changes
          </button>
        )}
      </div>

      {/* Monaco diff editor — read-only, side-by-side, unchanged regions collapsed */}
      <div className="flex-1 overflow-hidden min-h-0">
        <MonacoDiffEditor
          height="100%"
          original={original}
          modified={modified}
          language={language}
          theme="vs-dark"
          options={{
            renderSideBySide: true,
            readOnly: true,
            hideUnchangedRegions: { enabled: true, minimumLineCount: 3, contextLineCount: 3 },
            fontSize: 13,
            fontFamily: '"JetBrains Mono", "Cascadia Code", "Fira Code", ui-monospace, monospace',
            fontLigatures: true,
            scrollBeyondLastLine: false,
            automaticLayout: true,
            minimap: { enabled: false },
            scrollbar: { verticalScrollbarSize: 8, horizontalScrollbarSize: 8 },
            padding: { top: 6, bottom: 6 },
            lineNumbers: 'on',
          }}
        />
      </div>
    </div>
  );
}
