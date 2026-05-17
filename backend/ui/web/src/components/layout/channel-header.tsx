"use client";

import { Avatar } from "@/components/chat/primitives";

const roleEmoji: Record<string, string> = { Lead: "👑", Engineer: "💻", Researcher: "🔬", Writer: "✍️", Analyst: "📊", DevOps: "🔧" };
const soulColors = ["#84cc16","#3b82f6","#22c55e","#a855f7","#f97316","#06b6d4","#ec4899","#f59e0b"];
function getColor(key: string) { let h=0; for(let i=0;i<key.length;i++) h=((h<<5)-h+key.charCodeAt(i))|0; return soulColors[Math.abs(h)%soulColors.length]; }

export function ChannelHeader({ soul, room, typingSouls }: { soul?: any; room?: any; typingSouls?: string[] }) {
  if (room) {
    return (
      <>
        <span style={{ fontSize: 15, fontWeight: 600 }}># {room.display_name || room.name}</span>
        {typingSouls && typingSouls.length > 0 && (
          <>
            <div style={{ width: 1, height: 16, background: "rgba(255,255,255,0.1)", margin: "0 8px" }} />
            <span style={{ fontSize: 12, color: "var(--vs-primary)", display: "flex", alignItems: "center", gap: 4 }}>
              {typingSouls.join(", ")} {typingSouls.length === 1 ? "is" : "are"} typing
              <span className="vs-typing-dots"><span className="vs-typing-dot" /><span className="vs-typing-dot" /><span className="vs-typing-dot" /></span>
            </span>
          </>
        )}
        <div style={{ flex: 1 }} />
      </>
    );
  }

  if (soul) {
    const emoji = roleEmoji[soul.role || ""] || "🤖";
    return (
      <>
        <Avatar name={soul.display_name} emoji={emoji} color={getColor(soul.agent_key || "")} size={28} presence="online" />
        <div>
          <div style={{ fontSize: 14, fontWeight: 600 }}>{soul.display_name}</div>
        </div>
        <div style={{ width: 1, height: 20, background: "rgba(255,255,255,0.1)", margin: "0 4px" }} />
        <select
          defaultValue={soul.model || "kimi-k2.5"}
          style={{ padding: "3px 8px", borderRadius: 6, border: "1px solid rgba(255,255,255,0.08)", background: "var(--vs-surface)", color: "rgba(255,255,255,0.5)", fontSize: 11, cursor: "pointer", outline: "none" }}>
          <option value="kimi-k2.5">kimi-k2.5</option>
          <option value="deepseek-v3.2">deepseek-v3.2</option>
          <option value="qwen3-235b">qwen3-235b</option>
        </select>
        <div style={{ flex: 1 }} />
      </>
    );
  }

  return <span style={{ fontSize: 14, fontWeight: 600, color: "rgba(255,255,255,0.4)" }}>Select a conversation</span>;
}
