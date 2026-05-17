'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

// Inline citation chips.
//
// The agent emits prose with bracketed references like [1], [2]
// when it is quoting a web result. Today those render as plain text,
// which (a) distracts from the reply and (b) gives the reader no way
// to jump to the source. This module provides a tiny transformer
// that replaces bracketed numbers with a small superscript chip that:
//
//   - shows a tooltip with the source title + favicon on hover
//   - scrolls the citation panel at the end of the message into view
//     on click (and gently highlights the corresponding source pill)
//   - opens the source in a new tab on shift-click or middle-click
//
// Works as a text transformer so existing markdown (bold, italics,
// lists) keeps rendering as before.

import React from 'react';
import { cn } from '@/lib/utils';

interface Source {
  index: number | string;
  title: string;
  url: string;
  snippet?: string;
}

// Match bracketed digits: [1], [12], [1,2,3]. Does not match
// [hello], [link text](url), or markdown footnote refs ([^1]).
const CITATION_RE = /\[(\d+(?:\s*,\s*\d+)*)\]/g;

function buildIndex(sources: Source[]): Map<string, Source> {
  const m = new Map<string, Source>();
  for (const s of sources) {
    m.set(String(s.index), s);
  }
  return m;
}

// transformCitations takes a plain string and returns an array of
// strings and React elements with [N] replaced by chips. Returns the
// raw string unchanged when there are no citations.
export function transformCitations(
  text: string,
  sources: Source[] | undefined,
): React.ReactNode {
  if (!sources?.length || !text) return text;
  if (!CITATION_RE.test(text)) return text;

  const lookup = buildIndex(sources);
  const parts: React.ReactNode[] = [];
  let lastIndex = 0;

  CITATION_RE.lastIndex = 0;
  let match: RegExpExecArray | null;
  while ((match = CITATION_RE.exec(text)) !== null) {
    const start = match.index;
    const end = start + match[0].length;
    if (start > lastIndex) {
      parts.push(text.slice(lastIndex, start));
    }
    const nums = match[1]!.split(',').map((n) => n.trim());
    nums.forEach((n, i) => {
      const src = lookup.get(n);
      if (!src) {
        parts.push(`[${n}]`);
      } else {
        parts.push(<CitationChip key={`${start}-${i}`} source={src} />);
      }
      if (i < nums.length - 1) parts.push(' ');
    });
    lastIndex = end;
  }
  if (lastIndex < text.length) {
    parts.push(text.slice(lastIndex));
  }
  return parts;
}

function CitationChip({ source }: { source: Source }) {
  const domain = safeHost(source.url);
  const targetId = `citation-${source.index}`;

  const onClick = (e: React.MouseEvent) => {
    if (e.shiftKey || e.button === 1) {
      window.open(source.url, '_blank', 'noopener,noreferrer');
      return;
    }
    e.preventDefault();
    const el = document.getElementById(targetId);
    if (el) {
      el.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
      el.classList.add('citation-flash');
      setTimeout(() => el.classList.remove('citation-flash'), 1400);
    } else {
      window.open(source.url, '_blank', 'noopener,noreferrer');
    }
  };

  return (
    <a
      href={source.url}
      onClick={onClick}
      onAuxClick={onClick}
      title={`${source.title}\n${domain}`}
      className={cn(
        'inline-flex items-center justify-center align-super',
        'ml-0.5 h-4 min-w-[1.1rem] rounded-full px-1 text-xs font-semibold',
        'bg-primary/15 text-primary hover:bg-primary/25 transition-colors',
        'no-underline cursor-pointer select-none',
      )}
    >
      {source.index}
    </a>
  );
}

function safeHost(url: string): string {
  try {
    return new URL(url).hostname.replace(/^www\./, '');
  } catch {
    return url;
  }
}

// SourcePillWithAnchor — same shape as the existing citation pill,
// but with an id so the inline chip's scrollIntoView can target it.
export function SourcePillWithAnchor({ source }: { source: Source }) {
  const domain = safeHost(source.url);
  return (
    <a
      id={`citation-${source.index}`}
      href={source.url}
      target="_blank"
      rel="noopener noreferrer"
      className={cn(
        'group inline-flex items-center gap-1.5 rounded-full border border-border/60',
        'bg-muted/30 px-3 py-1.5 text-xs font-medium text-foreground/70',
        'hover:text-foreground hover:border-border hover:bg-muted/50',
        'transition-all cursor-pointer',
      )}
    >
      <span className="flex h-4 w-4 items-center justify-center rounded-full bg-primary/15 text-xs font-semibold text-primary">
        {source.index}
      </span>
      <img
        src={`https://www.google.com/s2/favicons?domain=${domain}&sz=16`}
        alt=""
        className="h-3.5 w-3.5 rounded-sm"
        onError={(e) => {
          (e.target as HTMLImageElement).style.display = 'none';
        }}
      />
      <span className="max-w-[160px] truncate">{source.title || domain}</span>
    </a>
  );
}
