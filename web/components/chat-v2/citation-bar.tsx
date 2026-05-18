'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { ExternalLink } from 'lucide-react';
import { cn } from '@/lib/utils';

export interface Source {
  index?: number | string;
  title?: string;
  url: string;
  snippet?: string;
  source?: string;
}

function domain(url: string): string {
  try { return new URL(url).hostname.replace(/^www\./, ''); } catch { return url; }
}

function favicon(url: string): string {
  try { return `https://www.google.com/s2/favicons?domain=${new URL(url).hostname}&sz=16`; } catch { return ''; }
}

interface CitationBarProps {
  sources: Source[];
  className?: string;
}

export function CitationBar({ sources, className }: CitationBarProps) {
  if (!sources.length) return null;

  return (
    <div className={cn('mt-3 pt-3 border-t border-border/40', className)}>
      <p className="text-2xs font-medium text-muted-foreground mb-2 uppercase tracking-wide">Sources</p>
      <div className="flex flex-wrap gap-1.5">
        {sources.map((s, i) => {
          const idx = s.index ?? i + 1;
          const d = domain(s.url);
          const fav = favicon(s.url);
          return (
            <a
              key={i}
              href={s.url}
              target="_blank"
              rel="noopener noreferrer"
              title={s.title || s.url}
              className="group flex items-center gap-1.5 rounded-full border border-border/60 bg-card/60 px-2 py-1 text-xs text-muted-foreground hover:border-primary/30 hover:text-foreground transition-colors"
            >
              <span className="flex h-4 w-4 shrink-0 items-center justify-center rounded-full bg-muted text-2xs font-mono font-bold text-foreground/60">
                {idx}
              </span>
              {fav && (
                // eslint-disable-next-line @next/next/no-img-element
                <img src={fav} alt="" className="h-3 w-3 rounded-sm shrink-0" aria-hidden />
              )}
              <span className="max-w-[160px] truncate">{s.title || d}</span>
              <ExternalLink className="h-2.5 w-2.5 shrink-0 opacity-0 group-hover:opacity-60 transition-opacity" />
            </a>
          );
        })}
      </div>
    </div>
  );
}

// Transforms [1], [2] etc in markdown text into inline citation chips.
// Used inside ReactMarkdown's p component.
export function transformCitations(
  text: string,
  sources: Source[],
): React.ReactNode[] {
  const parts = text.split(/(\[\d+\])/g);
  return parts.map((part, i) => {
    const m = part.match(/^\[(\d+)\]$/);
    if (!m) return part;
    const idx = parseInt(m[1]!, 10);
    const source = sources.find((s) => Number(s.index ?? 0) === idx || Number(s.index ?? 0) === idx);
    if (source) {
      const fav = favicon(source.url);
      return (
        <a
          key={i}
          href={source.url}
          target="_blank"
          rel="noopener noreferrer"
          title={source.title || source.url}
          className="inline-flex items-center gap-0.5 mx-0.5 px-1.5 py-0 rounded-full bg-primary/10 text-primary text-2xs font-mono hover:bg-primary/20 transition-colors cursor-pointer"
        >
          {fav && <img src={fav} alt="" className="h-2.5 w-2.5 rounded-sm" aria-hidden />}
          {idx}
        </a>
      );
    }
    return (
      <span key={i} className="inline-flex items-center mx-0.5 px-1.5 py-0 rounded-full bg-muted/80 text-2xs font-mono text-muted-foreground">
        {m[1]}
      </span>
    );
  });
}
