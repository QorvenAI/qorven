"use client";

import { type ReactNode, useState, useRef, useCallback } from "react";

export function AppShell({
  rail, sidebar, header, children, panel,
}: {
  rail: ReactNode; sidebar: ReactNode; header?: ReactNode;
  children: ReactNode; panel?: ReactNode;
}) {
  const [sidebarOpen, setSidebarOpen] = useState(true);
  const [panelOpen, setPanelOpen] = useState(true);
  const [panelWidth, setPanelWidth] = useState(260);
  const resizing = useRef(false);

  const onMouseDown = useCallback(() => {
    resizing.current = true;
    const onMove = (e: MouseEvent) => {
      if (!resizing.current) return;
      const w = window.innerWidth - e.clientX;
      setPanelWidth(Math.max(200, Math.min(500, w)));
    };
    const onUp = () => { resizing.current = false; window.removeEventListener("mousemove", onMove); window.removeEventListener("mouseup", onUp); };
    window.addEventListener("mousemove", onMove);
    window.addEventListener("mouseup", onUp);
  }, []);

  return (
    <div style={{ display: "flex", height: "100dvh", width: "100vw", overflow: "hidden" }}>
      {/* Rail */}
      <div style={{ width: 56, minWidth: 56, height: "100dvh", background: "var(--vs-rail-bg, #12121c)", borderRight: "1px solid var(--vs-border)", display: "flex", flexDirection: "column" }}>
        {rail}
      </div>

      {/* Sidebar — collapsible */}
      {sidebarOpen && (
        <div style={{ width: 240, minWidth: 240, height: "100dvh", background: "var(--vs-sidebar-bg, #1a1a27)", borderRight: "1px solid var(--vs-border)", display: "flex", flexDirection: "column", overflow: "hidden" }}>
          {sidebar}
        </div>
      )}

      {/* Center */}
      <div style={{ flex: 1, minWidth: 0, height: "100dvh", display: "flex", flexDirection: "column", overflow: "hidden", background: "var(--vs-bg)" }}>
        {/* Header */}
        <div style={{ height: 48, minHeight: 48, borderBottom: "1px solid var(--vs-border)", display: "flex", alignItems: "center", padding: "0 12px", gap: 8 }}>
          {/* Sidebar toggle */}
          <button onClick={() => setSidebarOpen(!sidebarOpen)} title={sidebarOpen ? "Collapse sidebar" : "Expand sidebar"} style={{
            width: 28, height: 28, borderRadius: 4, border: "none", cursor: "pointer",
            background: "transparent", color: "var(--vs-text-muted)", display: "flex", alignItems: "center", justifyContent: "center",
          }}>
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5"><rect x="3" y="3" width="18" height="18" rx="2"/><path d="M9 3v18"/></svg>
          </button>
          {header}
          <div style={{ flex: 1 }} />
          {/* Panel toggle */}
          {panel && (
            <button onClick={() => setPanelOpen(!panelOpen)} title={panelOpen ? "Collapse panel" : "Expand panel"} style={{
              width: 28, height: 28, borderRadius: 4, border: "none", cursor: "pointer",
              background: "transparent", color: "var(--vs-text-muted)", display: "flex", alignItems: "center", justifyContent: "center",
            }}>
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5"><rect x="3" y="3" width="18" height="18" rx="2"/><path d="M15 3v18"/></svg>
            </button>
          )}
        </div>
        {/* Content */}
        <div style={{ flex: 1, minHeight: 0, display: "flex", flexDirection: "column", overflow: "hidden" }}>
          {children}
        </div>
      </div>

      {/* Right panel — collapsible + resizable */}
      {panel && panelOpen && (
        <>
          {/* Resize handle */}
          <div onMouseDown={onMouseDown} style={{
            width: 4, cursor: "col-resize", background: "transparent",
            transition: "background 0.15s",
          }}
          onMouseEnter={e => e.currentTarget.style.background = "var(--vs-primary)"}
          onMouseLeave={e => { if (!resizing.current) e.currentTarget.style.background = "transparent"; }}
          />
          <div style={{ width: panelWidth, minWidth: panelWidth, height: "100dvh", background: "var(--vs-panel-bg, #1a1a27)", overflow: "hidden", overflowY: "auto" }}>
            {panel}
          </div>
        </>
      )}
    </div>
  );
}
