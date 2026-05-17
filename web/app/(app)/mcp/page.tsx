'use client'

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useState, useEffect } from 'react'
import { Plug, Trash2 } from 'lucide-react'
import { EmptyState, emptyStates } from '@/components/empty-state';
import { mcp } from '@/lib/api';

type Server = { id: string; name: string; status?: string; tool_count?: number; url?: string; command?: string }

export default function McpPage() {
  const [servers, setServers] = useState<Server[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [showForm, setShowForm] = useState(false)
  const [name, setName] = useState('')
  const [command, setCommand] = useState('')
  const [deleting, setDeleting] = useState<string | null>(null)

  const fetchServers = () => {
    setLoading(true)
    mcp.servers()
      .then(d => setServers(d as Server[]))
      .catch(e => setError(e.message))
      .finally(() => setLoading(false))
  }

  useEffect(() => { fetchServers() }, [])

  const addServer = () => {
    if (!name.trim() || !command.trim()) return
    mcp.createServer(name.trim(), command.trim())
      .then(() => { setName(''); setCommand(''); setShowForm(false); fetchServers() })
      .catch(e => setError(e instanceof Error ? e.message : 'Failed to add server'))
  }

  const deleteServer = (id: string) => {
    setDeleting(id)
    mcp.deleteServer(id)
      .then(() => fetchServers())
      .catch(e => setError(e instanceof Error ? e.message : 'Failed to delete server'))
      .finally(() => setDeleting(null))
  }

  const connected = servers.filter(s => s.status === 'connected').length

  // Empty state for fresh install
  // if (servers.length === 0) return <EmptyState {...emptyStates.mcp} />;
  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <Plug className="text-primary" size={28} />
          <h1 className="text-lg font-semibold">MCP Servers</h1>
          {!loading && <span className="text-sm text-muted-foreground">{connected}/{servers.length} connected</span>}
        </div>
        <button onClick={() => setShowForm(!showForm)} className="bg-primary hover:bg-primary/90 px-4 py-2 rounded-lg text-sm font-medium transition">
          {showForm ? 'Cancel' : 'Add Server'}
        </button>
      </div>

      {showForm && (
        <div className="bg-card border border-border rounded-xl p-5">
          <h2 className="text-sm font-medium text-muted-foreground mb-3 uppercase tracking-wider">New Server</h2>
          <div className="flex gap-3">
            <input value={name} onChange={e => setName(e.target.value)} placeholder="Server name"
              className="qr-input flex-1" />
            <input value={command} onChange={e => setCommand(e.target.value)} placeholder="Command"
              className="qr-input flex-1" />
            <button onClick={addServer} className="bg-primary hover:bg-primary/90 px-4 py-2 rounded-lg text-sm">Save</button>
          </div>
        </div>
      )}

      {error && <p className="text-destructive mb-4">{error}</p>}

      {loading ? (
        <p className="text-muted-foreground">Loading servers…</p>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {servers.map(s => (
            <div key={s.id} className="bg-card border border-border rounded-xl p-5">
              <div className="flex items-center justify-between mb-3">
                <h3 className="font-semibold text-lg">{s.name}</h3>
                <div className="flex items-center gap-2">
                  <span className={`inline-block w-2.5 h-2.5 rounded-full ${s.status === 'connected' ? 'bg-green-500' : 'bg-red-500'}`} />
                  <span className="text-xs text-muted-foreground">{s.status ?? 'unknown'}</span>
                </div>
              </div>
              <div className="space-y-1 text-sm text-muted-foreground mb-4">
                <p>{s.tool_count ?? 0} tools available</p>
                {s.url && <p className="truncate">{s.url}</p>}
                {s.command && <p className="truncate font-mono text-xs">{s.command}</p>}
              </div>
              <button onClick={() => deleteServer(s.id)} disabled={deleting === s.id}
                className="flex items-center gap-1.5 text-destructive hover:text-red-300 text-sm disabled:opacity-50 transition">
                <Trash2 size={14} />
                {deleting === s.id ? 'Deleting…' : 'Delete'}
              </button>
            </div>
          ))}
          {servers.length === 0 && (
            <div className="col-span-full">
              <EmptyState
                {...emptyStates.mcp}
                description="No MCP servers configured yet. Click Add Server to connect an external tool server."
              />
            </div>
          )}
        </div>
      )}
    </div>
  )
}
