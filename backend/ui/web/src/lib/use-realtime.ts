"use client";

import { useEffect, useRef, useState, useCallback } from "react";

export interface RealtimeEvent {
  type: string;
  session_id?: string;
  agent_id?: string;
  data?: any;
  timestamp: number;
}

interface UseRealtimeOptions {
  onMessage?: (event: RealtimeEvent) => void;
  onNotification?: (title: string, body: string) => void;
}

export function useRealtime(options: UseRealtimeOptions = {}) {
  const [connected, setConnected] = useState(false);
  const [clientCount, setClientCount] = useState(0);
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectTimer = useRef<NodeJS.Timeout>();
  const optionsRef = useRef(options);
  optionsRef.current = options;

  const connect = useCallback(() => {
    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const wsUrl = `${protocol}//${window.location.hostname}:4200/ws/realtime`;

    const ws = new WebSocket(wsUrl);
    wsRef.current = ws;

    ws.onopen = () => {
      setConnected(true);
      // Request notification permission
      if ("Notification" in window && Notification.permission === "default") {
        Notification.requestPermission();
      }
    };

    ws.onmessage = (e) => {
      try {
        const event: RealtimeEvent = JSON.parse(e.data);

        // Call handler
        optionsRef.current.onMessage?.(event);

        // Browser notification for background tabs
        if (document.hidden && event.type === "new_message") {
          const data = event.data as any;
          if (data?.role === "assistant" && "Notification" in window && Notification.permission === "granted") {
            new Notification("Qorven", {
              body: (data.content || "").slice(0, 100),
              icon: "/logo.svg",
            });
          }
        }

        if (event.type === "notification") {
          const data = event.data as any;
          optionsRef.current.onNotification?.(data?.title || "", data?.body || "");
          if ("Notification" in window && Notification.permission === "granted") {
            new Notification(data?.title || "Qorven", {
              body: data?.body || "",
              icon: "/logo.svg",
            });
          }
        }
      } catch {}
    };

    ws.onclose = () => {
      setConnected(false);
      wsRef.current = null;
      // Reconnect after 3s
      reconnectTimer.current = setTimeout(connect, 3000);
    };

    ws.onerror = () => {
      ws.close();
    };
  }, []);

  useEffect(() => {
    connect();
    return () => {
      clearTimeout(reconnectTimer.current);
      wsRef.current?.close();
    };
  }, [connect]);

  return { connected };
}
