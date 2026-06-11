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

/** Read computed xterm theme colors from CSS variables set in config.qorven.css */
function getXtermTheme(el: Element) {
  const s = getComputedStyle(el);
  const v = (name: string) => s.getPropertyValue(name).trim();
  return {
    background:       v('--xterm-bg'),
    foreground:       v('--xterm-fg'),
    cursor:           v('--xterm-cursor'),
    cursorAccent:     v('--xterm-cursor-accent'),
    selectionBackground: v('--xterm-selection-bg'),
    black:            v('--xterm-black'),
    red:              v('--xterm-red'),
    green:            v('--xterm-green'),
    yellow:           v('--xterm-yellow'),
    blue:             v('--xterm-blue'),
    magenta:          v('--xterm-magenta'),
    cyan:             v('--xterm-cyan'),
    white:            v('--xterm-white'),
    brightBlack:      v('--xterm-bright-black'),
    brightRed:        v('--xterm-bright-red'),
    brightGreen:      v('--xterm-bright-green'),
    brightYellow:     v('--xterm-bright-yellow'),
    brightBlue:       v('--xterm-bright-blue'),
    brightMagenta:    v('--xterm-bright-magenta'),
    brightCyan:       v('--xterm-bright-cyan'),
    brightWhite:      v('--xterm-bright-white'),
  };
}

/**
 * Real PTY terminal pane backed by xterm.js. Creates a session via
 * POST /v1/terminal/sessions then connects to the returned WebSocket endpoint.
 *
 * Protocol (JSON frames):
 *   Server → Client: { type: "output", data: "<raw bytes as string>" }
 *                    { type: "closed", code: <exit code> }
 *   Client → Server: { type: "input", data: "<keystroke string>" }
 *                    { type: "resize", cols: N, rows: N }
 */
