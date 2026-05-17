"use client";

import { useEffect, useState } from "react";

const roleEmoji: Record<string, string> = { Lead: "👑", Engineer: "💻", Researcher: "🔬", Writer: "✍️", Analyst: "📊", DevOps: "🔧" };

const SectionHeader = ({ children }: { children: React.ReactNode }) => (
  <div style={{ fontSize: 11, fontWeight: 600, textTransform: "uppercase", letterSpacing: "0.05em", color: "rgba(255,255,255,0.4)", padding: "12px 0 6px" }}>
    {children}
  </div>
);

const ListItem = ({ icon, label, detail }: { icon: string; label: string; detail?: string }) => (
  <div style={{ display: "flex", alignItems: "center", gap: 8, padding: "4px 0", fontSize: 12, color: "rgba(255,255,255,0.7)" }}>
    <span style={{ fontSize: 13 }}>{icon}</span>
    <span style={{ flex: 1 }}>{label}</span>
    {detail && <span style={{ fontSize: 10, color: "rgba(255,255,255,0.3)" }}>{detail}</span>}
  </div>
);

/* ═══════════════════════════════════════════
   SOUL PANEL — shows when a Soul DM is active
   ═══════════════════════════════════════════ */
export function SoulPanel({ soulId, onClose }: { soulId: string; onClose?: () => void }) {
  const [soul, setSoul] = useState<any>(null);
  const [channels, setChannels] = useState<any[]>([]);
  const [budget, setBudget] = useState<any>(null);
  const [tasks, setTasks] = useState<any[]>([]);
  const [skills, setSkills] = useState<any[]>([]);

  useEffect(() => {
    if (!soulId) return;
    fetch(`/v1/agents/${soulId}`, { headers: { Authorization: "Bearer test123" } }).then(r => r.json()).then(setSoul).catch(() => {});
    fetch(`/v1/channels`, { headers: { Authorization: "Bearer test123" } }).then(r => r.json()).then(d => {
      setChannels((d.channels || []).filter((c: any) => c.agent_id === soulId));
    }).catch(() => {});
    fetch(`/v1/budgets`, { headers: { Authorization: "Bearer test123" } }).then(r => r.json()).then(d => {
      setBudget((d.budgets || []).find((b: any) => b.id === soulId));
    }).catch(() => {});
    fetch(`/v1/tasks?agent_id=${soulId}`, { headers: { Authorization: "Bearer test123" } }).then(r => r.json()).then(d => setTasks(d.tasks || [])).catch(() => {});
    fetch(`/v1/agents/${soulId}/skills`, { headers: { Authorization: "Bearer test123" } }).then(r => r.json()).then(d => setSkills(d.skills || [])).catch(() => {});
  }, [soulId]);

  if (!soul) return null;
  const emoji = roleEmoji[soul.role || ""] || "🤖";

  return (
    <div style={{ padding: 16, overflowY: "auto", height: "100%" }}>
      {/* Soul header */}
      <div style={{ textAlign: "center", paddingBottom: 12, borderBottom: "1px solid rgba(255,255,255,0.06)", position: "relative" }}>
        {onClose && <button onClick={onClose} style={{ position: "absolute", top: 0, right: 0, background: "none", border: "none", color: "var(--vs-text-muted)", cursor: "pointer", fontSize: 16 }}>✕</button>}
        <div style={{ fontSize: 32, marginBottom: 4 }}>{emoji}</div>
        <div style={{ fontSize: 14, fontWeight: 600 }}>{soul.display_name}</div>
        <div style={{ fontSize: 11, color: "rgba(255,255,255,0.4)" }}>@{soul.agent_key}</div>
        <div style={{ fontSize: 11, color: "rgba(255,255,255,0.4)", marginTop: 2 }}>{soul.role || "Soul"} · {soul.model || "default"}</div>
      </div>

      {/* Skills */}
      {skills.length > 0 && <>
        <SectionHeader>Skills ({skills.length})</SectionHeader>
        {skills.map((s: any) => (
          <ListItem key={s.slug} icon="⚡" label={s.name || s.slug} detail={s.category} />
        ))}
      </>}

      {/* Budget */}
      {budget && (
        <>
          <SectionHeader>Budget</SectionHeader>
          <div style={{ fontSize: 13, fontWeight: 600 }}>${(budget.used_cents / 100).toFixed(2)}</div>
          {budget.budget_cents > 0 && (
            <div style={{ marginTop: 4, height: 4, borderRadius: 2, background: "rgba(255,255,255,0.06)", overflow: "hidden" }}>
              <div style={{ height: "100%", borderRadius: 2, background: "#84cc16", width: `${Math.min((budget.used_cents / budget.budget_cents) * 100, 100)}%` }} />
            </div>
          )}
        </>
      )}

      {/* Channels */}
      <SectionHeader>Channels</SectionHeader>
      {channels.length === 0 && <div style={{ fontSize: 11, color: "rgba(255,255,255,0.3)" }}>No channels configured</div>}
      {channels.map((c: any) => {
        const icons: Record<string, string> = { email: "📧", telegram: "📱", slack: "💬", discord: "🎮", whatsapp: "📲" };
        return <ListItem key={c.id} icon={icons[c.channel_type] || "📨"} label={c.name || c.channel_type} detail={c.status} />;
      })}

      {/* Tasks */}
      <SectionHeader>Tasks ({tasks.length})</SectionHeader>
      {tasks.length === 0 && <div style={{ fontSize: 11, color: "rgba(255,255,255,0.3)" }}>No tasks</div>}
      {tasks.slice(0, 8).map(t => (
        <ListItem key={t.id} icon={t.status === "done" ? "☑" : "◻"} label={t.title?.replace("Delegated: ", "").slice(0, 40)} detail={t.status} />
      ))}
    </div>
  );
}

