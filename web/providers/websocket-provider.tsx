'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, type ReactNode } from 'react';
import { connectWebSocket, disconnectWebSocket } from '@/lib/websocket';

export function WebSocketProvider({ children }: { children: ReactNode }) {
  useEffect(() => {
    connectWebSocket();
    return () => disconnectWebSocket();
  }, []);

  return <>{children}</>;
}
