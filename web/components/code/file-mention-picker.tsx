'use client';
// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useRef } from 'react';
import { File } from 'lucide-react';
import { Command, CommandEmpty, CommandItem, CommandList } from '@/components/ui/command';
import { ScrollArea } from '@/components/ui/scroll-area';
import { cn } from '@/lib/utils';
import { fileColor } from './code-utils';
import type { FileNode } from './code-types';

interface FileMentionPickerProps {
  query: string;
  files: FileNode[];
  onSelect: (path: string) => void;
  onClose: () => void;
  anchorRef: React.RefObject<HTMLElement>;
}

function flattenTree(nodes: FileNode[]): FileNode[] {
  const result: FileNode[] = [];
  for (const node of nodes) {
    if (node.type === 'file') result.push(node);
    if (node.children) result.push(...flattenTree(node.children));
  }
  return result;
}

function fuzzyMatch(query: string, path: string): boolean {
  if (!query) return true;
  const q = query.toLowerCase();
  const p = path.toLowerCase();
  // Simple substring match on both full path and filename
  const name = p.split('/').pop() || p;
  return name.includes(q) || p.includes(q);
}

export function FileMentionPicker({ query, files, onSelect, onClose, anchorRef }: FileMentionPickerProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const allFiles = flattenTree(files);
  const filtered = allFiles.filter(f => fuzzyMatch(query, f.path)).slice(0, 12);

  // Position the picker above the anchor
  useEffect(() => {
    const anchor = anchorRef.current;
    const container = containerRef.current;
    if (!anchor || !container) return;
    const rect = anchor.getBoundingClientRect();
    const height = container.offsetHeight;
    container.style.bottom = `${window.innerHeight - rect.top + 4}px`;
    container.style.left = `${rect.left}px`;
    container.style.width = `${rect.width}px`;
  });

  // Close on outside click
  useEffect(() => {
    function handleClick(e: MouseEvent) {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        onClose();
      }
    }
    document.addEventListener('mousedown', handleClick);
    return () => document.removeEventListener('mousedown', handleClick);
  }, [onClose]);

  if (filtered.length === 0) return null;

  return (
    <div
      ref={containerRef}
      className="fixed z-50 overflow-hidden rounded-xl border border-border bg-popover shadow-xl shadow-black/20"
    >
      <ScrollArea className="max-h-48">
        <Command>
          <CommandList>
            {filtered.length === 0 ? (
              <CommandEmpty className="py-3 text-center text-xs text-muted-foreground">No files found</CommandEmpty>
            ) : (
              filtered.map(f => {
                const name = f.path.split('/').pop() || f.path;
                const dir = f.path.includes('/') ? f.path.slice(0, f.path.lastIndexOf('/')) : '';
                return (
                  <CommandItem
                    key={f.path}
                    value={f.path}
                    onSelect={() => onSelect(f.path)}
                    className="flex cursor-pointer items-center gap-2 px-3 py-2 text-xs hover:bg-accent"
                  >
                    <File className={cn('h-3.5 w-3.5 shrink-0', fileColor(f.name))} />
                    <span className="font-medium text-foreground">{name}</span>
                    {dir && <span className="truncate font-mono text-2xs text-muted-foreground">{dir}</span>}
                  </CommandItem>
                );
              })
            )}
          </CommandList>
        </Command>
      </ScrollArea>
    </div>
  );
}