/* ═══════════════════════════════════════════
   THREAD PANEL — shows when a Thread is active
   ═══════════════════════════════════════════ */
export function ThreadPanel({ roomId }: { roomId: string }) {
  const [room, setRoom] = useState<any>(null);
  const [tasks, setTasks] = useState<any[]>([]);

  useEffect(() => {
    if (!roomId) return;
    fetch(`/v1/rooms/${roomId}`, { headers: { Authorization: "Bearer test123" } }).then(r => r.json()).then(setRoom).catch(() => {});
    fetch(`/v1/tasks`, { headers: { Authorization: "Bearer test123" } }).then(r => r.json()).then(d => setTasks(d.tasks || [])).catch(() => {});
  }, [roomId]);

  if (!room) return null;
  const members = room.members || [];
  const soulColors = ["#84cc16","#3b82f6","#22c55e","#a855f7","#f97316","#06b6d4","#ec4899","#f59e0b"];
  const getColor = (key: string) => { let h=0; for(let i=0;i<key.length;i++) h=((h<<5)-h+key.charCodeAt(i))|0; return soulColors[Math.abs(h)%soulColors.length]; };

  return (
    <div style={{ overflowY: "auto", height: "100%" }}>
      {/* Header — same height as chat header (48px) */}
      <div style={{ height: 48, minHeight: 48, display: "flex", alignItems: "center", padding: "0 16px", borderBottom: "1px solid var(--vs-border)" }}>
        <span style={{ fontSize: 14, fontWeight: 600 }}># {room.display_name || room.name}</span>
      </div>

      <div style={{ padding: "12px 16px" }}>
        {/* Members — horizontal avatars */}
        <SectionHeader>Members ({members.length})</SectionHeader>
        <div style={{ display: "flex", gap: 6, flexWrap: "wrap", padding: "4px 0" }}>
          {members.map((m: any) => (
            <div key={m.id} title={`@${m.agent_key} — ${m.role || "Soul"}`} style={{ display: "flex", flexDirection: "column", alignItems: "center", gap: 2, width: 44 }}>
              <div style={{ width: 32, height: 32, borderRadius: "50%", background: getColor(m.agent_key), display: "flex", alignItems: "center", justifyContent: "center", fontSize: 14, color: "#fff", fontWeight: 600 }}>
                {(roleEmoji[m.role || ""] || m.agent_key?.[0]?.toUpperCase() || "?")}
              </div>
              <span style={{ fontSize: 9, color: "rgba(255,255,255,0.5)", textAlign: "center", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap", width: "100%" }}>@{m.agent_key}</span>
            </div>
          ))}
        </div>

        {/* Tasks */}
        <SectionHeader>Tasks</SectionHeader>
        {tasks.length === 0 && <div style={{ fontSize: 11, color: "rgba(255,255,255,0.3)" }}>No tasks yet</div>}
        {tasks.slice(0, 8).map(t => (
          <ListItem key={t.id} icon={t.status === "done" ? "☑" : "◻"} label={t.title?.replace("Delegated: ", "").slice(0, 40)} detail={t.status} />
        ))}

        {/* Activity */}
        <SectionHeader>Activity</SectionHeader>
        <ListItem icon="💬" label={`${room.message_count || 0} messages`} />
      </div>
    </div>
  );
}
