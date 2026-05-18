'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

// Terminal page — persistent tmux-backed sessions
// Each tab = one tmux session. Survives navigation and browser disconnect.
// Reconnects automatically when you return to the page.

import { useState, useEffect, useRef, useCallback } from 'react';
import { Plus, X, SquareTerminal, RefreshCw, Trash2, Loader2 } from 'lucide-react';
import { cn } from '@/lib/utils';
import { wsBase } from '@/lib/api-url';
import { request, getToken } from '@/lib/api-core';

interface TermSession {
  id: string;
  name: string;
  created_at: string;
}

export default function TerminalPage() {
  const [sessions, setSessions] = useState<TermSession[]>([]);
  const [activeId, setActiveId] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [creating, setCreating] = useState(false);
  const [newName, setNewName] = useState('');
  const [showCreate, setShowCreate] = useState(false);

  // Load existing tmux sessions from backend
  const loadSessions = useCallback(async () => {
    try {
      const data = await request<any>('/terminal/sessions');
      const list: TermSession[] = Array.isArray(data) ? data : data.sessions ?? [];
      setSessions(list);
      if (list.length > 0 && !activeId) setActiveId(list[0]!.id);
    } catch {}
    setLoading(false);
  }, [activeId]);

  useEffect(() => { loadSessions(); }, []);

  const createSession = async (name: string) => {
    setCreating(true);
    try {
      const sess = await request<TermSession>('/terminal/sessions', {
        method: 'POST',
        body: JSON.stringify({ name: name || `Terminal ${sessions.length + 1}` }),
      });
      setSessions(prev => [...prev, sess]);
      setActiveId(sess.id);
      setNewName('');
      setShowCreate(false);
    } catch {}
    setCreating(false);
  };

  const deleteSession = async (id: string) => {
    try {
      await request<any>(`/terminal/sessions/${id}`, { method: 'DELETE' });
      setSessions(prev => prev.filter(s => s.id !== id));
      if (activeId === id) setActiveId(sessions.find(s => s.id !== id)?.id ?? null);
    } catch {}
  };

  // If no sessions at all, auto-create one
  useEffect(() => {
    if (!loading && sessions.length === 0) {
      createSession('main');
    }
  }, [loading]);

  return (
    <div className="full-bleed flex flex-col bg-background" style={{ height: 'calc(100vh - var(--header-height))' }}>

      {/* Tab bar */}
      <div className="flex items-center border-b border-border bg-muted/10 overflow-x-auto scrollbar-none shrink-0">
        {sessions.map(sess => (
          <div key={sess.id}
            className={cn(
              'group flex items-center gap-1.5 pl-3 pr-1.5 py-2 text-xs border-r border-border cursor-pointer shrink-0 transition-colors',
              sess.id === activeId ? 'bg-background text-foreground border-b-2 border-b-primary' : 'text-muted-foreground hover:bg-accent/50 hover:text-foreground'
            )}
            onClick={() => setActiveId(sess.id)}>
            <SquareTerminal className="h-3.5 w-3.5 shrink-0" />
            <span className="max-w-[120px] truncate">{sess.name}</span>
            <button
              onClick={e => { e.stopPropagation(); deleteSession(sess.id); }}
              className="opacity-0 group-hover:opacity-100 h-4 w-4 flex items-center justify-center rounded hover:bg-destructive/20 hover:text-destructive transition-all ml-0.5">
              <X className="h-3 w-3" />
            </button>
          </div>
        ))}

        {/* New tab button */}
        {showCreate ? (
          <div className="flex items-center gap-1.5 px-2 py-1.5 shrink-0">
            <input
              autoFocus
              value={newName}
              onChange={e => setNewName(e.target.value)}
              onKeyDown={e => { if (e.key === 'Enter') createSession(newName); if (e.key === 'Escape') setShowCreate(false); }}
              placeholder="Session name…"
              className="qr-input w-32 text-xs h-6 py-0" />
            <button onClick={() => createSession(newName)} disabled={creating}
              className="text-xs text-primary hover:underline cursor-pointer disabled:opacity-50">
              {creating ? <Loader2 className="h-3 w-3 animate-spin" /> : 'Create'}
            </button>
            <button onClick={() => setShowCreate(false)} className="text-xs text-muted-foreground hover:text-foreground cursor-pointer">Cancel</button>
          </div>
        ) : (
          <button
            onClick={() => setShowCreate(true)}
            className="flex h-9 w-9 items-center justify-center text-muted-foreground hover:text-foreground hover:bg-accent transition-colors shrink-0">
            <Plus className="h-4 w-4" />
          </button>
        )}

        <div className="ml-auto flex items-center px-2 gap-1">
          <button onClick={loadSessions} title="Refresh sessions" className="h-7 w-7 flex items-center justify-center rounded text-muted-foreground hover:text-foreground hover:bg-accent">
            <RefreshCw className="h-3.5 w-3.5" />
          </button>
        </div>
      </div>

      {/* Terminal area */}
      <div className="flex-1 min-h-0">
        {loading ? (
          <div className="flex h-full items-center justify-center">
            <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
          </div>
        ) : activeId ? (
          <TerminalPane key={activeId} sessionId={activeId} />
        ) : (
          <div className="flex h-full flex-col items-center justify-center gap-3">
            <SquareTerminal className="h-12 w-12 text-muted-foreground/30" />
            <p className="text-sm text-muted-foreground">No terminal sessions</p>
            <button onClick={() => createSession('main')} disabled={creating}
              className="rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50 cursor-pointer">
              {creating ? 'Creating…' : 'New Terminal'}
            </button>
          </div>
        )}
      </div>
    </div>
  );
}

