'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import React, { useState, useEffect, useRef, useMemo } from 'react';
import { cn } from '@/lib/utils';
import { Copy, Check, ChevronUp, ChevronDown, ChevronsUpDown } from 'lucide-react';
import type { Highlighter, BundledLanguage } from 'shiki';

interface CodeBlockProps {
  language: string;
  code: string;
}

// Shared highlighter instance — Shiki is heavy (~400KB gzipped with
// grammars), so we lazy-load on first render and cache the promise so
// every code block reuses one instance.
let shikiPromise: Promise<Highlighter> | null = null;
const STARTER_LANGS: BundledLanguage[] = [
  'bash', 'shell', 'go', 'typescript', 'javascript', 'tsx', 'jsx',
  'python', 'json', 'yaml', 'toml', 'markdown', 'html', 'css',
  'sql', 'dockerfile', 'diff',
];
async function getShiki(): Promise<Highlighter> {
  if (!shikiPromise) {
    shikiPromise = import('shiki').then((s) =>
      s.createHighlighter({
        themes: ['github-dark-default'],
        langs: STARTER_LANGS,
      }),
    );
  }
  return shikiPromise;
}

// Lazy-load a grammar that wasn't in the starter set.
async function ensureLang(h: Highlighter, lang: string): Promise<string> {
  const loaded = h.getLoadedLanguages();
  if (loaded.includes(lang)) return lang;
  try {
    await h.loadLanguage(lang as BundledLanguage);
    return lang;
  } catch {
    return 'text';
  }
}

// Map common aliases to Shiki's canonical language IDs.
function normaliseLang(raw: string): string {
  const l = (raw || '').toLowerCase().trim();
  const alias: Record<string, string> = {
    ts: 'typescript', js: 'javascript', py: 'python',
    yml: 'yaml', sh: 'bash', zsh: 'bash', console: 'bash',
    golang: 'go', rb: 'ruby', rs: 'rust', md: 'markdown',
    'c++': 'cpp', 'c#': 'csharp', cs: 'csharp',
  };
  return alias[l] || l || 'text';
}

export function MermaidDiagram({ code }: { code: string }) {
  const ref = useRef<HTMLDivElement>(null);
  const [svg, setSvg] = useState('');
  useEffect(() => {
    import('mermaid').then((m) => {
      m.default.initialize({ startOnLoad: false, theme: 'dark', themeVariables: { primaryColor: 'var(--primary)' } });
      const id = 'mermaid-' + Math.random().toString(36).slice(2);
      m.default.render(id, code).then(({ svg }) => setSvg(svg)).catch(() => setSvg(''));
    }).catch(() => setSvg(''));
  }, [code]);
  if (!svg) return <pre className="p-3 text-xs text-muted-foreground">{code}</pre>;
  return <div ref={ref} className="my-2 overflow-x-auto" dangerouslySetInnerHTML={{ __html: svg }} />;
}

// ShikiCode renders pre-highlighted HTML from Shiki. The innerHTML
// comes from Shiki's trusted renderer (never user-supplied HTML), so
// the usual XSS concern doesn't apply here. We isolate the
// dangerouslySetInnerHTML use in this one tiny component so the
// eslint-disable annotation is localised.
function ShikiCode({ html }: { html: string }) {
  return (
    // eslint-disable-next-line react/no-danger
    <div className="shiki-wrap" dangerouslySetInnerHTML={{ __html: html }} />
  );
}

export function CodeBlock({ language, code }: CodeBlockProps) {
  const [copied, setCopied] = useState(false);
  const [html, setHtml] = useState<string | null>(null);

  const handleCopy = () => {
    navigator.clipboard.writeText(code);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const rawLang = language?.toLowerCase() || '';
  const isBash = ['bash', 'sh', 'shell', 'zsh'].includes(rawLang);
  const shikiLang = normaliseLang(rawLang);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const h = await getShiki();
        const lang = await ensureLang(h, shikiLang);
        const out = h.codeToHtml(code, {
          lang,
          theme: 'github-dark-default',
        });
        if (!cancelled) setHtml(out);
      } catch {
        if (!cancelled) setHtml(null);
      }
    })();
    return () => { cancelled = true; };
  }, [code, shikiLang]);

  return (
    <div className={cn('rounded-lg border border-border/60 overflow-hidden my-3', isBash ? 'max-w-[70%]' : '')}>
      {/* Header */}
      <div className="flex items-center justify-between bg-card/80 px-3 py-1.5">
        <div className="flex items-center gap-2">
          {isBash && <span className="text-emerald-400 text-xs">$</span>}
          <span className="text-xs font-medium text-muted-foreground">{language || 'code'}</span>
        </div>
        <button onClick={handleCopy} className="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground/80 transition-colors cursor-pointer">
          {copied ? <><Check className="h-3 w-3 text-emerald-400" />Copied</> : <><Copy className="h-3 w-3" />Copy</>}
        </button>
      </div>
      {/* Code */}
      {html ? (
        <div className="shiki-host overflow-x-auto text-[13px] leading-snug font-mono [&_pre]:px-3 [&_pre]:py-2 [&_pre]:m-0">
          <ShikiCode html={html} />
        </div>
      ) : (
        <pre className="bg-background px-3 py-2 overflow-x-auto">
          <code className="text-[13px] leading-snug font-mono text-foreground/90">{code}</code>
        </pre>
      )}
    </div>
  );
}

