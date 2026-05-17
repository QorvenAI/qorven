"use client";

import { useMemo } from "react";

import type { PlanNode } from "@/lib/api";
import {
  EVT_GRAPH_NODE_COMPLETED,
  EVT_GRAPH_NODE_FAILED,
  EVT_GRAPH_NODE_PAUSED,
  EVT_GRAPH_NODE_STARTED,
  type QorvenEvent,
} from "@/lib/events";

/**
 * Compiles a seed list of plan nodes (fetched via REST on mount) +
 * a stream of realtime events into a live view of the plan graph.
 *
 * ## Design choices
 *
 *   • Server-as-truth model. The initial node list comes from the
 *     REST `/v1/plans/{id}/nodes` response. Realtime events are
 *     applied on top — they can only mutate `state`, not create or
 *     delete nodes. If the server creates a NEW node mid-run, the
 *     UI won't see it until the next REST refresh. That's a known
 *     trade; a live graph builder is out of scope for this template.
 *
 *   • Pure derivation. This hook does not mutate external state;
 *     every render walks the event array. For plans with hundreds
 *     of nodes + thousands of events this becomes measurable —
 *     memoize the reducer output (already done via useMemo) and
 *     cap the upstream event buffer (useQorvenSocket's bufferSize).
 *
 *   • Only `graph.node_*` events affect state. `agent.progress`
 *     is handed through for display but doesn't flip node state.
 *
 * ## Return shape
 *
 *   nodes: Map<node_id, NodeView>
 *   lastEventFor: Map<node_id, QorvenEvent>   // most recent event per node
 *   banner: string | null                     // highest-severity human message (e.g. "Paused: awaiting approval")
 */

export type NodeView = PlanNode & {
  /** Reason from the last paused/failed event, if applicable. */
  message?: string;
  /** Outcome string from the last completed event, if applicable. */
  outcome?: string;
};

export function useGraphState(
  initial: PlanNode[],
  events: QorvenEvent[],
  planID: string,
) {
  return useMemo(() => {
    const nodes = new Map<string, NodeView>();
    for (const n of initial) nodes.set(n.id, { ...n });
    const lastEventFor = new Map<string, QorvenEvent>();
    let banner: string | null = null;

    for (const ev of events) {
      // Filter to events for this plan. The hub broadcasts global;
      // consumers filter client-side. (A production backend could
      // filter server-side by subscription; today's hub doesn't.)
      const data = ev.data as { plan_id?: string; node_id?: string } | undefined;
      if (!data || data.plan_id !== planID || !data.node_id) continue;

      const cur = nodes.get(data.node_id);
      if (!cur) continue; // unknown node — see "server-as-truth" note above

      switch (ev.type) {
        case EVT_GRAPH_NODE_STARTED: {
          nodes.set(cur.id, { ...cur, state: "running" });
          banner = null;
          break;
        }
        case EVT_GRAPH_NODE_COMPLETED: {
          const d = ev.data as { outcome?: string };
          nodes.set(cur.id, {
            ...cur,
            state: "done",
            outcome: d.outcome,
          });
          break;
        }
        case EVT_GRAPH_NODE_PAUSED: {
          const d = ev.data as { reason?: string };
          nodes.set(cur.id, {
            ...cur,
            state: "blocked",
            message: d.reason,
          });
          banner = `Paused: ${d.reason ?? "awaiting input"}`;
          break;
        }
        case EVT_GRAPH_NODE_FAILED: {
          const d = ev.data as { error?: string };
          nodes.set(cur.id, {
            ...cur,
            state: "failed",
            message: d.error,
          });
          banner = `Failed: ${d.error ?? "node failed"}`;
          break;
        }
        default:
          // agent.progress and unknown types — keep the event in
          // lastEventFor for display, but don't mutate state.
          break;
      }
      lastEventFor.set(cur.id, ev);
    }
    return { nodes, lastEventFor, banner };
  }, [initial, events, planID]);
}
