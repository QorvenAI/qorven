'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

/**
 * StatusBar — 24px bar pinned to the bottom of the viewport (P9 T1.3).
 *
 * Single source of truth for "is the platform alive and what is it
 * running?". Replaces a fifth panel / extra rail icons for Terminal
 * + Models; those now appear here as live chips.
 *
 * Layout (left → right):
 *   • "Qorven" brand link  — links to qorven.ai
 *   • Version chip         — clicks open a changelog lightbox
 *   • spacer
 *   • Disconnect dot       — only visible when WS is offline
 *
 * Intentionally does NOT include breadcrumbs, titles, or page chrome —
 * that's the header's job.
 */

import Link from 'next/link';
import { useEffect, useRef, useState } from 'react';
import { usePathname } from 'next/navigation';
import { useStore } from '@/store';
import { X, ExternalLink } from 'lucide-react';

export function StatusBar() {
  const pathname = usePathname();
  const wsConnected = useStore((s) => s.wsConnected);
  const [version, setVersion] = useState<string>('');
  const [changelogOpen, setChangelogOpen] = useState(false);
  const [changelogMd, setChangelogMd] = useState<string>('');
  const modalRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    fetch('/api/health/detailed')
      .then((r) => r.ok ? r.json() : null)
      .then((d: { version?: string } | null) => {
        if (d?.version) {
          // Strip any leading "v" from the backend string — we add it ourselves.
          setVersion(d.version.replace(/^v/, ''));
        }
      })
      .catch(() => { /* leave empty */ });
  }, []);

  // Pages that paint their own bottom bar (e.g. /terminal's tmux footer)
  // set data-qorven-no-status-bar on the main canvas.
  const [hide, setHide] = useState(false);
  useEffect(() => {
    if (typeof document === 'undefined') return;
    const flagged = document.querySelector('[data-qorven-no-status-bar]');
    setHide(!!flagged);
  }, [pathname]);

  // Close modal on Escape
  useEffect(() => {
    if (!changelogOpen) return;
    const handler = (e: KeyboardEvent) => { if (e.key === 'Escape') setChangelogOpen(false); };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [changelogOpen]);

  const openChangelog = async () => {
    setChangelogOpen(true);
    if (changelogMd) return; // already loaded
    try {
      const r = await fetch('/api/v1/changelog');
      const d = await r.json();
      setChangelogMd(d.changelog ?? '');
    } catch {
      setChangelogMd('Failed to load changelog.');
    }
  };

  // Parse the first version section from the markdown.
  // Returns the heading + body up to (but not including) the next ## heading.
  const currentSection = (() => {
    if (!changelogMd || !version) return changelogMd;
    const lines = changelogMd.split('\n');
    let inSection = false;
    const out: string[] = [];
    for (const line of lines) {
      if (line.startsWith('## ')) {
        if (inSection) break; // hit next version section
        // Match flexibly: "v0.1.11-alpha" anywhere in the heading
        if (line.includes(version)) { inSection = true; out.push(line); }
      } else if (inSection) {
        out.push(line);
      }
    }
    return out.join('\n').trim() || changelogMd;
  })();

  if (hide) return null;

  return (
    <>
      <div
        className="qorven-status-bar fixed bottom-0 z-30 h-6 flex items-center gap-2 border-t border-border bg-muted px-2 text-2xs text-muted-foreground select-none"
        style={{ left: 'var(--rail-width)', width: 'var(--sidebar-default-width)' }}
      >
        {/* Brand — links to qorven.ai */}
        <Link
          href="https://qorven.ai"
          target="_blank"
          rel="noopener noreferrer"
          title="Visit qorven.ai"
          className="flex items-center px-1.5 h-full font-medium text-muted-foreground/60 hover:text-muted-foreground transition-colors rounded-sm hover:bg-accent"
        >
          Qorven
        </Link>

        {/* Version chip — opens changelog lightbox */}
        {version ? (
          <button
            onClick={openChangelog}
            title="View current changelog"
            className="flex items-center gap-1 px-1.5 h-full font-mono text-muted-foreground/50 hover:text-muted-foreground transition-colors tabular-nums rounded-sm hover:bg-accent cursor-pointer"
          >
            v{version}
          </button>
        ) : null}

        <div className="flex-1" />

        {/* Connection dot — subtle, no text */}
        {!wsConnected && (
          <span title="Disconnected — reconnecting" className="relative flex h-1.5 w-1.5 mr-1">
            <span className="relative inline-flex h-1.5 w-1.5 rounded-full bg-destructive/70" />
          </span>
        )}
      </div>

      {/* Changelog lightbox */}
      {changelogOpen && (
        <div
          className="fixed inset-0 z-50 flex items-end justify-start"
          style={{ paddingLeft: 'calc(var(--rail-width) + 8px)', paddingBottom: '32px' }}
          onClick={(e) => { if (e.target === e.currentTarget) setChangelogOpen(false); }}
        >
          <div
            ref={modalRef}
            role="dialog"
            aria-modal="true"
            aria-label="Changelog"
            className="relative bg-popover border border-border rounded-xl shadow-xl flex flex-col overflow-hidden"
            style={{ width: '420px', maxHeight: '480px' }}
          >
            {/* Header */}
            <div className="flex items-center justify-between px-4 py-2.5 border-b border-border shrink-0">
              <span className="text-xs font-semibold text-foreground">
                {version ? `What's new in v${version}` : "What's new"}
              </span>
              <div className="flex items-center gap-2">
                <Link
                  href="https://qorven.ai/changelog"
                  target="_blank"
                  rel="noopener noreferrer"
                  className="flex items-center gap-1 text-2xs text-muted-foreground hover:text-foreground transition-colors"
                >
                  Full changelog
                  <ExternalLink className="h-3 w-3" />
                </Link>
                <button
                  onClick={() => setChangelogOpen(false)}
                  className="flex items-center justify-center h-5 w-5 rounded text-muted-foreground hover:text-foreground hover:bg-accent transition-colors"
                >
                  <X className="h-3.5 w-3.5" />
                </button>
              </div>
            </div>

            {/* Body — rendered as plain text / simple markdown */}
            <div className="overflow-y-auto flex-1 px-4 py-3">
              {currentSection ? (
                <ChangelogBody markdown={currentSection} />
              ) : (
                <span className="text-xs text-muted-foreground">Loading…</span>
              )}
            </div>
          </div>
        </div>
      )}
    </>
  );
}