// makeMarkdownComponents returns a ReactMarkdown component map tuned
// to the current message. Passing the message's sources in makes
// inline [N] citations click-to-scroll with a real tooltip; omitting
// sources falls back to the stripped-down static chip.
//
// Callers:
//   - MessageBubble / streaming renderer use the factory to pass the
//     message's own `sources` array
//   - legacy markdownComponents export keeps working for code paths
//     that don't know about sources yet
export function makeMarkdownComponents(
  sources?: { index: number | string; title: string; url: string }[],
) {
  return {
    ...markdownComponents,
    p({ children, node: _node, ...props }: any) {
      // Defer to the inline-citations helper when we have real
      // source metadata — it renders actual clickable chips. No
      // sources? keep the static fallback.
      if (sources?.length) {
        const processNode = (child: any, key: number): any => {
          if (typeof child === 'string') {
            // Lazy import to avoid a circular dependency at module
            // eval time — inline-citations imports from this file
            // is... wait, no. It is standalone. But requiring it
            // here keeps bundle size honest: chips are only built
            // when a source list exists.
            // eslint-disable-next-line @typescript-eslint/no-var-requires
            const { transformCitations } = require('@/components/chat/inline-citations');
            return <React.Fragment key={key}>{transformCitations(child, sources)}</React.Fragment>;
          }
          return child;
        };
        const kids = Array.isArray(children)
          ? children.map(processNode)
          : processNode(children, 0);
        return <p {...props}>{kids}</p>;
      }
      // Fallback: render raw [N] as inert pills using the original
      // behaviour below. Preserved for backwards compatibility.
      return markdownComponents.p({ children, node: _node, ...props });
    },
  };
}

// Custom markdown components for ReactMarkdown
export const markdownComponents = {
  code({ className, children, ...props }: any) {
    const match = /language-(\w+)/.exec(className || '');
    const code = String(children).replace(/\n$/, '');
    if (match) {
      if (match[1]! === 'mermaid') return <MermaidDiagram code={code} />;
      return <CodeBlock language={match[1]!} code={code} />;
    }
    return <code className="rounded bg-muted/60 border border-border/50 px-1.5 py-0.5 text-sm font-mono text-foreground" {...props}>{children}</code>;
  },
  table({ children }: any) {
    return <SortableMarkdownTable>{children}</SortableMarkdownTable>;
  },
  // th/td are rendered by SortableMarkdownTable directly; these fall
  // back to plain cells if a stray th/td escapes the parser.
  th({ children }: any) {
    return <th className="border border-border bg-muted/50 px-3 py-1.5 text-left font-medium">{children}</th>;
  },
  td({ children }: any) {
    return <td className="border border-border px-3 py-1.5">{children}</td>;
  },
  a({ href, children }: any) {
    return <a href={href} target="_blank" rel="noopener noreferrer" className="text-primary hover:underline">{children}</a>;
  },
  // Make [N] citation references into Perplexity-style badges
  p({ children, node, ...props }: any) {
    const processNode = (child: any, key: number): any => {
      if (typeof child === 'string') {
        const parts = child.split(/(\[\d+\])/g);
        if (parts.length <= 1) return child;
        return parts.map((part: string, i: number) => {
          const m = part.match(/^\[(\d+)\]$/);
          if (m) {
            return <span key={`${key}-${i}`} className="inline-flex items-center mx-0.5 px-1.5 py-0 rounded-full bg-muted/80 text-2xs font-mono text-muted-foreground hover:text-foreground hover:bg-muted cursor-pointer transition-colors">{m[1]}</span>;
          }
          return part;
        });
      }
      return child;
    };
    const kids = Array.isArray(children) ? children.map(processNode) : processNode(children, 0);
    return <p {...props}>{kids}</p>;
  },
};

// Re-export math plugins for ReactMarkdown
export { default as remarkMath } from 'remark-math';
export { default as rehypeKatex } from 'rehype-katex';

// ─── SortableMarkdownTable ────────────────────────────────────
//
// ReactMarkdown hands us `<table>` with a `<thead>` and `<tbody>` of
// already-rendered <tr>/<th>/<td> nodes. We flatten those into plain
// string arrays so we can sort rows client-side without losing the
// original cell ReactNodes (links, inline code, emphasis, etc.).
// Comparison prefers numeric when both sides parse as numbers so
// "10" sorts after "2" instead of lexicographically before it.

