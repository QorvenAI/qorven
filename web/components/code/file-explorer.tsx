'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { type MouseEvent as ReactMouseEvent, useEffect, useRef, useState } from 'react';
import { FilePlus, FolderPlus, RefreshCw } from 'lucide-react';
import { cn } from '@/lib/utils';
import { projectFiles } from '@/lib/api-workspace';
import { TreeNode } from './tree-node';
import type { FileNode } from './code-types';

interface FileExplorerProps {
  projectId: string;
  tree: FileNode[];
  activePath: string;
  changedPaths?: Set<string>;
  onFileClick: (path: string) => void;
  onTreeRefresh: () => void;
}

interface ContextMenu {
  x: number;
  y: number;
  path: string;
  type: 'file' | 'directory';
}

export function FileExplorer({
  projectId,
  tree,
  activePath,
  changedPaths,
  onFileClick,
  onTreeRefresh,
}: FileExplorerProps) {
  const [menu, setMenu] = useState<ContextMenu | null>(null);
  const menuRef = useRef<HTMLDivElement>(null);

  // Close menu on outside click or Escape
  useEffect(() => {
    if (!menu) return;
    const handleClick = (e: globalThis.MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        setMenu(null);
      }
    };
    const handleKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setMenu(null);
    };
    document.addEventListener('mousedown', handleClick);
    document.addEventListener('keydown', handleKey);
    return () => {
      document.removeEventListener('mousedown', handleClick);
      document.removeEventListener('keydown', handleKey);
    };
  }, [menu]);

  const handleContextMenu = (path: string, type: 'file' | 'directory', e: ReactMouseEvent) => {
    e.preventDefault();
    e.stopPropagation();
    setMenu({ x: e.clientX, y: e.clientY, path, type });
  };

  const handleNewFile = async (parentPath?: string) => {
    setMenu(null);
    const name = window.prompt('New file name:');
    if (!name?.trim()) return;
    const base = parentPath ?? '';
    const newPath = base ? `${base}/${name.trim()}` : name.trim();
    try {
      await projectFiles.mkdir(projectId, newPath.split('/').slice(0, -1).join('/') || '.');
    } catch {}
    onTreeRefresh();
  };

  const handleNewFolder = async (parentPath?: string) => {
    setMenu(null);
    const name = window.prompt('New folder name:');
    if (!name?.trim()) return;
    const base = parentPath ?? '';
    const newPath = base ? `${base}/${name.trim()}` : name.trim();
    try {
      await projectFiles.mkdir(projectId, newPath);
    } catch {}
    onTreeRefresh();
  };

  const handleRename = async (path: string) => {
    setMenu(null);
    const parts = path.split('/');
    const oldName = parts[parts.length - 1];
    const newName = window.prompt('Rename to:', oldName);
    if (!newName?.trim() || newName.trim() === oldName) return;
    const newPath = [...parts.slice(0, -1), newName.trim()].join('/');
    try {
      await projectFiles.rename(projectId, path, newPath);
    } catch {}
    onTreeRefresh();
  };

  const handleDelete = async (path: string, type: 'file' | 'directory') => {
    setMenu(null);
    const label = type === 'directory' ? 'folder' : 'file';
    const name = path.split('/').pop() ?? path;
    if (!window.confirm(`Delete ${label} "${name}"?`)) return;
    try {
      await projectFiles.delete(projectId, path);
    } catch {}
    onTreeRefresh();
  };

  const menuNode = menu;

  return (
    <div className="flex flex-col h-full overflow-hidden">
      {/* Header */}
      <div className="flex shrink-0 items-center justify-between border-b border-border px-2 py-1">
        <span className="text-xs font-semibold text-muted-foreground uppercase tracking-wider">Files</span>
        <div className="flex items-center gap-0.5">
          <button
            onClick={() => handleNewFile()}
            title="New file"
            className="flex items-center justify-center rounded p-1 text-muted-foreground hover:text-foreground hover:bg-accent/50 transition-colors"
          >
            <FilePlus className="h-3.5 w-3.5" />
          </button>
          <button
            onClick={() => handleNewFolder()}
            title="New folder"
            className="flex items-center justify-center rounded p-1 text-muted-foreground hover:text-foreground hover:bg-accent/50 transition-colors"
          >
            <FolderPlus className="h-3.5 w-3.5" />
          </button>
          <button
            onClick={onTreeRefresh}
            title="Refresh"
            className="flex items-center justify-center rounded p-1 text-muted-foreground hover:text-foreground hover:bg-accent/50 transition-colors"
          >
            <RefreshCw className="h-3 w-3" />
          </button>
        </div>
      </div>

      {/* Tree */}
      <div className="flex-1 overflow-y-auto py-1 scrollbar-none">
        {tree.length === 0 ? (
          <p className="px-3 py-4 text-xs text-muted-foreground/60 text-center">No files</p>
        ) : (
          tree.map(node => (
            <TreeNode
              key={node.path}
              node={node}
              depth={0}
              selected={activePath}
              onSelect={onFileClick}
              changedPaths={changedPaths}
              onContextMenu={handleContextMenu}
            />
          ))
        )}
      </div>

      {/* Context menu */}
      {menuNode && (
        <div
          ref={menuRef}
          role="menu"
          className={cn(
            'fixed z-50 min-w-[140px] overflow-hidden rounded-md border border-border bg-popover py-1 shadow-md',
          )}
          style={{ top: menuNode.y, left: menuNode.x }}
        >
          {menuNode.type === 'directory' && (
            <>
              <button
                role="menuitem"
                onClick={() => handleNewFile(menuNode.path)}
                className="flex w-full items-center px-3 py-1.5 text-xs text-foreground hover:bg-accent transition-colors"
              >
                New File
              </button>
              <button
                role="menuitem"
                onClick={() => handleNewFolder(menuNode.path)}
                className="flex w-full items-center px-3 py-1.5 text-xs text-foreground hover:bg-accent transition-colors"
              >
                New Folder
              </button>
              <div className="my-1 border-t border-border" />
            </>
          )}
          <button
            role="menuitem"
            onClick={() => handleRename(menuNode.path)}
            className="flex w-full items-center px-3 py-1.5 text-xs text-foreground hover:bg-accent transition-colors"
          >
            Rename
          </button>
          <button
            role="menuitem"
            onClick={() => handleDelete(menuNode.path, menuNode.type)}
            className="flex w-full items-center px-3 py-1.5 text-xs text-destructive hover:bg-destructive/10 transition-colors"
          >
            Delete
          </button>
        </div>
      )}
    </div>
  );
}
