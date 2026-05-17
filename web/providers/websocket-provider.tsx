'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useEffect, type ReactNode } from 'react';
import { connectWebSocket, disconnectWebSocket } from '@/lib/websocket';

export function WebSocketProvider({ children }: { children: ReactNode }) {
  useEffect(() => {
    connectWebSocket();
    return () => disconnectWebSocket();
  }, []);

  return <>{children}</>;
}
