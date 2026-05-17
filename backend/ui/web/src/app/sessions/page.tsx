"use client";

import { useEffect, useState } from "react";
import { AppShell } from "@/components/layout/app-shell";

const API = process.env.NEXT_PUBLIC_API_URL || "http://localhost:4200";
const TOKEN = process.env.NEXT_PUBLIC_API_TOKEN || "";

interface Session { id: string; agent_id: string; channel: string; label: string; summary: string; input_tokens: number; output_tokens: number; created_at: string; updated_at: string; }

export default function SessionsPage() {
  const [sessions, setSessions] = useState<Session[]>([]);

  useEffect(() => {
    fetch(`${API}/v1/sessions`, { headers: { Authorization: `Bearer ${TOKEN}` } })
      .then(r => r.json())
      .then(d => setSessions(d.sessions || []))
      .catch(() => {});
  }, []);

  async function deleteSession(id: string) {
    if (!confirm("Delete this session?")) return;
    await fetch(`${API}/v1/sessions/${id}`, { method: "DELETE", headers: { Authorization: `Bearer ${TOKEN}` } });
    setSessions(s => s.filter(x => x.id !== id));
  }

  return (
    <AppShell>
      <div className="p-6 max-w-4xl mx-auto">
        <h1 className="text-2xl font-bold mb-6">Sessions</h1>

        <div className="space-y-3">
          {sessions.map(s => (
            <div key={s.id} className="p-4 border border-zinc-700 rounded-lg bg-zinc-900 flex justify-between items-start">
              <div>
                <div className="flex items-center gap-2">
                  <span className="font-semibold">{s.label || s.id.slice(0, 8)}</span>
                  <span className="text-xs px-2 py-0.5 bg-zinc-800 rounded">{s.channel || "web"}</span>
                </div>
                {s.summary && <p className="text-sm text-zinc-400 mt-1">{s.summary.slice(0, 120)}</p>}
                <div className="flex gap-3 text-xs text-zinc-500 mt-1">
                  <span>{new Date(s.created_at).toLocaleDateString()}</span>
                  {s.input_tokens > 0 && <span>{s.input_tokens + s.output_tokens} tokens</span>}
                </div>
              </div>
              <div className="flex gap-2">
                <a href={`/chat?session=${s.id}`} className="px-3 py-1 bg-purple-600/20 text-purple-400 rounded text-sm hover:bg-purple-600/30">Resume</a>
                <button onClick={() => deleteSession(s.id)} className="px-3 py-1 bg-red-600/20 text-red-400 rounded text-sm hover:bg-red-600/30">Delete</button>
              </div>
            </div>
          ))}
          {sessions.length === 0 && <p className="text-zinc-500 text-center py-8">No sessions yet. Start a chat to create one.</p>}
        </div>
      </div>
    </AppShell>
  );
}
