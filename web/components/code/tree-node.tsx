'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { type MouseEvent, useState } from 'react';
import { ChevronDown, ChevronRight, File, FolderOpen } from 'lucide-react';
import { cn } from '@/lib/utils';
import { fileColor } from './code-utils';
import type { FileNode } from './code-types';

export function TreeNode({ node, depth, selected, onSelect, changedPaths, onContextMenu }: {
  node: FileNode; depth: number; selected: string; onSelect: (p: string) => void;
  changedPaths?: Set<string>;
  onContextMenu?: (path: string, type: 'file' | 'directory', e: MouseEvent) => void;
}) {
  const [open, setOpen] = useState(depth === 0);
  const isDir = node.type === 'dir';
  const active = node.path === selected;
  const changed = changedPaths?.has(node.path) ?? false;
  return (
    <>
      <button
        onClick={() => isDir ? setOpen(!open) : onSelect(node.path)}
        onContextMenu={onContextMenu ? (e: MouseEvent<HTMLButtonElement>) => onContextMenu(node.path, isDir ? 'directory' : 'file', e) : undefined}
        className={cn(
          'flex w-full items-center gap-1 py-0.5 pr-2 text-xs transition-colors select-none',
          active ? 'bg-accent text-accent-foreground' : 'text-muted-foreground hover:text-foreground hover:bg-accent/50'
        )}
        style={{ paddingLeft: `${depth * 12 + 6}px` }}
      >
        {isDir
          ? <>{open ? <ChevronDown className="h-3 w-3 shrink-0" /> : <ChevronRight className="h-3 w-3 shrink-0" />}<FolderOpen className="h-3.5 w-3.5 shrink-0 text-amber-500/80" /></>
          : <><span className="w-3 shrink-0" /><File className={cn('h-3.5 w-3.5 shrink-0', fileColor(node.name))} /></>}
        <span className="truncate leading-[20px]">{node.name}</span>
        {changed && <span className="ml-auto shrink-0 w-1.5 h-1.5 rounded-full bg-primary" />}
      </button>
      {isDir && open && node.children?.map(c => (
        <TreeNode key={c.path} node={c} depth={depth + 1} selected={selected} onSelect={onSelect}
          changedPaths={changedPaths} onContextMenu={onContextMenu} />
      ))}
    </>
  );
}
