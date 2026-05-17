"use client";

import { useEffect, useState } from "react";
import { AppShell } from "@/components/layout/app-shell";

const API = process.env.NEXT_PUBLIC_API_URL || "http://localhost:4200";
const TOKEN = process.env.NEXT_PUBLIC_API_TOKEN || "";

interface Provider { name: string; display_name: string; provider_type: string; enabled: boolean; api_base: string; }
interface Health { status: string; version: string; uptime: string; tools: number; providers: number; agents: number; }

export default function SettingsPage() {
  const [health, setHealth] = useState<Health | null>(null);
  const [providers, setProviders] = useState<Provider[]>([]);

  useEffect(() => {
    fetch(`${API}/health/detailed`, { headers: { Authorization: `Bearer ${TOKEN}` } })
      .then(r => r.json()).then(setHealth).catch(() => {});
    fetch(`${API}/v1/providers`, { headers: { Authorization: `Bearer ${TOKEN}` } })
      .then(r => r.json()).then(d => setProviders(d.providers || [])).catch(() => {});
  }, []);

  return (
    <AppShell>
      <div className="p-6 max-w-4xl mx-auto">
        <h1 className="text-2xl font-bold mb-6">Settings</h1>

        {/* System Status */}
        <div className="mb-8">
          <h2 className="text-lg font-semibold mb-3">System Status</h2>
          {health ? (
            <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
              <StatusCard label="Status" value={health.status} color={health.status === "ok" ? "green" : "red"} />
              <StatusCard label="Version" value={health.version} />
              <StatusCard label="Uptime" value={health.uptime?.split(".")[0] || "—"} />
              <StatusCard label="Tools" value={String(health.tools || 0)} />
            </div>
          ) : (
            <p className="text-zinc-500">Loading...</p>
          )}
        </div>

        {/* Providers */}
        <div className="mb-8">
          <h2 className="text-lg font-semibold mb-3">LLM Providers</h2>
          <div className="space-y-2">
            {providers.map(p => (
              <div key={p.name} className="p-3 border border-zinc-700 rounded-lg bg-zinc-900 flex justify-between items-center">
                <div>
                  <span className="font-medium">{p.display_name || p.name}</span>
                  <span className="text-xs text-zinc-500 ml-2">{p.provider_type}</span>
                  {p.api_base && <span className="text-xs text-zinc-600 ml-2">{p.api_base}</span>}
                </div>
                <span className={`text-xs px-2 py-0.5 rounded ${p.enabled ? "bg-green-900/30 text-green-400" : "bg-zinc-800 text-zinc-500"}`}>
                  {p.enabled ? "enabled" : "disabled"}
                </span>
              </div>
            ))}
            {providers.length === 0 && <p className="text-zinc-500">No providers configured. Run <code className="bg-zinc-800 px-1 rounded">qorven init</code> to set up.</p>}
          </div>
        </div>

        {/* API Info */}
        <div>
          <h2 className="text-lg font-semibold mb-3">API</h2>
          <div className="p-4 border border-zinc-700 rounded-lg bg-zinc-900 text-sm font-mono">
            <p>Gateway: {API}</p>
            <p className="mt-1">Health: <a href={`${API}/health`} className="text-purple-400 hover:underline">{API}/health</a></p>
            <p className="mt-1">Chat: POST {API}/v1/chat/completions</p>
            <p className="mt-1">Graph: GET {API}/v1/graph</p>
          </div>
        </div>
      </div>
    </AppShell>
  );
}

function StatusCard({ label, value, color }: { label: string; value: string; color?: string }) {
  const colorClass = color === "green" ? "text-green-400" : color === "red" ? "text-red-400" : "text-zinc-200";
  return (
    <div className="p-3 border border-zinc-700 rounded-lg bg-zinc-900">
      <p className="text-xs text-zinc-500">{label}</p>
      <p className={`text-lg font-semibold ${colorClass}`}>{value}</p>
    </div>
  );
}
