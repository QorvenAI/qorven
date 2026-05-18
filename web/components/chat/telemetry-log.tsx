'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

/**
 * TelemetryLog — in-chat rendering of orchestrator events.
 *
 * Subscribes to the store's per-session telemetry slice and renders
 * one line per event while the agent is mid-execution. Lives inline
 * in the chat feed — NOT a floating panel, NOT a modal, NOT a
 * separate tab. The goal is that a user waiting for an answer sees
 * what the agent is doing right now ("running graph.node_started:
 * write_file") instead of a silent dot-dot-dot.
 *
 * Designed around the Phase 9 Right-Sidebar-chat topology but
 * deliberately layout-agnostic: it renders a flat flexbox column
 * with semantic color hints and will look correct inside any
 * chat feed that already sets `text-xs` conventions.
 */

import { useStore } from '@/store';
import { cn } from '@/lib/utils';
import {
  EVT_AGENT_PROGRESS,
  EVT_GRAPH_NODE_COMPLETED,
  EVT_GRAPH_NODE_FAILED,
  EVT_GRAPH_NODE_PAUSED,
  EVT_GRAPH_NODE_STARTED,
  type TelemetryEvent,
} from '@/lib/graph-events';
import { CheckCircle2, Loader2, AlertCircle, PauseCircle, Wrench } from 'lucide-react';

interface Props {
  /** Session id whose telemetry to show. When empty the component
   *  renders nothing — a chat that hasn't created a session yet
   *  has nothing to stream. */
  sessionId: string;
  /**
   * When false, the log hides itself. Caller typically passes
   * `isLoading` so the log appears while a turn is in flight and
   * collapses after completion. Telemetry events themselves stay
   * in the store — flipping `active` back on shows the same history.
   */
  active: boolean;
  /** Optional cap on the number of lines rendered. Default 30 —
   *  the full session buffer lives in the store (see
   *  TELEMETRY_CAP_PER_SESSION); this is just the visible tail. */
  maxLines?: number;
}

export function TelemetryLog({ sessionId, active, maxLines = 30 }: Props) {
  // Select only this session's slice so unrelated stores don't
  // trigger re-renders of this component.
  const events = useStore((s) => s.telemetryBySession[sessionId] ?? []);

  if (!active) return null;
  if (!sessionId || events.length === 0) {
    // Don't leave users with a blank chat while the agent is
    // warming up — the generic spinner still has a purpose BEFORE
    // the first telemetry event arrives.
    return (
      <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
        <Loader2 className="h-3 w-3 animate-spin" />
        <span>Thinking…</span>
      </div>
    );
  }

  const visible = events.slice(-maxLines);

  return (
    <div className="rounded-lg border border-border bg-muted/30 p-2 text-2xs font-mono space-y-1">
      <div className="flex items-center gap-1.5 text-muted-foreground pb-1 border-b border-border/50">
        <Loader2 className="h-3 w-3 animate-spin" />
        <span className="uppercase tracking-wider">agent stream</span>
      </div>
      {visible.map((ev, i) => (
        <TelemetryLine key={`${ev.seq ?? ''}-${ev.timestamp}-${i}`} event={ev} />
      ))}
    </div>
  );
}

function TelemetryLine({ event }: { event: TelemetryEvent }) {
  const { icon, color, title, detail } = describe(event);
  return (
    <div className="flex items-start gap-1.5 py-0.5">
      <span className={cn('shrink-0 mt-[1px]', color)}>{icon}</span>
      <span className="flex-1 min-w-0">
        <span className={cn('font-medium', color)}>{title}</span>
        {detail && (
          <span className="text-muted-foreground ml-1 break-all">{detail}</span>
        )}
      </span>
    </div>
  );
}

/**
 * describe — map the discriminated TelemetryEvent onto icon + color +
 * human-readable title/detail. Kept in one pure function so adding
 * a new event kind is a single-location change.
 *
 * The function is exported-adjacent (not exported) because consumers
 * of TelemetryLog should render through the component, not re-roll
 * the formatting.
 */
function describe(event: TelemetryEvent): {
  icon: React.ReactNode;
  color: string;
  title: string;
  detail?: string;
} {
  switch (event.type) {
    case EVT_GRAPH_NODE_STARTED: {
      const d = event.data;
      return {
        icon: <Loader2 className="h-3 w-3 animate-spin" />,
        color: 'text-primary',
        title: `▶ ${d.kind ?? 'node'}`,
        detail: d.title || d.node_id.slice(0, 8),
      };
    }
    case EVT_GRAPH_NODE_COMPLETED: {
      const d = event.data;
      return {
        icon: <CheckCircle2 className="h-3 w-3" />,
        color: 'text-emerald-400',
        title: `✓ ${d.kind ?? 'node'}`,
        detail: d.outcome || d.title,
      };
    }
    case EVT_GRAPH_NODE_PAUSED: {
      const d = event.data;
      return {
        icon: <PauseCircle className="h-3 w-3" />,
        color: 'text-amber-400',
        title: '⏸ paused',
        detail: d.reason || (d.approval_id ? `awaiting approval ${d.approval_id.slice(0, 8)}` : d.title),
      };
    }
    case EVT_GRAPH_NODE_FAILED: {
      const d = event.data;
      return {
        icon: <AlertCircle className="h-3 w-3" />,
        color: 'text-destructive',
        title: `✗ ${d.kind ?? 'node'} failed`,
        detail: d.error,
      };
    }
    case EVT_AGENT_PROGRESS: {
      const d = event.data;
      // Surface tool_start/tool_end as their own shape since that's
      // what the user most cares about during a turn. Other kinds
      // pass through with a generic icon.
      if (d.kind === 'tool_start' || d.kind === 'tool_end') {
        const toolName =
          (d.detail && typeof d.detail['tool'] === 'string' && (d.detail['tool'] as string)) ||
          '';
        return {
          icon: <Wrench className="h-3 w-3" />,
          color: d.kind === 'tool_start' ? 'text-cyan-400' : 'text-muted-foreground',
          title: d.kind === 'tool_start' ? '→ tool' : '← tool',
          detail: toolName || d.kind,
        };
      }
      return {
        icon: <Loader2 className="h-3 w-3" />,
        color: 'text-muted-foreground',
        title: d.kind || 'progress',
        detail: d.agent_key,
      };
    }
  }
}
