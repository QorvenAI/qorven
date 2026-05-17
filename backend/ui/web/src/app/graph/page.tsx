"use client";

import { useEffect, useRef, useState } from "react";
import dynamic from "next/dynamic";
import { AppShell } from "@/components/layout/app-shell";

const API = process.env.NEXT_PUBLIC_API_URL || "http://localhost:4200";
const TOKEN = process.env.NEXT_PUBLIC_API_TOKEN || "";

interface GraphNode { id: string; label: string; type: string; color: string; size: number; link_count: number; }
interface GraphEdge { id: string; source: string; target: string; label: string; weight: number; color: string; }
interface GraphData { nodes: GraphNode[]; edges: GraphEdge[]; stats: { total_nodes: number; total_edges: number; nodes_by_type: Record<string, number>; avg_degree: number; }; }

export default function GraphPage() {
  const containerRef = useRef<HTMLDivElement>(null);
  const [data, setData] = useState<GraphData | null>(null);
  const [error, setError] = useState("");
  const [hovered, setHovered] = useState<GraphNode | null>(null);

  useEffect(() => {
    fetch(`${API}/v1/graph`, { headers: { Authorization: `Bearer ${TOKEN}` } })
      .then(r => r.json())
      .then(d => { if (d.nodes) setData(d); else setError(d.error || "No graph data"); })
      .catch(e => setError(e.message));
  }, []);

  useEffect(() => {
    if (!data || !containerRef.current || data.nodes.length === 0) return;

    // Dynamic import for browser-only libraries
    Promise.all([
      import("graphology"),
      import("sigma"),
    ]).then(([graphologyMod, sigmaMod]) => {
      const Graph = graphologyMod.default;
      const Sigma = sigmaMod.default;

      const graph = new Graph();

    // Add nodes with random initial positions
    data.nodes.forEach((n, i) => {
      const angle = (2 * Math.PI * i) / data.nodes.length;
      graph.addNode(n.id, {
        label: n.label,
        x: Math.cos(angle) * 100 + Math.random() * 20,
        y: Math.sin(angle) * 100 + Math.random() * 20,
        size: n.size,
        color: n.color,
        type: n.type,
      });
    });

    // Add edges
    data.edges.forEach(e => {
      if (graph.hasNode(e.source) && graph.hasNode(e.target)) {
        try { graph.addEdge(e.source, e.target, { label: e.label, color: e.color, size: e.weight }); } catch {}
      }
    });

    // Render with Sigma
    const sigma = new Sigma(graph, containerRef.current, {
      renderLabels: true,
      labelSize: 12,
      labelColor: { color: "#e4e4e7" },
      defaultEdgeColor: "#3f3f46",
      defaultNodeColor: "#8b5cf6",
    });

    sigma.on("enterNode", ({ node }) => {
      const attrs = graph.getNodeAttributes(node);
      setHovered({ id: node, label: attrs.label, type: attrs.type, color: attrs.color, size: attrs.size, link_count: 0 });
    });
    sigma.on("leaveNode", () => setHovered(null));

    return () => sigma.kill();
    }); // end Promise.all
  }, [data]);

  return (
    <AppShell>
      <div className="flex flex-col h-full">
        <div className="p-4 border-b border-zinc-800 flex justify-between items-center">
          <h1 className="text-xl font-bold">Knowledge Graph</h1>
          {data && (
            <div className="flex gap-4 text-sm text-zinc-400">
              <span>{data.stats.total_nodes} nodes</span>
              <span>{data.stats.total_edges} edges</span>
              <span>avg degree: {data.stats.avg_degree.toFixed(1)}</span>
            </div>
          )}
        </div>

        {error && <div className="p-8 text-center text-zinc-500">{error}</div>}

        {data && data.nodes.length === 0 && (
          <div className="p-8 text-center text-zinc-500">
            <p className="text-lg">No entities in the knowledge graph yet.</p>
            <p className="mt-2">Chat with an agent to start building knowledge.</p>
          </div>
        )}

        <div className="flex-1 relative">
          <div ref={containerRef} className="absolute inset-0" />

          {/* Legend */}
          {data && data.stats.nodes_by_type && (
            <div className="absolute top-4 left-4 bg-zinc-900/90 border border-zinc-700 rounded-lg p-3 text-sm">
              <p className="font-semibold mb-2">Entity Types</p>
              {Object.entries(data.stats.nodes_by_type).map(([type, count]) => (
                <div key={type} className="flex items-center gap-2">
                  <div className="w-3 h-3 rounded-full" style={{ backgroundColor: typeColor(type) }} />
                  <span>{type}: {count}</span>
                </div>
              ))}
            </div>
          )}

          {/* Hover tooltip */}
          {hovered && (
            <div className="absolute top-4 right-4 bg-zinc-900/90 border border-zinc-700 rounded-lg p-3 text-sm">
              <p className="font-semibold">{hovered.label}</p>
              <p className="text-zinc-400">Type: {hovered.type}</p>
            </div>
          )}
        </div>
      </div>
    </AppShell>
  );
}

function typeColor(type: string): string {
  const colors: Record<string, string> = {
    person: "#4A90D9", organization: "#E67E22", product: "#2ECC71",
    concept: "#9B59B6", technology: "#1ABC9C", event: "#E74C3C",
    location: "#F39C12", source: "#95A5A6",
  };
  return colors[type] || "#8b5cf6";
}
