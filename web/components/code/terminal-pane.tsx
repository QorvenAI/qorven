'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useRef, useCallback, useState } from 'react';
import { Terminal, Loader2, AlertCircle, RefreshCw } from 'lucide-react';
import { terminal } from '@/lib/api';
import { cn } from '@/lib/utils';

interface Props {
  /** Called when a PTY command produces output (for parent log, optional). */
  onOutput?: (line: string) => void;
}

type ConnState = 'connecting' | 'open' | 'closed' | 'error';

/**
 * Real PTY terminal pane. Creates a session via POST /v1/terminal/sessions
 * then connects to the returned session's WebSocket endpoint.
 *
 * Protocol (JSON frames):
 *   Server → Client: { type: "output", data: "<raw bytes as string>" }
 *                    { type: "closed", code: <exit code> }
 *   Client → Server: { type: "input", data: "<keystroke string>" }
 *                    { type: "resize", cols: N, rows: N }
 */
export function TerminalPane({ onOutput }: Props) {
  const [lines, setLines] = useState<string[]>([]);
  const [connState, setConnState] = useState<ConnState>('connecting');
  const [error, setError] = useState<string | null>(null);
  const [cmd, setCmd] = useState('');

  const sessionIdRef = useRef<string | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const bottomRef = useRef<HTMLDivElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);

  const appendLine = useCallback((text: string) => {
    setLines(prev => {
      // Split raw output on newlines, preserving carriage returns as display
      const parts = text.split('\n');
      return [...prev, ...parts.filter(p => p !== '')];
    });
    onOutput?.(text);
  }, [onOutput]);

  const connect = useCallback(async () => {
    // Clean up any existing WS.
    if (wsRef.current) {
      wsRef.current.close();
      wsRef.current = null;
    }

    setConnState('connecting');
    setError(null);
    setLines([]);

    try {
      // Reuse existing session if we have one; otherwise create a new one.
      let id = sessionIdRef.current;
      if (!id) {
        const sess = await terminal.create('Code');
        id = sess.id;
        sessionIdRef.current = id;
      }

      const url = terminal.wsUrl(id);
      const ws = new WebSocket(url);
      wsRef.current = ws;

      ws.onopen = () => {
        setConnState('open');
        // Send initial resize based on container dimensions.
        sendResize(ws);
      };

      ws.onmessage = (ev) => {
        try {
          const msg = JSON.parse(ev.data as string) as { type: string; data?: string; code?: number };
          if (msg.type === 'output' && msg.data) {
            appendLine(msg.data);
          } else if (msg.type === 'closed') {
            setConnState('closed');
            appendLine(`\n[Process exited with code ${msg.code ?? 0}]`);
          }
        } catch {
          // non-JSON frame — treat as raw output
          appendLine(ev.data as string);
        }
      };

      ws.onerror = () => {
        setConnState('error');
        setError('WebSocket error — check backend logs.');
      };

      ws.onclose = (ev) => {
        if (connState !== 'closed') {
          setConnState('closed');
          if (!ev.wasClean) appendLine('\n[Connection lost]');
        }
      };
    } catch (err) {
      setConnState('error');
      setError(err instanceof Error ? err.message : 'Failed to create terminal session');
    }
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  // Initial connect.
  useEffect(() => {
    connect();
    return () => {
      if (wsRef.current) wsRef.current.close();
      // Delete session on unmount so the backend doesn't accumulate idle sessions.
      if (sessionIdRef.current) {
        terminal.delete(sessionIdRef.current).catch(() => {});
        sessionIdRef.current = null;
      }
    };
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  // Auto-scroll to bottom on new output.
  useEffect(() => { bottomRef.current?.scrollIntoView({ behavior: 'auto' }); }, [lines]);

  // Observe container resize and send PTY resize event.
  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    const ro = new ResizeObserver(() => {
      if (wsRef.current?.readyState === WebSocket.OPEN) sendResize(wsRef.current);
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  const sendResize = (ws: WebSocket) => {
    const el = containerRef.current;
    if (!el) return;
    // Estimate cols/rows from container dimensions using 8px char width, 18px line height.
    const cols = Math.max(40, Math.floor(el.clientWidth / 8));
    const rows = Math.max(10, Math.floor(el.clientHeight / 18));
    ws.send(JSON.stringify({ type: 'resize', cols, rows }));
  };

  const sendInput = useCallback((data: string) => {
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      wsRef.current.send(JSON.stringify({ type: 'input', data }));
    }
  }, []);

  const handleKeyDown = useCallback((e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter') {
      if (!cmd.trim()) return;
      // Send the command + newline to the PTY.
      sendInput(cmd + '\n');
      setCmd('');
      e.preventDefault();
    } else if (e.key === 'c' && e.ctrlKey) {
      // Ctrl+C — send ETX
      sendInput('\x03');
      e.preventDefault();
    } else if (e.key === 'd' && e.ctrlKey) {
      // Ctrl+D — send EOT
      sendInput('\x04');
      e.preventDefault();
    }
  }, [cmd, sendInput]);

  const handleReconnect = useCallback(() => {
    sessionIdRef.current = null; // force a new session
    connect();
  }, [connect]);

  return (
    <div ref={containerRef} className="flex h-full flex-col bg-background font-mono text-xs">
      {/* Header */}
      <div className="flex h-8 shrink-0 items-center justify-between border-b border-border px-3 gap-2">
        <div className="flex items-center gap-2">
          <Terminal className="h-3.5 w-3.5 text-muted-foreground" />
          <span className="text-xs font-medium text-foreground">Terminal</span>
          <span className={cn(
            'h-1.5 w-1.5 rounded-full',
            connState === 'open' ? 'bg-emerald-500' :
            connState === 'connecting' ? 'bg-amber-500 animate-pulse' :
            'bg-red-500'
          )} />
          {connState === 'connecting' && <Loader2 className="h-3 w-3 animate-spin text-muted-foreground" />}
          {connState === 'error' && <AlertCircle className="h-3 w-3 text-destructive" />}
        </div>
        {(connState === 'closed' || connState === 'error') && (
          <button
            onClick={handleReconnect}
            className="flex items-center gap-1 rounded px-2 py-0.5 text-2xs text-muted-foreground hover:bg-accent hover:text-foreground"
          >
            <RefreshCw className="h-3 w-3" /> Reconnect
          </button>
        )}
      </div>

      {/* Error banner */}
      {error && (
        <div className="shrink-0 border-b border-destructive/30 bg-destructive/10 px-3 py-1 text-2xs text-destructive">
          {error}
        </div>
      )}

      {/* Output */}
      <div className="flex-1 overflow-y-auto px-3 py-2 leading-[18px]">
        {lines.length === 0 && connState === 'open' && (
          <span className="text-muted-foreground/50">Connected. Type a command.</span>
        )}
        {lines.map((line, i) => (
          <div key={i} className={cn(
            'whitespace-pre-wrap break-all',
            line.startsWith('$') ? 'text-emerald-400' :
            line.startsWith('Error') || line.startsWith('[Process exited') || line.startsWith('[Connection') ? 'text-red-400' :
            'text-foreground/90'
          )}>
            {line}
          </div>
        ))}
        <div ref={bottomRef} />
      </div>

      {/* Input */}
      <div className={cn(
        'flex h-7 shrink-0 items-center border-t border-border px-3 gap-2',
        connState !== 'open' && 'opacity-50',
      )}>
        <span className="text-emerald-400">$</span>
        <input
          value={cmd}
          disabled={connState !== 'open'}
          onChange={e => setCmd(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder={connState === 'connecting' ? 'Connecting…' : connState === 'open' ? 'Enter command…' : 'Disconnected'}
          className="flex-1 bg-transparent outline-none placeholder:text-muted-foreground/40 text-foreground"
        />
      </div>
    </div>
  );
}
