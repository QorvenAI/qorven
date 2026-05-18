'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

/**
 * BottomDrawer — VSCode-style panel pinned to the bottom of the main
 * canvas. Single global instance, registered tabs per page (P9 T1.2).
 *
 * Ownership model:
 * - State (open, active tab id, registered tab descriptors, height) lives
 *   in the zustand store so any page / header icon / keyboard shortcut
 *   can control it without prop drilling.
 * - Tab descriptors are metadata only (id, label, icon, order). The
 *   actual React subtree for a tab body is rendered via {@link BottomDrawerTab}
 *   from the page that owns it — portaled into the slot this component
 *   exposes. That keeps state in the page (effects, refs, streams) while
 *   the chrome stays centralized.
 *
 * Pages use it like:
 *
 *     <BottomDrawerTab id="terminal" label="Terminal" iconName="SquareTerminal" order={10}>
 *       <MyTerminal />
 *     </BottomDrawerTab>
 *
 * On unmount the tab is removed from the registry and, if it was the
 * only tab, the drawer auto-closes (see store.unregisterBottomDrawerTab).
 */

import { useEffect, useRef, useState, useCallback, type ReactNode } from 'react';
import { createPortal } from 'react-dom';
import {
  ChevronDown, X,
  SquareTerminal, Zap, FileText, Users, AlertCircle, Bot,
  type LucideIcon,
} from 'lucide-react';
import { useStore, type BottomDrawerTab as TabDescriptor } from '@/store';
import { cn } from '@/lib/utils';

// Icon registry — pages pass iconName strings (so descriptors stay
// serializable in the store); we map them to components here. Extend
// as new tabs are added. Unknown names fall back to FileText.
const ICONS: Record<string, LucideIcon> = {
  SquareTerminal,
  Zap,
  FileText,
  Users,
  AlertCircle,
  Bot,
};

const MIN_H = 120;
const MAX_H = 800;

export function BottomDrawer() {
  const open = useStore((s) => s.bottomDrawerOpen);
  const heightPx = useStore((s) => s.bottomDrawerHeightPx);
  const tabs = useStore((s) => s.bottomDrawerTabs);
  const activeTabId = useStore((s) => s.bottomDrawerActiveTabId);
  const setActive = useStore((s) => s.setBottomDrawerActiveTab);
  const close = useStore((s) => s.closeBottomDrawer);
  const setHeight = useStore((s) => s.setBottomDrawerHeight);
  const toggleDrawer = useStore((s) => s.toggleBottomDrawer);

  // Keyboard shortcut: Ctrl/Cmd+J toggles the drawer (matches VSCode).
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'j') {
        e.preventDefault();
        toggleDrawer();
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [toggleDrawer]);

  // Drag-to-resize handle. Sets a CSS transform during drag for 60fps
  // feedback and commits the value to the store on mouseup, so we don't
  // thrash every other subscriber while dragging.
  const dragStartRef = useRef<{ startY: number; startH: number } | null>(null);
  const [draftH, setDraftH] = useState<number | null>(null);

  const onDragStart = useCallback(
    (e: React.MouseEvent) => {
      e.preventDefault();
      dragStartRef.current = { startY: e.clientY, startH: heightPx };
      setDraftH(heightPx);

      const onMove = (ev: MouseEvent) => {
        if (!dragStartRef.current) return;
        const delta = dragStartRef.current.startY - ev.clientY;
        const next = Math.max(MIN_H, Math.min(MAX_H, dragStartRef.current.startH + delta));
        setDraftH(next);
      };
      const onUp = () => {
        if (dragStartRef.current && draftHRef.current != null) {
          setHeight(draftHRef.current);
        }
        dragStartRef.current = null;
        setDraftH(null);
        window.removeEventListener('mousemove', onMove);
        window.removeEventListener('mouseup', onUp);
      };
      window.addEventListener('mousemove', onMove);
      window.addEventListener('mouseup', onUp);
    },
    [heightPx, setHeight],
  );

  // Keep the latest draftH accessible inside onUp without re-creating
  // handlers. Ref is enough because onUp captures dragStartRef already.
  const draftHRef = useRef<number | null>(null);
  draftHRef.current = draftH;

  if (!open || tabs.length === 0) return null;

  const effectiveH = draftH ?? heightPx;
  const activeTab = tabs.find((t) => t.id === activeTabId) ?? tabs[0]!;

  return (
    <div
      className="qorven-bottom-drawer fixed left-[var(--rail-width)] right-0 bottom-0 z-20 flex flex-col border-t border-border bg-background shadow-[0_-2px_8px_rgba(0,0,0,0.2)]"
      style={{ height: effectiveH }}
    >
      {/* Resize handle — 4px tall invisible bar on top edge */}
      <div
        onMouseDown={onDragStart}
        className="absolute left-0 right-0 -top-[3px] h-[6px] cursor-row-resize hover:bg-primary/30 z-10"
        title="Drag to resize"
      />

      {/* Tab bar */}
      <div className="flex items-center border-b border-border bg-muted/20 shrink-0 select-none">
        <div className="flex items-stretch flex-1 overflow-x-auto scrollbar-none">
          {tabs.map((t) => (
            <TabButton
              key={t.id}
              tab={t}
              active={t.id === activeTab.id}
              onClick={() => setActive(t.id)}
            />
          ))}
        </div>
        <button
          title="Close panel (⌘J)"
          onClick={close}
          className="h-7 w-7 flex items-center justify-center text-muted-foreground hover:text-foreground hover:bg-accent mr-1"
        >
          <ChevronDown className="h-4 w-4" />
        </button>
      </div>

      {/* Tab body — each BottomDrawerTab portals into the slot whose
          data-tab-id matches. Only the active tab's slot is visible;
          inactive slots stay mounted (hidden) so editor/terminal state
          isn't thrown away when the user switches tabs. */}
      <div className="flex-1 min-h-0 relative">
        {tabs.map((t) => (
          <div
            key={t.id}
            data-tab-id={t.id}
            data-bottom-drawer-slot=""
            className={cn(
              'absolute inset-0 overflow-hidden',
              t.id === activeTab.id ? 'block' : 'hidden',
            )}
          />
        ))}
      </div>
    </div>
  );
}