// ─── Terminal pane — WebSocket to backend tmux session ───────────────────────

function TerminalPane({ sessionId }: { sessionId: string }) {
  const outputRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const [lines, setLines] = useState<string[]>(['Connecting to session…']);
  const [cmd, setCmd] = useState('');
  const [connected, setConnected] = useState(false);

  // Connect to the tmux session via WebSocket
  useEffect(() => {
    const url = wsBase(`/v1/terminal/sessions/${sessionId}/ws?token=${getToken()}`);
    const ws = new WebSocket(url);
    wsRef.current = ws;

    ws.onopen = () => {
      setConnected(true);
      setLines(prev => [...prev.filter(l => l !== 'Connecting to session…'), '']);
    };

    ws.onmessage = (e) => {
      // Backend sends terminal output as raw text or JSON {type:'output',data:'...'}
      try {
        const msg = JSON.parse(e.data);
        if (msg.type === 'output' || msg.type === 'data') {
          appendOutput(msg.data || msg.output || '');
        }
      } catch {
        appendOutput(e.data);
      }
    };

    ws.onclose = () => {
      setConnected(false);
      setLines(prev => [...prev, '', '─── Session disconnected. Click reconnect to reattach. ───']);
    };

    ws.onerror = () => {
      setLines(prev => [...prev, '', '─── WebSocket error. The terminal backend may not be running. ───',
        '    You can still run commands via the exec tool in your Qors chat.',
      ]);
    };

    return () => { ws.close(); };
  }, [sessionId]);

  const appendOutput = (text: string) => {
    const newLines = text.split('\n');
    setLines(prev => {
      const combined = [...prev, ...newLines];
      // Keep last 2000 lines to prevent memory bloat
      return combined.length > 2000 ? combined.slice(combined.length - 2000) : combined;
    });
  };

  useEffect(() => {
    if (outputRef.current) {
      outputRef.current.scrollTop = outputRef.current.scrollHeight;
    }
  }, [lines]);

  const sendCmd = () => {
    if (!cmd.trim()) return;
    const text = cmd.trim();
    setCmd('');
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      wsRef.current.send(JSON.stringify({ type: 'input', data: text + '\n' }));
    } else {
      // Fallback: run via exec API
      setLines(prev => [...prev, `$ ${text}`, '(WebSocket not connected — running via API…)']);
      runViaApi(text);
    }
  };

  const runViaApi = async (command: string) => {
    try {
      const data = await request<any>('/chat/completions', {
        method: 'POST',
        body: JSON.stringify({
          agent_id: 'prime', message: `Run this shell command using the exec tool: ${command}`, stream: false,
        }),
      });
      const output = data.choices?.[0]?.message?.content || '(no output)';
      setLines(prev => [...prev, output]);
    } catch (e) {
      setLines(prev => [...prev, `Error: ${e instanceof Error ? e.message : 'API call failed'}`]);
    }
  };

  const reconnect = () => {
    if (wsRef.current) wsRef.current.close();
    setLines(['Reconnecting…']);
    // The useEffect cleanup + re-run will handle reconnect
    // but since sessionId is same key, we need to force it
    const url = wsBase(`/v1/terminal/sessions/${sessionId}/ws?token=${getToken()}`);
    const ws = new WebSocket(url);
    wsRef.current = ws;
    ws.onopen = () => { setConnected(true); setLines(['Reconnected.', '']); };
    ws.onmessage = (e) => { try { const m = JSON.parse(e.data); appendOutput(m.data || m.output || ''); } catch { appendOutput(e.data); } };
    ws.onclose = () => { setConnected(false); };
  };

  return (
    <div className="flex h-full flex-col bg-background font-mono text-sm"
      onClick={() => inputRef.current?.focus()}>

      {/* Output */}
      <div ref={outputRef} className="flex-1 overflow-y-auto p-3 space-y-0 scrollbar-none">
        {lines.map((line, i) => (
          <div key={i} className={cn('leading-[1.5] whitespace-pre-wrap break-all text-[12px]',
            line.startsWith('─── ') ? 'text-yellow-500/70' : 'text-emerald-300/90')}>
            {line || '\u00A0'}
          </div>
        ))}
      </div>

      {/* Input */}
      <div className="border-t border-border/30 px-3 py-2 flex items-center gap-2">
        {!connected && (
          <button onClick={reconnect} title="Reconnect"
            className="flex h-5 w-5 items-center justify-center rounded text-amber-400 hover:bg-amber-400/10">
            <RefreshCw className="h-3 w-3" />
          </button>
        )}
        <span className="text-primary text-[12px] shrink-0">
          {connected ? '❯' : '○'}
        </span>
        <input
          ref={inputRef}
          value={cmd}
          onChange={e => setCmd(e.target.value)}
          onKeyDown={e => { if (e.key === 'Enter') sendCmd(); }}
          placeholder={connected ? 'Enter command…' : 'Not connected — commands run via API'}
          className="flex-1 bg-transparent text-[12px] text-emerald-400 outline-none placeholder:text-muted-foreground/40"
          autoFocus />
      </div>
    </div>
  );
}