function extractText(node: any): string {
  if (node == null || node === false) return '';
  if (typeof node === 'string' || typeof node === 'number') return String(node);
  if (Array.isArray(node)) return node.map(extractText).join('');
  if (React.isValidElement(node)) {
    return extractText((node.props as any).children);
  }
  return '';
}

function flattenChildren(children: any): any[] {
  const out: any[] = [];
  React.Children.forEach(children, (c) => {
    if (c == null || c === false) return;
    out.push(c);
  });
  return out;
}

// Pull the array of <tr> React elements out of a <thead>/<tbody>
// node. ReactMarkdown wraps rows in <tr> children of those sections.
function rowsOf(section: React.ReactElement | undefined): React.ReactElement[] {
  if (!section) return [];
  const kids = flattenChildren((section.props as any).children);
  return kids.filter((k) => React.isValidElement(k) && (k.type === 'tr' || (typeof k.type !== 'string' && (k.props as any)?.node?.tagName === 'tr'))) as React.ReactElement[];
}

function cellsOf(row: React.ReactElement): React.ReactElement[] {
  const kids = flattenChildren((row.props as any).children);
  return kids.filter((k) => React.isValidElement(k)) as React.ReactElement[];
}

export function SortableMarkdownTable({ children }: { children: any }) {
  const [sortCol, setSortCol] = useState<number | null>(null);
  const [sortDir, setSortDir] = useState<'asc' | 'desc'>('asc');

  // Walk the ReactMarkdown output to find thead / tbody.
  const sections = flattenChildren(children);
  const thead = sections.find((s) => React.isValidElement(s) && s.type === 'thead') as React.ReactElement | undefined;
  const tbody = sections.find((s) => React.isValidElement(s) && s.type === 'tbody') as React.ReactElement | undefined;

  const headerRow = rowsOf(thead)[0];
  const headerCells = headerRow ? cellsOf(headerRow) : [];
  const bodyRows = rowsOf(tbody);

  // useMemo must be called unconditionally — moved before the early return
  // to avoid the React hooks rule violation ("Rendered more hooks than during
  // the previous render") that fires when !headerRow || bodyRows.length === 0.
  const sortedRows = useMemo(() => {
    if (sortCol == null) return bodyRows;
    const copy = bodyRows.slice();
    copy.sort((a, b) => {
      const av = extractText((cellsOf(a)[sortCol]?.props as any)?.children).trim();
      const bv = extractText((cellsOf(b)[sortCol]?.props as any)?.children).trim();
      const an = Number(av.replace(/[, ]/g, ''));
      const bn = Number(bv.replace(/[, ]/g, ''));
      let cmp: number;
      if (av !== '' && bv !== '' && !Number.isNaN(an) && !Number.isNaN(bn)) {
        cmp = an - bn;
      } else {
        cmp = av.localeCompare(bv);
      }
      return sortDir === 'asc' ? cmp : -cmp;
    });
    return copy;
  }, [bodyRows, sortCol, sortDir]);

  // If the shape doesn't match (e.g. no thead), render the raw table
  // without sorting so we never break the output.
  if (!headerRow || bodyRows.length === 0) {
    return (
      <div className="my-3 overflow-x-auto rounded-xl border border-border/60 bg-card/30">
        <table className="w-full text-sm">{children}</table>
      </div>
    );
  }

  const toggleSort = (idx: number) => {
    if (sortCol === idx) {
      setSortDir(sortDir === 'asc' ? 'desc' : 'asc');
    } else {
      setSortCol(idx);
      setSortDir('asc');
    }
  };

  return (
    <div className="my-3 overflow-x-auto rounded-xl border border-border/60 bg-card/30">
      <table className="w-full text-sm">
        <thead className="bg-muted/30">
          <tr>
            {headerCells.map((cell, i) => (
              <th
                key={i}
                onClick={() => toggleSort(i)}
                className="cursor-pointer select-none px-3 py-2 text-left font-medium text-muted-foreground hover:text-foreground border-b border-border/50"
              >
                <span className="inline-flex items-center gap-1">
                  {(cell.props as any).children}
                  {sortCol === i ? (
                    sortDir === 'asc' ? (
                      <ChevronUp className="h-3 w-3" />
                    ) : (
                      <ChevronDown className="h-3 w-3" />
                    )
                  ) : (
                    <ChevronsUpDown className="h-3 w-3 opacity-30" />
                  )}
                </span>
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {sortedRows.map((row, i) => (
            <tr key={i} className="border-b border-border/20 hover:bg-accent/20 transition-colors">
              {cellsOf(row).map((cell, j) => (
                <td key={j} className="px-3 py-1.5 align-top">
                  {(cell.props as any).children}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
