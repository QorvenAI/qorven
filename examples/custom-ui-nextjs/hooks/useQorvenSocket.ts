"use client";

import { useCallback, useEffect, useRef, useState } from "react";

import { wsBase } from "@/lib/api";
import { getAuth, subscribeAuth } from "@/lib/auth";
import type { QorvenEvent } from "@/lib/events";

/**
 * useQorvenSocket is a resilient WebSocket subscription to Qorven's
 * realtime hub. It handles:
 *
 *   • Token attachment via the `?token=` query parameter. The browser
 *     cannot set an `Authorization` header on a WebSocket upgrade,
 *     so the server (gateway/middleware.go:wsAuth) reads the JWT
 *     from the query string.
 *
 *   • Ping keep-alives at PING_INTERVAL. If the browser WebSocket
 *     has been silent for longer than PING_INTERVAL × STALE_MULTIPLIER
 *     we declare the socket dead and start reconnecting — the server
 *     may be up but the TCP path between us can be gone without any
 *     FIN ever landing (mobile networks, NAT timeouts, tunneled VPNs).
 *     Browsers do NOT automatically surface dead-TCP-without-close;
 *     this is the only way to catch it.
 *
 *   • Exponential backoff reconnection: 1s, 2s, 4s, 8s, 16s, capped
 *     at 30s. Reset to 0 on a successful `onopen`.
 *
 *   • Cleanup on unmount. The socket is closed with code 1000 and
 *     the timers are cleared.
 *
 * ## Usage
 *
 *   const { status, events, lastError } = useQorvenSocket();
 *   useEffect(() => {
 *     const last = events[events.length - 1];
 *     if (!last) return;
 *     switch (last.type) { … }
 *   }, [events]);
 *
 * Events arrive in order (the hub emits monotonic `seq`); the hook
 * preserves that order. It caps the buffer at DEFAULT_BUFFER_SIZE to
 * prevent unbounded memory growth on a slow consumer.
 */

const PING_INTERVAL = 20_000; // 20s — RFC 6455 pings are cheap
const STALE_MULTIPLIER = 3; // declare dead after 60s of silence
const BACKOFF_CAP = 30_000; // 30s ceiling
const DEFAULT_BUFFER_SIZE = 500;

export type SocketStatus = "idle" | "connecting" | "open" | "closed" | "error";

export type UseQorvenSocketOptions = {
  /** Path suffix after `/ws/realtime`. Usually empty; tests use `?user_id=…`. */
  query?: Record<string, string>;
  /** Max events to keep in the buffer. Default 500. */
  bufferSize?: number;
  /** Called for every incoming event. Use for side effects (mutation, sound). */
  onEvent?: (ev: QorvenEvent) => void;
};

