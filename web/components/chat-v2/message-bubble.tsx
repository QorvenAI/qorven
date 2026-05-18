'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import React, { useState, useEffect, memo } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { type UIMessage } from 'ai';
import { Copy, Check, RefreshCw, ThumbsUp, ThumbsDown } from 'lucide-react';
import { cn } from '@/lib/utils';
import { modelDisplayName } from '@/lib/model-names';
import { ToolInvocation } from './tool-invocation';
import { CitationBar, transformCitations, type Source } from './citation-bar';
import { markdownComponents, makeMarkdownComponents, CodeBlock } from '@/components/chat/code-block';
import { BrandIcon, getBrandTitle } from '@/components/brand-icon';

function relativeTime(ts?: string | number): string {
  if (!ts) return '';
  const d = typeof ts === 'number' ? new Date(ts) : new Date(ts);
  const diff = (Date.now() - d.getTime()) / 1000;
  if (diff < 5) return 'now';
  if (diff < 60) return `${Math.floor(diff)}s ago`;
  if (diff < 3600) return `${Math.floor(diff / 60)}m ago`;
  if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`;
  return d.toLocaleDateString();
}

function Timestamp({ ts }: { ts?: string | number }) {
  const [, tick] = useState(0);
  useEffect(() => {
    const id = setInterval(() => tick((n) => n + 1), 30_000);
    return () => clearInterval(id);
  }, []);
  return <span className="text-2xs text-muted-foreground/60">{relativeTime(ts)}</span>;
}

interface MessageBubbleProps {
  message: UIMessage & { timestamp?: string; model?: string };
  isStreaming?: boolean;
  onRegenerate?: () => void;
  agentName?: string;
  agentId?: string;
}

const HIDDEN_CHANNELS = new Set(['web', 'webchat', '']);

export const MessageBubble = memo(function MessageBubble({ message, isStreaming, onRegenerate, agentName, agentId }: MessageBubbleProps) {
  const [copied, setCopied] = useState(false);
  const isUser = message.role === 'user';
  const meta = message.metadata as Record<string, unknown> | undefined;
  const sourceChannel = ((meta?.source_channel ?? '') as string);
  const showChannel = sourceChannel && !HIDDEN_CHANNELS.has(sourceChannel);
  // Timestamp: prefer top-level (set by AI SDK for live messages), fall back to metadata (set for history)
  const ts = message.timestamp ?? (meta?.timestamp as string | number | undefined);

  // Extract sources from message annotations (written by the route bridge as data-sources chunks)
  const sources: Source[] = [];
  if (message.metadata) {
    const meta = message.metadata as Record<string, unknown>;
    if (Array.isArray(meta.sources)) {
      sources.push(...(meta.sources as Source[]));
    }
  }

  // Also check annotations array
  const annotations = (message as unknown as { annotations?: unknown[] }).annotations ?? [];
  for (const ann of annotations) {
    if (ann && typeof ann === 'object' && (ann as Record<string, unknown>).type === 'data-sources') {
      const d = (ann as Record<string, unknown>).data;
      if (Array.isArray(d)) sources.push(...(d as Source[]));
    }
  }

  // Extract widgets from annotations (stored by route bridge as data-widget chunks)
  const widgets: Array<{ type: string; data: Record<string, unknown> }> = [];
  for (const ann of annotations) {
    if (ann && typeof ann === 'object' && (ann as Record<string, unknown>).type === 'data-widget') {
      const d = (ann as Record<string, unknown>).data as Record<string, unknown> | undefined;
      const wType = d?.type as string | undefined;
      if (wType) widgets.push({ type: wType, data: (d?.data as Record<string, unknown>) ?? {} });
    }
  }

  const mdComponents = sources.length
    ? makeMarkdownComponents(sources.map((s, i) => ({ index: Number(s.index ?? i + 1), title: s.title ?? '', url: s.url })))
    : markdownComponents;

  const handleCopy = () => {
    const text = message.parts
      ?.filter((p) => p.type === 'text')
      .map((p) => (p as { type: 'text'; text: string }).text)
      .join('\n') ?? '';
    navigator.clipboard.writeText(text);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  if (isUser) {
    // User messages: render parts (text + file attachments)
    const textParts = message.parts?.filter((p) => p.type === 'text') ?? [];
    const fileParts = message.parts?.filter((p) => p.type === 'file') ?? [];

    return (
      <div className="flex justify-end gap-2 group">
        <div className="max-w-[75%] space-y-2">
          {fileParts.map((p, i) => {
            const fp = p as { type: 'file'; mediaType?: string; filename?: string; url?: string };
            return (
              <div key={i} className="rounded-xl border border-border bg-card px-3 py-2 text-sm text-muted-foreground">
                📎 {fp.filename ?? 'attachment'}
              </div>
            );
          })}
          {textParts.map((p, i) => {
            const tp = p as { type: 'text'; text: string };
            return (
              <div key={i} className="rounded-2xl bg-primary px-4 py-2.5 text-sm text-primary-foreground leading-relaxed">
                {tp.text}
              </div>
            );
          })}
          <div className="flex justify-end items-center gap-1.5">
            {showChannel && (
              <span className="inline-flex items-center gap-1 text-2xs text-muted-foreground/60">
                <BrandIcon name={sourceChannel} size={11} />
                <span>{getBrandTitle(sourceChannel)}</span>
              </span>
            )}
            <Timestamp ts={ts} />
          </div>
        </div>
      </div>
    );
  }

  // Assistant message — render parts in order
  return (
    <div className="group flex gap-3">
      {/* Avatar */}
      <div className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-primary/15 text-primary text-xs font-bold mt-0.5">
        {(agentName ?? 'Q').charAt(0).toUpperCase()}
      </div>

      <div className="flex-1 min-w-0 space-y-1">
        {/* Parts */}
        {message.parts?.map((part, i) => {
          if (part.type === 'text') {
            const tp = part as { type: 'text'; text: string };
            return (
              <div key={i} className="prose prose-sm max-w-none text-foreground leading-relaxed font-normal [&_p]:text-foreground [&_p]:font-normal [&_li]:text-foreground [&_li]:font-normal [&_strong]:text-foreground [&_strong]:font-semibold [&_p]:text-sm [&_li]:text-sm [&_h1]:text-foreground [&_h2]:text-foreground [&_h3]:text-foreground [&_h4]:text-foreground [&_h5]:text-foreground [&_h6]:text-foreground [&_h1]:font-semibold [&_h2]:font-semibold [&_h3]:font-semibold [&_h4]:font-medium [&_h1]:text-base [&_h2]:text-sm [&_h3]:text-sm [&_h4]:text-sm [&_blockquote]:text-muted-foreground [&_a]:text-primary [&_code]:text-foreground">
                <ReactMarkdown
                  remarkPlugins={[remarkGfm]}
                  components={mdComponents as Record<string, unknown> as Parameters<typeof ReactMarkdown>[0]['components']}
                >
                  {tp.text}
                </ReactMarkdown>
              </div>
            );
          }

          if (part.type === 'reasoning') {
            const rp = part as { type: 'reasoning'; text: string };
            return (
              <ThinkingBlock key={i} content={rp.text} />
            );
          }

          if (part.type === 'dynamic-tool') {
            const tp = part as {
              type: 'dynamic-tool';
              toolCallId: string;
              toolName: string;
              state: 'input-streaming' | 'input-available' | 'input' | 'output-available' | 'output' | 'output-error' | 'output-denied';
              input?: unknown;
              output?: unknown;
            };
            const invState = (tp.state === 'output' || tp.state === 'output-available' || tp.state === 'output-error') ? 'result' : 'calling';
            return (
              <ToolInvocation
                key={i}
                toolCallId={tp.toolCallId}
                toolName={tp.toolName}
                input={tp.input}
                output={tp.output}
                state={invState}
                agentId={agentId}
              />
            );
          }

          if (part.type === 'source-url' || part.type === 'source-document') {
            // Individual source parts — collected into CitationBar below
            return null;
          }

          if (part.type === 'file') {
            const fp = part as { type: 'file'; mediaType?: string; url?: string };
            if (fp.mediaType?.startsWith('image/')) {
              return (
                <img key={i} src={fp.url} alt="attachment" className="max-w-sm rounded-xl border border-border" />
              );
            }
            return null;
          }

          // Persisted widget parts from session history (WidgetPart from agent/parts.go)
          if ((part as { type: string }).type === 'widget') {
            const wp = part as unknown as { type: 'widget'; widgetType?: string; widgetData?: Record<string, unknown> };
            if (wp.widgetType === 'audio') {
              const src = wp.widgetData?.src as string | undefined;
              if (!src) return null;
              return (
                <div key={i} className="rounded-xl border border-border bg-card/50 p-3 max-w-sm">
                  <p className="text-xs text-muted-foreground mb-2">🔊 Audio</p>
                  <audio controls src={src} className="w-full h-8" />
                </div>
              );
            }
            if (wp.widgetType === 'image') {
              const src = wp.widgetData?.url as string | undefined;
              if (!src) return null;
              return <img key={i} src={src} alt={(wp.widgetData?.prompt as string) ?? 'generated image'} className="max-w-sm rounded-xl border border-border" />;
            }
            return null;
          }

          return null;
        })}

        {/* Widget cards — audio, image, etc. */}
        {widgets.length > 0 && (
          <div className="space-y-2">
            {widgets.map((w, i) => {
              if (w.type === 'audio') {
                const src = w.data.src as string | undefined;
                if (!src) return null;
                return (
                  <div key={i} className="rounded-xl border border-border bg-card/50 p-3 max-w-sm">
                    <p className="text-xs text-muted-foreground mb-2">🔊 Audio</p>
                    <audio controls src={src} className="w-full h-8" />
                  </div>
                );
              }
              if (w.type === 'image') {
                const src = w.data.url as string | undefined;
                if (!src) return null;
                return <img key={i} src={src} alt={(w.data.prompt as string) ?? 'generated image'} className="max-w-sm rounded-xl border border-border" />;
              }
              return null;
            })}
          </div>
        )}

        {/* Citation bar — unified source display */}
        {sources.length > 0 && <CitationBar sources={sources} />}

        {/* Actions row */}
        {!isStreaming && (
          <div className="flex items-center gap-1 pt-1">
            <Timestamp ts={ts} />
            {showChannel && (
              <span className="inline-flex items-center gap-1 text-2xs text-muted-foreground/60 ml-1">
                <BrandIcon name={sourceChannel} size={11} />
                <span>{getBrandTitle(sourceChannel)}</span>
              </span>
            )}
            {message.model && (
              <span className="text-2xs font-mono text-muted-foreground/50 ml-1">{modelDisplayName(message.model)}</span>
            )}
            <div className="flex-1" />
            <div className="opacity-0 group-hover:opacity-100 transition-opacity flex items-center gap-1">
              <ActionButton onClick={handleCopy} title={copied ? 'Copied' : 'Copy'}>
                {copied ? <Check className="h-3 w-3 text-emerald-500" /> : <Copy className="h-3 w-3" />}
              </ActionButton>
              {onRegenerate && (
                <ActionButton onClick={onRegenerate} title="Regenerate">
                  <RefreshCw className="h-3 w-3" />
                </ActionButton>
              )}
              <ActionButton title="Good response">
                <ThumbsUp className="h-3 w-3" />
              </ActionButton>
              <ActionButton title="Bad response">
                <ThumbsDown className="h-3 w-3" />
              </ActionButton>
            </div>
          </div>
        )}
      </div>
    </div>
  );
});

function ActionButton({ onClick, title, children }: { onClick?: () => void; title?: string; children: React.ReactNode }) {
  return (
    <button
      onClick={onClick}
      title={title}
      className="flex h-6 w-6 items-center justify-center rounded-md text-muted-foreground hover:bg-accent hover:text-foreground transition-colors"
    >
      {children}
    </button>
  );
}

function ThinkingBlock({ content }: { content: string }) {
  const [expanded, setExpanded] = useState(false);
  return (
    <div className="my-1 rounded-xl border border-border/40 bg-card/30 overflow-hidden">
      <button
        type="button"
        onClick={() => setExpanded(!expanded)}
        className="flex w-full items-center gap-2 px-3 py-2 text-xs text-muted-foreground hover:bg-accent/30 transition-colors cursor-pointer"
      >
        <span className="animate-pulse text-amber-400">◆</span>
        <span>Thinking</span>
        <span className="ml-auto">{expanded ? '▲' : '▼'}</span>
      </button>
      {expanded && (
        <div className="px-3 pb-3 text-xs text-muted-foreground/70 font-mono whitespace-pre-wrap border-t border-border/30 pt-2">
          {content}
        </div>
      )}
    </div>
  );
}
