"use client";

import type { NodeView } from "@/hooks/useGraphState";
import styles from "./GraphVisualizer.module.css";

/**
 * GraphVisualizer renders a column of plan nodes colored by state.
 *
 * This intentionally does not use a graph-layout library — the plan
 * graph is linear today (planner → human_feedback → agent_task)
 * and a simple ordered list is the clearest visualization. Swap in
 * @xyflow/react, react-flow, or cytoscape if your graphs fan out.
 *
 * State → color mapping matches `backend/internal/plans`
 * NodeState constants. If the backend adds a new state, add a
 * corresponding CSS class here; the fallback is `.state-unknown`.
 */

type Props = {
  nodes: Map<string, NodeView>;
  banner: string | null;
};

const STATE_CLASS: Record<NodeView["state"] | "unknown", string> = {
  pending: styles.statePending,
  running: styles.stateRunning,
  blocked: styles.stateBlocked,
  done: styles.stateDone,
  failed: styles.stateFailed,
  cancelled: styles.stateCancelled,
  unknown: styles.stateUnknown,
};

export function GraphVisualizer({ nodes, banner }: Props) {
  const ordered = Array.from(nodes.values()).sort((a, b) =>
    a.created_at.localeCompare(b.created_at),
  );

  return (
    <div className={styles.wrap}>
      {banner && <div className={styles.banner}>{banner}</div>}
      <ol className={styles.list}>
        {ordered.map((n) => {
          const cls =
            STATE_CLASS[n.state as NodeView["state"]] ?? STATE_CLASS.unknown;
          return (
            <li key={n.id} className={`${styles.node} ${cls}`}>
              <div className={styles.rowTop}>
                <span className={styles.kind}>{n.kind}</span>
                <span className={styles.state}>{n.state}</span>
              </div>
              <div className={styles.title}>{n.title || "(untitled)"}</div>
              {n.message && <div className={styles.msg}>{n.message}</div>}
              {n.outcome && (
                <div className={styles.outcome}>outcome: {n.outcome}</div>
              )}
            </li>
          );
        })}
      </ol>
    </div>
  );
}