function TabButton({
  tab,
  active,
  onClick,
}: {
  tab: TabDescriptor;
  active: boolean;
  onClick: () => void;
}) {
  const Icon = tab.iconName ? ICONS[tab.iconName] ?? FileText : FileText;
  return (
    <button
      onClick={onClick}
      className={cn(
        'flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium border-b-2 whitespace-nowrap transition-colors',
        active
          ? 'border-primary text-foreground bg-background'
          : 'border-transparent text-muted-foreground hover:text-foreground',
      )}
    >
      <Icon className="h-3.5 w-3.5" />
      {tab.label}
      {tab.badge != null && (
        <span className="ml-1 rounded-full bg-primary/20 px-1.5 py-0 text-xs font-semibold text-primary">
          {tab.badge}
        </span>
      )}
    </button>
  );
}

/**
 * BottomDrawerTab — page-side. Registers a tab in the drawer on mount
 * and portals its children into the drawer's matching slot. Unregisters
 * on unmount (drawer auto-closes if this was the only tab).
 *
 * Caller is responsible for stable `id` across renders. Re-rendering
 * with the same id is cheap; changing id on the fly will unmount the
 * old tab and mount a new one.
 */
export function BottomDrawerTab({
  id,
  label,
  iconName,
  order,
  badge,
  children,
}: {
  id: string;
  label: string;
  iconName?: string;
  order?: number;
  badge?: number | string;
  children: ReactNode;
}) {
  const register = useStore((s) => s.registerBottomDrawerTab);
  const unregister = useStore((s) => s.unregisterBottomDrawerTab);
  const drawerOpen = useStore((s) => s.bottomDrawerOpen);

  const [slot, setSlot] = useState<HTMLElement | null>(null);

  // Register / unregister tab descriptor.
  useEffect(() => {
    register({ id, label, iconName, order, badge });
    return () => unregister(id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id, label, iconName, order, badge]);

  // Locate the drawer slot once it renders. Poll on every paint while
  // the drawer is open and the slot isn't found — covers the common
  // case where the page mounts before the drawer is first opened.
  useEffect(() => {
    if (!drawerOpen) {
      setSlot(null);
      return;
    }
    let raf = 0;
    const find = () => {
      const el = document.querySelector<HTMLElement>(
        `[data-bottom-drawer-slot][data-tab-id="${CSS.escape(id)}"]`,
      );
      if (el) setSlot(el);
      else raf = requestAnimationFrame(find);
    };
    find();
    return () => cancelAnimationFrame(raf);
  }, [drawerOpen, id]);

  if (!slot) return null;
  return createPortal(children, slot);
}
