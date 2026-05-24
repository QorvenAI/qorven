'use client';
// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useRef } from 'react';
import {
  Plus, Undo2, Download, FilePlus, Lock, GitCompare, FileCode2, Eraser
} from 'lucide-react';
import { Command, CommandEmpty, CommandGroup, CommandItem, CommandList, CommandSeparator } from '@/components/ui/command';
import { ScrollArea } from '@/components/ui/scroll-area';

export interface SlashCommand {
  id: string;
  name: string;
  description: string;
  icon: React.ReactNode;
  group: 'session' | 'context' | 'view';
}

const COMMANDS: SlashCommand[] = [
  { id: 'new',    name: '/new',    description: 'Start a new session',              icon: <Plus className="h-3.5 w-3.5" />,       group: 'session' },
  { id: 'undo',   name: '/undo',   description: 'Revert last agent change',         icon: <Undo2 className="h-3.5 w-3.5" />,      group: 'session' },
  { id: 'clear',  name: '/clear',  description: 'Clear build log',                  icon: <Eraser className="h-3.5 w-3.5" />,     group: 'session' },
  { id: 'export', name: '/export', description: 'Download session as Markdown',     icon: <Download className="h-3.5 w-3.5" />,   group: 'session' },
  { id: 'add',    name: '/add',    description: 'Add a file to context',            icon: <FilePlus className="h-3.5 w-3.5" />,   group: 'context' },
  { id: 'plan',   name: '/plan',   description: 'Toggle plan mode (no file writes)',icon: <Lock className="h-3.5 w-3.5" />,       group: 'context' },
  { id: 'diff',   name: '/diff',   description: 'Open diff view',                   icon: <GitCompare className="h-3.5 w-3.5" />, group: 'view' },
  { id: 'init',   name: '/init',   description: 'Generate AGENTS.md for this project',icon: <FileCode2 className="h-3.5 w-3.5" />,group: 'view' },
];

interface SlashCommandPaletteProps {
  query: string;
  onSelect: (cmd: SlashCommand) => void;
  onClose: () => void;
  anchorRef: React.RefObject<HTMLElement>;
}

export function SlashCommandPalette({ query, onSelect, onClose, anchorRef }: SlashCommandPaletteProps) {
  const containerRef = useRef<HTMLDivElement>(null);

  const filtered = COMMANDS.filter(
    c => !query || c.id.startsWith(query.toLowerCase()) || c.name.includes(query.toLowerCase()),
  );

  const groups = ['session', 'context', 'view'] as const;

  // Position above anchor
  useEffect(() => {
    const anchor = anchorRef.current;
    const container = containerRef.current;
    if (!anchor || !container) return;
    const rect = anchor.getBoundingClientRect();
    container.style.bottom = `${window.innerHeight - rect.top + 4}px`;
    container.style.left = `${rect.left}px`;
    container.style.width = `${rect.width}px`;
  });

  useEffect(() => {
    function handleClick(e: MouseEvent) {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        onClose();
      }
    }
    function handleKeyDown(e: KeyboardEvent) {
      if (e.key === 'Escape') onClose();
    }
    document.addEventListener('mousedown', handleClick);
    document.addEventListener('keydown', handleKeyDown);
    return () => {
      document.removeEventListener('mousedown', handleClick);
      document.removeEventListener('keydown', handleKeyDown);
    };
  }, [onClose]);

  if (filtered.length === 0) return null;

  return (
    <div
      ref={containerRef}
      className="fixed z-50 overflow-hidden rounded-xl border border-border bg-popover shadow-xl shadow-black/20"
    >
      <ScrollArea className="max-h-64">
        <Command>
          <CommandList>
            <CommandEmpty className="py-3 text-center text-xs text-muted-foreground">No commands found</CommandEmpty>
            {groups.map((group, gi) => {
              const items = filtered.filter(c => c.group === group);
              if (items.length === 0) return null;
              const labels: Record<string, string> = { session: 'Session', context: 'Context', view: 'View' };
              return (
                <CommandGroup key={group} heading={labels[group]}>
                  {gi > 0 && <CommandSeparator />}
                  {items.map(cmd => (
                    <CommandItem
                      key={cmd.id}
                      value={cmd.id}
                      onSelect={() => onSelect(cmd)}
                      className="flex cursor-pointer items-center gap-2 px-3 py-2 hover:bg-accent"
                    >
                      <span className="flex h-5 w-5 shrink-0 items-center justify-center text-muted-foreground">
                        {cmd.icon}
                      </span>
                      <span className="text-xs font-medium text-foreground font-mono">{cmd.name}</span>
                      <span className="ml-1 flex-1 truncate text-2xs text-muted-foreground">{cmd.description}</span>
                    </CommandItem>
                  ))}
                </CommandGroup>
              );
            })}
          </CommandList>
        </Command>
      </ScrollArea>
    </div>
  );
}
