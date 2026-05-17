"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { MessageSquare, LayoutDashboard, Activity, ListTodo, GitBranch, Radio, BarChart3, Settings, Zap } from "lucide-react";

const NAV = [
  { href: "/chat", icon: MessageSquare, label: "Chat" },
  { href: "/agents", icon: LayoutDashboard, label: "Agents" },
  { href: "/graph", icon: GitBranch, label: "Knowledge Graph" },
  { href: "/memory", icon: Activity, label: "Memory" },
  { href: "/sessions", icon: ListTodo, label: "Sessions" },
  { href: "/channels", icon: Radio, label: "Channels" },
  { href: "/settings", icon: Settings, label: "Settings" },
];

export function RailBar() {
  const pathname = usePathname();

  return (
    <>
      <div style={{ height: 48, display: "flex", alignItems: "center", justifyContent: "center", borderBottom: "1px solid var(--vs-border)" }}>
        <Zap size={20} color="var(--vs-primary)" />
      </div>
      <div style={{ flex: 1, display: "flex", flexDirection: "column", gap: 2, padding: "8px 0" }}>
        {NAV.map(item => {
          const active = pathname === item.href || (item.href === "/chat" && pathname === "/");
          return (
            <Link key={item.href} href={item.href} title={item.label} style={{
              display: "flex", alignItems: "center", justifyContent: "center",
              height: 40, margin: "0 8px", borderRadius: 6, textDecoration: "none",
              background: active ? "var(--vs-primary-muted)" : "transparent",
              color: active ? "var(--vs-primary)" : "var(--vs-text-muted)",
              transition: "all 0.15s",
            }}
            onMouseEnter={e => { if (!active) { e.currentTarget.style.background = "var(--vs-surface-hover)"; e.currentTarget.style.color = "var(--vs-text-secondary)"; }}}
            onMouseLeave={e => { if (!active) { e.currentTarget.style.background = "transparent"; e.currentTarget.style.color = "var(--vs-text-muted)"; }}}
            >
              <item.icon size={20} strokeWidth={1.5} />
            </Link>
          );
        })}
      </div>
    </>
  );
}
