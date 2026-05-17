'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { createContext, useContext, useState, type ReactNode } from 'react';

interface ToolbarContextValue {
  left: ReactNode;
  right: ReactNode;
  setLeft: (node: ReactNode) => void;
  setRight: (node: ReactNode) => void;
}

const ToolbarContext = createContext<ToolbarContextValue>({
  left: null, right: null, setLeft: () => {}, setRight: () => {},
});

export function ToolbarProvider({ children }: { children: ReactNode }) {
  const [left, setLeft] = useState<ReactNode>(null);
  const [right, setRight] = useState<ReactNode>(null);
  return (
    <ToolbarContext.Provider value={{ left, right, setLeft, setRight }}>
      {children}
    </ToolbarContext.Provider>
  );
}

export function useToolbar() {
  return useContext(ToolbarContext);
}

export function Toolbar() {
  const { left, right } = useToolbar();
  if (!left && !right) return null;
  return (
    <div className="toolbar">
      <div className="flex items-stretch gap-2.5 overflow-x-auto scrollbar-none flex-1 min-w-0">
        {left}
      </div>
      {right && (
        <div className="flex items-center gap-2 shrink-0 self-center">
          {right}
        </div>
      )}
    </div>
  );
}
