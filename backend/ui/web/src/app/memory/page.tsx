"use client";

import { useEffect, useState } from "react";
import { AppShell } from "@/components/layout/app-shell";

const API = process.env.NEXT_PUBLIC_API_URL || "http://localhost:4200";
const TOKEN = process.env.NEXT_PUBLIC_API_TOKEN || "";

interface Memory { id: string; content: string; type: string; source: string; importance: number; created_at: string; }
interface Session { id: string; agent_id: string; channel: string; label: string; created_at: string; updated_at: string; }

async function api(path: string) {
  const res = await fetch(`${API}/v1${path}`, { headers: { Authorization: `Bearer ${TOKEN}` } });
  return res.json();
}

// ── Memory Page ──
export default function MemoryPage() {
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<Memory[]>([]);
  const [searching, setSearching] = useState(false);

  async function search() {
    if (!query.trim()) return;
    setSearching(true);
    const data = await api(`/memory/search?q=${encodeURIComponent(query)}&limit=20`);
    setResults(data.results || []);
    setSearching(false);
  }

  return (
    <AppShell>
      <div className="p-6 max-w-4xl mx-auto">
        <h1 className="text-2xl font-bold mb-6">Memory</h1>

        <div className="flex gap-2 mb-6">
          <input value={query} onChange={e => setQuery(e.target.value)} onKeyDown={e => e.key === "Enter" && search()}
            placeholder="Search memories..." className="flex-1 px-4 py-2 bg-zinc-800 border border-zinc-700 rounded-lg" />
          <button onClick={search} disabled={searching}
            className="px-4 py-2 bg-purple-600 text-white rounded-lg hover:bg-purple-700 disabled:opacity-50">
            {searching ? "..." : "Search"}
          </button>
        </div>

        <div className="space-y-3">
          {results.map(m => (
            <div key={m.id} className="p-4 border border-zinc-700 rounded-lg bg-zinc-900">
              <div className="flex justify-between items-start">
                <span className="text-xs px-2 py-0.5 bg-zinc-800 rounded">{m.type}</span>
                <span className="text-xs text-zinc-500">{new Date(m.created_at).toLocaleDateString()}</span>
              </div>
              <p className="mt-2 text-sm">{m.content}</p>
              {m.source && <p className="text-xs text-zinc-500 mt-1">Source: {m.source}</p>}
            </div>
          ))}
          {results.length === 0 && query && !searching && <p className="text-zinc-500 text-center py-4">No results found.</p>}
          {!query && <p className="text-zinc-500 text-center py-8">Search agent memories by keyword or topic.</p>}
        </div>
      </div>
    </AppShell>
  );
}
