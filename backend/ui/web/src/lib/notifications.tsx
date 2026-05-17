"use client";

import { createContext, useContext, useState, useCallback, type ReactNode } from "react";

export interface Notification {
  id: string;
  soulKey: string;
  soulName: string;
  message: string;
  ts: number;
  read: boolean;
}

interface NotificationCtx {
  notifications: Notification[];
  unreadCount: number;
  unreadBySoul: Record<string, number>;
  add: (n: Omit<Notification, "id" | "read">) => void;
  markAllRead: () => void;
  markSoulRead: (soulKey: string) => void;
}

const Ctx = createContext<NotificationCtx>({
  notifications: [], unreadCount: 0, unreadBySoul: {},
  add: () => {}, markAllRead: () => {}, markSoulRead: () => {},
});

export function NotificationProvider({ children }: { children: ReactNode }) {
  const [notifications, setNotifications] = useState<Notification[]>([]);

  const add = useCallback((n: Omit<Notification, "id" | "read">) => {
    setNotifications(prev => [{
      ...n, id: `${n.ts}-${Math.random().toString(36).slice(2, 6)}`, read: false,
    }, ...prev.slice(0, 49)]);
  }, []);

  const markAllRead = useCallback(() => {
    setNotifications(prev => prev.map(n => ({ ...n, read: true })));
  }, []);

  const markSoulRead = useCallback((soulKey: string) => {
    setNotifications(prev => prev.map(n => n.soulKey === soulKey ? { ...n, read: true } : n));
  }, []);

  const unreadCount = notifications.filter(n => !n.read).length;
  const unreadBySoul: Record<string, number> = {};
  notifications.filter(n => !n.read).forEach(n => {
    unreadBySoul[n.soulKey] = (unreadBySoul[n.soulKey] || 0) + 1;
  });

  return (
    <Ctx.Provider value={{ notifications, unreadCount, unreadBySoul, add, markAllRead, markSoulRead }}>
      {children}
    </Ctx.Provider>
  );
}

export function useNotifications() { return useContext(Ctx); }