export function TerminalPane({ onOutput }: Props) {
  const [connState, setConnState] = useState<ConnState>('connecting');
  const [error, setError] = useState<string | null>(null);

  const sessionIdRef  = useRef<string | null>(null);
  const wsRef         = useRef<WebSocket | null>(null);
  const containerRef  = useRef<HTMLDivElement>(null);
  // xterm instance refs — typed as any to avoid importing types at module level
  // (the actual Terminal / FitAddon are dynamically imported inside useEffect)
  const termRef       = useRef<any>(null);
  const fitAddonRef   = useRef<any>(null);
  const onOutputRef   = useRef(onOutput);
  onOutputRef.current = onOutput;

  // ── helpers ──────────────────────────────────────────────────────────────

  const sendWs = useCallback((msg: object) => {
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      wsRef.current.send(JSON.stringify(msg));
    }
  }, []);

  // ── xterm bootstrap ──────────────────────────────────────────────────────

  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;

    let disposed = false;
    let term: any = null;
    let fitAddon: any = null;
    let ro: ResizeObserver | null = null;
    let onDataDisposable: { dispose: () => void } | null = null;
    let onResizeDisposable: { dispose: () => void } | null = null;

    async function init() {
      const { Terminal: XTerm }   = await import('@xterm/xterm');
      const { FitAddon }          = await import('@xterm/addon-fit');
      const { WebLinksAddon }     = await import('@xterm/addon-web-links');
      if (disposed || !containerRef.current) return;

      fitAddon = new FitAddon();
      term = new XTerm({
        fontFamily: '"JetBrains Mono", "Cascadia Code", ui-monospace, monospace',
        fontSize: 13,
        lineHeight: 1.4,
        scrollback: 10000,
        cursorBlink: true,
        cursorStyle: 'block',
        allowProposedApi: false,
        theme: getXtermTheme(document.documentElement),
      });

      term.loadAddon(fitAddon);
      term.loadAddon(new WebLinksAddon());
      term.open(containerRef.current);
      fitAddon.fit();

      termRef.current    = term;
      fitAddonRef.current = fitAddon;

      // Wire keystrokes → WS (xterm's onData sends raw chars incl. Ctrl-C/D)
      onDataDisposable = term.onData((data: string) => {
        sendWs({ type: 'input', data });
        onOutputRef.current?.(data);
      });

      // Wire PTY resize → WS
      onResizeDisposable = term.onResize(({ cols, rows }: { cols: number; rows: number }) => {
        sendWs({ type: 'resize', cols, rows });
      });

      // Re-fit + re-apply theme when the container resizes or dark-mode toggles
      ro = new ResizeObserver(() => {
        fitAddon?.fit();
      });
      ro.observe(containerRef.current!);

      // Connect WS now that xterm is ready
      connectWs(term, fitAddon);
    }

    init().catch(() => {});

    return () => {
      disposed = true;
      onDataDisposable?.dispose();
      onResizeDisposable?.dispose();
      ro?.disconnect();
      term?.dispose();
      termRef.current     = null;
      fitAddonRef.current = null;
    };
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  // Re-apply xterm theme when dark/light class toggles on <html>
  useEffect(() => {
    const update = () => {
      if (termRef.current) {
        termRef.current.options.theme = getXtermTheme(document.documentElement);
      }
    };
    const obs = new MutationObserver(update);
    obs.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] });
    return () => obs.disconnect();
  }, []);

  // ── WebSocket connect ─────────────────────────────────────────────────────

  const connectWs = useCallback(async (term: any, fitAddon: any) => {
    if (wsRef.current) {
      wsRef.current.close();
      wsRef.current = null;
    }

    setConnState('connecting');
    setError(null);

    try {
      let id = sessionIdRef.current;
      if (!id) {
        const sess = await terminal.create('Code');
        id = sess.id;
        sessionIdRef.current = id;
      }

      const url = terminal.wsUrl(id);
      const ws  = new WebSocket(url);
      wsRef.current = ws;

      ws.onopen = () => {
        setConnState('open');
        // Sync the PTY size to the actual xterm dimensions after connect
        if (fitAddon) fitAddon.fit();
      };

      ws.onmessage = (ev) => {
        try {
          const msg = JSON.parse(ev.data as string) as { type: string; data?: string; code?: number };
          if (msg.type === 'output' && msg.data) {
            term?.write(msg.data);
            onOutputRef.current?.(msg.data);
          } else if (msg.type === 'closed') {
            setConnState('closed');
            term?.write(`\r\n\x1b[2m[process exited with code ${msg.code ?? 0}]\x1b[0m\r\n`);
          }
        } catch {
          // Non-JSON frame — write raw
          term?.write(ev.data as string);
        }
      };

      ws.onerror = () => {
        setConnState('error');
        setError('WebSocket error — check backend logs.');
      };

      ws.onclose = (ev) => {
        setConnState(prev => {
          if (prev === 'closed') return prev;
          if (!ev.wasClean) term?.write('\r\n\x1b[2m[connection lost]\x1b[0m\r\n');
          return 'closed';
        });
      };
    } catch (err) {
      setConnState('error');
      setError(err instanceof Error ? err.message : 'Failed to create terminal session');
    }
  }, [sendWs]); // eslint-disable-line react-hooks/exhaustive-deps

  // Initial WS connect (xterm may not exist yet — connectWs is called from
  // within the xterm init useEffect once the Terminal is open).
  // Unmount cleanup: close WS + delete session
  useEffect(() => {
    return () => {
      if (wsRef.current) { wsRef.current.close(); wsRef.current = null; }
      if (sessionIdRef.current) {
        terminal.delete(sessionIdRef.current).catch(() => {});
        sessionIdRef.current = null;
      }
    };
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  const handleReconnect = useCallback(() => {
    sessionIdRef.current = null; // force a new session
    const t = termRef.current;
    const f = fitAddonRef.current;
    if (t) {
      t.clear();
      connectWs(t, f);
    }
  }, [connectWs]);

  // ── render ────────────────────────────────────────────────────────────────

  return (
    <div className="flex h-full flex-col bg-background">
      {/* Header */}
      <div className="flex h-8 shrink-0 items-center justify-between border-b border-border px-3 gap-2">
        <div className="flex items-center gap-2">
          <Terminal className="h-3.5 w-3.5 text-muted-foreground" />
          <span className="text-xs font-medium text-foreground">Terminal</span>
          <span className={cn(
            'h-1.5 w-1.5 rounded-full',
            connState === 'open'        ? 'bg-emerald-500' :
            connState === 'connecting'  ? 'bg-amber-500 animate-pulse' :
                                          'bg-red-500'
          )} />
          {connState === 'connecting' && <Loader2 className="h-3 w-3 animate-spin text-muted-foreground" />}
          {connState === 'error'      && <AlertCircle className="h-3 w-3 text-destructive" />}
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

      {/* xterm container — takes all remaining space */}
      <div
        ref={containerRef}
        className="flex-1 overflow-hidden p-1"
      />
    </div>
  );
}
