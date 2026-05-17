"use client";

import { useEffect, useState } from "react";
import { AppShell } from "@/components/layout/app-shell";

interface Agent {
  id: string;
  agent_key: string;
  display_name: string;
  model: string;
  system_prompt: string;
  status: string;
}

const API = process.env.NEXT_PUBLIC_API_URL || "http://localhost:4200";
const TOKEN = process.env.NEXT_PUBLIC_API_TOKEN || "";

async function api(path: string, opts?: RequestInit) {
  const res = await fetch(`${API}/v1${path}`, {
    ...opts,
    headers: { "Authorization": `Bearer ${TOKEN}`, "Content-Type": "application/json", ...opts?.headers },
  });
  return res.json();
}

export default function AgentsPage() {
  const [agents, setAgents] = useState<Agent[]>([]);
  const [creating, setCreating] = useState(false);
  const [form, setForm] = useState({ key: "", name: "", model: "deepseek-chat", prompt: "" });

  useEffect(() => { loadAgents(); }, []);

  async function loadAgents() {
    const data = await api("/agents");
    setAgents(data.agents || []);
  }

  async function createAgent() {
    await api("/agents", {
      method: "POST",
      body: JSON.stringify({ agent_key: form.key, display_name: form.name, model: form.model, system_prompt: form.prompt }),
    });
    setCreating(false);
    setForm({ key: "", name: "", model: "deepseek-chat", prompt: "" });
    loadAgents();
  }

  async function deleteAgent(id: string) {
    if (!confirm("Delete this agent?")) return;
    await api(`/agents/${id}`, { method: "DELETE" });
    loadAgents();
  }

  return (
    <AppShell>
      <div className="p-6 max-w-4xl mx-auto">
        <div className="flex justify-between items-center mb-6">
          <h1 className="text-2xl font-bold">Agents</h1>
          <button onClick={() => setCreating(true)} className="px-4 py-2 bg-purple-600 text-white rounded-lg hover:bg-purple-700">
            + Create Agent
          </button>
        </div>

        {creating && (
          <div className="mb-6 p-4 border border-zinc-700 rounded-lg bg-zinc-900">
            <h2 className="text-lg font-semibold mb-3">New Agent</h2>
            <div className="grid grid-cols-2 gap-3">
              <input placeholder="Agent key (e.g. researcher)" value={form.key} onChange={e => setForm({...form, key: e.target.value})}
                className="px-3 py-2 bg-zinc-800 border border-zinc-700 rounded" />
              <input placeholder="Display name" value={form.name} onChange={e => setForm({...form, name: e.target.value})}
                className="px-3 py-2 bg-zinc-800 border border-zinc-700 rounded" />
              <select value={form.model} onChange={e => setForm({...form, model: e.target.value})}
                className="px-3 py-2 bg-zinc-800 border border-zinc-700 rounded">
                <option value="deepseek-chat">DeepSeek Chat</option>
                <option value="gpt-4o-mini">GPT-4o Mini</option>
                <option value="gemini-2.0-flash">Gemini 2.0 Flash</option>
                <option value="claude-sonnet-4-20250514">Claude Sonnet 4</option>
              </select>
              <div />
              <textarea placeholder="System prompt..." value={form.prompt} onChange={e => setForm({...form, prompt: e.target.value})}
                className="col-span-2 px-3 py-2 bg-zinc-800 border border-zinc-700 rounded h-24" />
            </div>
            <div className="flex gap-2 mt-3">
              <button onClick={createAgent} className="px-4 py-2 bg-green-600 text-white rounded hover:bg-green-700">Create</button>
              <button onClick={() => setCreating(false)} className="px-4 py-2 bg-zinc-700 text-white rounded hover:bg-zinc-600">Cancel</button>
            </div>
          </div>
        )}

        <div className="space-y-3">
          {agents.map(agent => (
            <div key={agent.id} className="p-4 border border-zinc-700 rounded-lg bg-zinc-900 flex justify-between items-start">
              <div>
                <div className="flex items-center gap-2">
                  <span className="text-lg font-semibold">{agent.display_name || agent.agent_key}</span>
                  <span className="text-xs px-2 py-0.5 bg-zinc-800 rounded">{agent.model}</span>
                </div>
                <p className="text-sm text-zinc-400 mt-1">{agent.system_prompt?.slice(0, 100)}{agent.system_prompt?.length > 100 ? "..." : ""}</p>
                <p className="text-xs text-zinc-500 mt-1">ID: {agent.id?.slice(0, 8)} · Key: {agent.agent_key}</p>
              </div>
              <div className="flex gap-2">
                <a href={`/chat?agent=${agent.agent_key}`} className="px-3 py-1 bg-purple-600/20 text-purple-400 rounded text-sm hover:bg-purple-600/30">Chat</a>
                <button onClick={() => deleteAgent(agent.id)} className="px-3 py-1 bg-red-600/20 text-red-400 rounded text-sm hover:bg-red-600/30">Delete</button>
              </div>
            </div>
          ))}
          {agents.length === 0 && <p className="text-zinc-500 text-center py-8">No agents yet. Create one to get started.</p>}
        </div>
      </div>
    </AppShell>
  );
}
