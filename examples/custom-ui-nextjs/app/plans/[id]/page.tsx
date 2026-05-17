"use client";

// Force dynamic rendering — see note in app/login/page.tsx.
export const dynamic = "force-dynamic";

import { useEffect, useState } from "react";
import { useParams, useRouter } from "next/navigation";

import { apiBase } from "@/lib/api";
import { bootstrap, getAuth, subscribeAuth, type User } from "@/lib/auth";
import { getPlan, listPlanNodes, type Plan, type PlanNode } from "@/lib/api";
import { useQorvenSocket } from "@/hooks/useQorvenSocket";
import { useGraphState } from "@/hooks/useGraphState";
import { GraphVisualizer } from "@/components/GraphVisualizer";

/**
 * Plan detail page — the headline demo.
 *
 * Lifecycle:
 *   1. Bootstrap auth; redirect to /login on failure.
 *   2. Fetch /v1/plans/{id} and /v1/plans/{id}/nodes once (seed data).
 *   3. Open the WebSocket via useQorvenSocket.
 *   4. Feed seed + events into useGraphState; render.
 *
 * Important: all the "live" logic is in the hooks. The page just
 * wires them together. Swapping the visualizer for a different
 * layout means touching only <GraphVisualizer /> below.
 */
export default function PlanPage() {
  const params = useParams<{ id: string }>();
  const router = useRouter();
  const planID = params.id;

  const [user, setUser] = useState<User | null>(null);
  const [plan, setPlan] = useState<Plan | null>(null);
  const [nodes, setNodes] = useState<PlanNode[]>([]);
  const [loadErr, setLoadErr] = useState<string | null>(null);

  // Subscribe early so we don't miss events that fire between REST
  // and WebSocket establishment. The hook buffers up to 500 events.
  const { status, events, lastError } = useQorvenSocket();
  const graph = useGraphState(nodes, events, planID);

  // Auth bootstrap + initial fetch.
  useEffect(() => {
    let cancelled = false;
    bootstrap(apiBase())
      .then(async (u) => {
        if (cancelled) return;
        if (!u) {
          router.replace("/login");
          return;
        }
        setUser(u);
        try {
          const [p, ns] = await Promise.all([
            getPlan(planID),
            listPlanNodes(planID),
          ]);
          if (cancelled) return;
          setPlan(p);
          setNodes(ns);
        } catch (e) {
          if (!cancelled) {
            setLoadErr(e instanceof Error ? e.message : String(e));
          }
        }
      });
    const unsub = subscribeAuth(() => {
      if (!getAuth().user) router.replace("/login");
    });
    return () => {
      cancelled = true;
      unsub();
    };
  }, [planID, router]);

  if (!user) return <main style={wrap}><p>Authenticating…</p></main>;
  if (loadErr) return (
    <main style={wrap}>
      <h1>Plan {planID.slice(0, 8)}…</h1>
      <p style={errStyle}>{loadErr}</p>
      <button onClick={() => router.push("/")}>Home</button>
    </main>
  );
  if (!plan) return <main style={wrap}><p>Loading plan…</p></main>;

  return (
    <main style={wrap}>
      <header style={header}>
        <div>
          <h1 style={{ margin: 0 }}>{plan.title || "(untitled plan)"}</h1>
          <div style={{ color: "#64748b", fontSize: "0.85rem", marginTop: 4 }}>
            <span>status: <strong>{plan.status}</strong></span>
            <span style={{ marginLeft: 12 }}>plan {plan.id.slice(0, 8)}…</span>
            <span style={{ marginLeft: 12 }}>
              ws: <strong style={{ color: wsColor(status) }}>{status}</strong>
              {lastError && <span style={{ marginLeft: 4, color: "#991b1b" }}>({lastError})</span>}
            </span>
          </div>
        </div>
        <button onClick={() => router.push("/")}>Home</button>
      </header>

      <GraphVisualizer nodes={graph.nodes} banner={graph.banner} />

      <details style={{ marginTop: "2rem", fontSize: "0.85rem" }}>
        <summary style={{ cursor: "pointer", color: "#475569" }}>
          Debug ({events.length} events received)
        </summary>
        <pre style={debug}>
          {events
            .slice(-10)
            .map((e) => JSON.stringify({ type: e.type, data: e.data }))
            .join("\n")}
        </pre>
      </details>
    </main>
  );
}

function wsColor(s: string): string {
  switch (s) {
    case "open": return "#166534";
    case "connecting": return "#92400e";
    case "error":
    case "closed": return "#991b1b";
    default: return "#64748b";
  }
}

const wrap: React.CSSProperties = {
  maxWidth: 860,
  margin: "0 auto",
  padding: "2rem 1.5rem",
};
const header: React.CSSProperties = {
  display: "flex",
  justifyContent: "space-between",
  alignItems: "flex-start",
  marginBottom: "1.5rem",
  paddingBottom: "1rem",
  borderBottom: "1px solid #e2e8f0",
};
const errStyle: React.CSSProperties = {
  color: "#991b1b",
  background: "#fef2f2",
  padding: "1rem",
  borderRadius: 4,
};
const debug: React.CSSProperties = {
  background: "#0f172a",
  color: "#e2e8f0",
  padding: "1rem",
  borderRadius: 4,
  overflow: "auto",
  fontSize: "0.75rem",
};