/** Lightweight markdown renderer for changelog content. */
function ChangelogBody({ markdown }: { markdown: string }) {
  const lines = markdown.split('\n');
  const nodes: React.ReactNode[] = [];
  let key = 0;

  for (const raw of lines) {
    const line = raw.trimEnd();
    if (line.startsWith('## ')) {
      nodes.push(
        <h2 key={key++} className="text-sm font-semibold text-foreground mt-1 mb-2">
          {line.slice(3)}
        </h2>
      );
    } else if (line.startsWith('### ')) {
      nodes.push(
        <h3 key={key++} className="text-xs font-semibold text-foreground/80 mt-3 mb-1 uppercase tracking-wide">
          {line.slice(4)}
        </h3>
      );
    } else if (line.startsWith('- **')) {
      // Bold title + description pattern: "- **Title** — description"
      const inner = line.slice(2);
      const m = inner.match(/^\*\*(.+?)\*\*(.*)$/);
      if (m) {
        nodes.push(
          <div key={key++} className="flex gap-1.5 text-xs mb-1 leading-relaxed">
            <span className="text-muted-foreground/50 shrink-0 mt-0.5">•</span>
            <span>
              <strong className="font-semibold text-foreground">{m[1]}</strong>
              <span className="text-muted-foreground">{m[2]}</span>
            </span>
          </div>
        );
      } else {
        nodes.push(
          <div key={key++} className="flex gap-1.5 text-xs mb-1 text-muted-foreground leading-relaxed">
            <span className="shrink-0 mt-0.5">•</span><span>{inner}</span>
          </div>
        );
      }
    } else if (line.startsWith('- ')) {
      nodes.push(
        <div key={key++} className="flex gap-1.5 text-xs mb-1 text-muted-foreground leading-relaxed">
          <span className="shrink-0 mt-0.5">•</span><span>{line.slice(2)}</span>
        </div>
      );
    } else if (line === '') {
      nodes.push(<div key={key++} className="h-1" />);
    } else {
      nodes.push(
        <p key={key++} className="text-xs text-muted-foreground mb-1 leading-relaxed">{line}</p>
      );
    }
  }

  return <>{nodes}</>;
}
