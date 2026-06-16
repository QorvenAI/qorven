'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, type ReactNode } from 'react';
import { connectWebSocket, disconnectWebSocket } from '@/lib/websocket';

export function WebSocketProvider({ children }: { children: ReactNode }) {
  useEffect(() => {
    // Don't open the realtime socket on the login/public pages: there's no
    // token, so it can only fail-loop — and its 401 probe would redirect to
    // /login, nesting a recursive `?next=/login?next=…` URL. The socket
    // connects once the user is authenticated inside the app shell.
    if (typeof window !== 'undefined' && window.location.pathname.startsWith('/login')) {
      return;
    }
    connectWebSocket();
    return () => disconnectWebSocket();
  }, []);

  return <>{children}</>;
}