export function useQorvenSocket(opts: UseQorvenSocketOptions = {}) {
  const [status, setStatus] = useState<SocketStatus>("idle");
  const [events, setEvents] = useState<QorvenEvent[]>([]);
  const [lastError, setLastError] = useState<string | null>(null);

  const onEventRef = useRef(opts.onEvent);
  const bufferSizeRef = useRef(opts.bufferSize ?? DEFAULT_BUFFER_SIZE);
  const queryRef = useRef(opts.query);
  // Update refs every render so the effect doesn't re-run just
  // because the caller passed a fresh callback.
  onEventRef.current = opts.onEvent;
  bufferSizeRef.current = opts.bufferSize ?? DEFAULT_BUFFER_SIZE;
  queryRef.current = opts.query;

  const socketRef = useRef<WebSocket | null>(null);
  const attemptRef = useRef(0);
  const reconnectTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const pingTimer = useRef<ReturnType<typeof setInterval> | null>(null);
  const lastTrafficAt = useRef<number>(Date.now());
  const stopRef = useRef(false);

  // Re-subscribe whenever the auth token changes (login/logout).
  const [authTick, setAuthTick] = useState(0);
  useEffect(() => subscribeAuth(() => setAuthTick((v) => v + 1)), []);

  const buildURL = useCallback((): string | null => {
    const { token } = getAuth();
    if (!token) return null;
    const base = wsBase();
    const url = new URL(`${base}/ws/realtime`);
    url.searchParams.set("token", token);
    const extra = queryRef.current;
    if (extra) {
      for (const [k, v] of Object.entries(extra)) {
        url.searchParams.set(k, v);
      }
    }
    return url.toString();
  }, []);

  const cleanup = useCallback(() => {
    if (reconnectTimer.current) {
      clearTimeout(reconnectTimer.current);
      reconnectTimer.current = null;
    }
    if (pingTimer.current) {
      clearInterval(pingTimer.current);
      pingTimer.current = null;
    }
    const sock = socketRef.current;
    socketRef.current = null;
    if (sock && sock.readyState <= WebSocket.OPEN) {
      try {
        sock.close(1000, "client unmount");
      } catch {
        // non-fatal
      }
    }
  }, []);

  const scheduleReconnect = useCallback(
    (connect: () => void) => {
      if (stopRef.current) return;
      attemptRef.current += 1;
      const delay = Math.min(
        BACKOFF_CAP,
        1000 * Math.pow(2, attemptRef.current - 1),
      );
      reconnectTimer.current = setTimeout(connect, delay);
    },
    [],
  );

  useEffect(() => {
    stopRef.current = false;

    const connect = () => {
      const url = buildURL();
      if (!url) {
        // No token yet — stay idle; the auth subscription above will
        // re-trigger this effect when login happens.
        setStatus("idle");
        return;
      }

      setStatus("connecting");
      setLastError(null);

      let sock: WebSocket;
      try {
        sock = new WebSocket(url);
      } catch (e) {
        setStatus("error");
        setLastError(e instanceof Error ? e.message : String(e));
        scheduleReconnect(connect);
        return;
      }
      socketRef.current = sock;

      sock.onopen = () => {
        attemptRef.current = 0; // reset backoff on successful open
        lastTrafficAt.current = Date.now();
        setStatus("open");
        setLastError(null);

        // Keep-alive pings. RFC 6455 doesn't let us send raw control
        // pings from the browser API — all browser WebSocket "pings"
        // are actually application-level messages. We send a tiny
        // JSON ping that the server ignores. The existence of server
        // traffic (any message) resets lastTrafficAt; if nothing
        // arrives for STALE_MULTIPLIER × PING_INTERVAL we declare the
        // socket dead.
        pingTimer.current = setInterval(() => {
          try {
            if (sock.readyState === WebSocket.OPEN) {
              sock.send(JSON.stringify({ type: "ping" }));
            }
          } catch {
            // ignore — if the underlying socket broke, onclose will fire
          }
          const silentFor = Date.now() - lastTrafficAt.current;
          if (silentFor > PING_INTERVAL * STALE_MULTIPLIER) {
            try {
              sock.close(4000, "stale: no server traffic");
            } catch {
              // ignore
            }
          }
        }, PING_INTERVAL);
      };

      sock.onmessage = (msg) => {
        lastTrafficAt.current = Date.now();
        let ev: QorvenEvent;
        try {
          ev = JSON.parse(msg.data) as QorvenEvent;
        } catch {
          // Non-JSON — server doesn't send these today, but don't
          // crash on them either.
          return;
        }
        // Our own outbound pings may echo back from a debug server or
        // proxy. Filter them so consumers don't see noise.
        if ((ev as { type?: unknown }).type === "ping") return;

        setEvents((prev) => {
          const next = [...prev, ev];
          const cap = bufferSizeRef.current;
          return next.length > cap ? next.slice(next.length - cap) : next;
        });
        onEventRef.current?.(ev);
      };

      sock.onerror = () => {
        setStatus("error");
        setLastError("websocket error");
        // onerror is always followed by onclose; the reconnect logic
        // lives there so we don't double-schedule.
      };

      sock.onclose = (ev) => {
        if (pingTimer.current) {
          clearInterval(pingTimer.current);
          pingTimer.current = null;
        }
        socketRef.current = null;
        setStatus("closed");
        if (!lastError) {
          setLastError(
            ev.code === 1000 ? null : `closed: ${ev.code} ${ev.reason}`,
          );
        }
        if (!stopRef.current) scheduleReconnect(connect);
      };
    };

    connect();
    return () => {
      stopRef.current = true;
      cleanup();
    };
    // Re-run when the token changes (authTick bump) so we reconnect
    // with the new JWT. buildURL/cleanup/scheduleReconnect are stable.
  }, [authTick, buildURL, cleanup, scheduleReconnect]);

  // Public API.
  return { status, events, lastError };
}
