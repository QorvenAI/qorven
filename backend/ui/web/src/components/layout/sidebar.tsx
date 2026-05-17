"use client";

import { useEffect, useState } from "react";
import { Avatar, UnreadBadge } from "@/components/chat/primitives";

interface Soul { id: string; agent_key: string; display_name: string; role?: string; }
interface Room { id: string; name: string; display_name: string; unread: number; }

const roleEmoji: Record<string, string> = {
  Lead: "👑", Engineer: "💻", Researcher: "🔬", Writer: "✍️",
  Designer: "🎨", Marketer: "📢", Analyst: "📊", DevOps: "🔧", Support: "🎧",
};
const soulColors = ["#84cc16","#3b82f6","#22c55e","#a855f7","#f97316","#06b6d4","#ec4899","#f59e0b"];
function getColor(key: string) { let h=0; for(let i=0;i<key.length;i++) h=((h<<5)-h+key.charCodeAt(i))|0; return soulColors[Math.abs(h)%soulColors.length]; }

interface ChatSidebarProps {
  activeSoulId?: string;
  activeRoomId?: string;
  onSelectSoul: (id: string) => void;
  onSelectRoom: (id: string) => void;
  unreadBySoul?: Record<string, number>;
  unreadByRoom?: Record<string, number>;
  lastActive?: Record<string, number>;
  soulStatuses?: Record<string, "online" | "working" | "idle">;
}

export function ChatSidebar({ activeSoulId, activeRoomId, onSelectSoul, onSelectRoom, unreadBySoul = {}, unreadByRoom = {}, lastActive = {}, soulStatuses = {} }: ChatSidebarProps) {
  const [souls, setSouls] = useState<Soul[]>([]);
  const [rooms, setRooms] = useState<Room[]>([]);
  const [channelCounts, setChannelCounts] = useState<Record<string, number>>({});
  const [search, setSearch] = useState("");

  useEffect(() => {
    fetch("/v1/agents", { headers: { Authorization: "Bearer test123" } })
      .then(r => r.json()).then(d => setSouls(d.agents || [])).catch(() => {});
    fetch("/v1/rooms", { headers: { Authorization: "Bearer test123" } })
      .then(r => r.json()).then(d => setRooms((d.rooms || []).map((r: any) => ({ id: r.id, name: r.name, display_name: r.display_name || r.name, unread: 0 })))).catch(() => {});
    fetch("/v1/channels", { headers: { Authorization: "Bearer test123" } })
      .then(r => r.json()).then(d => {
        const counts: Record<string, number> = {};
        (d.channels || []).forEach((c: any) => { counts[c.channel_type] = (counts[c.channel_type] || 0) + 1; });
        setChannelCounts(counts);
      }).catch(() => {});
  }, []);

  const getPresence = (soul: Soul): "online" | "busy" | "offline" => {
    const s = soulStatuses[soul.id];
    if (s === "working") return "busy";
    if (s === "online") return "online";
    return "online"; // default: all souls are online
  };
  const getEmoji = (soul: Soul) => roleEmoji[soul.role || ""] || "🤖";
  const primeSoul = souls.find(s => s.role === "Lead");
  const otherSouls = souls.filter(s => s.role !== "Lead");

  // Sort: unread first, then recently active, then alphabetical
  const sorted = [...otherSouls].sort((a, b) => {
    const ua = unreadBySoul[a.id] || unreadBySoul[a.agent_key] || 0;
    const ub = unreadBySoul[b.id] || unreadBySoul[b.agent_key] || 0;
    if (ua && !ub) return -1;
    if (!ua && ub) return 1;
    const la = lastActive[a.id] || lastActive[a.agent_key] || 0;
    const lb = lastActive[b.id] || lastActive[b.agent_key] || 0;
    if (la !== lb) return lb - la;
    return a.display_name.localeCompare(b.display_name);
  });

  // Filter by search
  const filtered = search
    ? sorted.filter(s => s.display_name.toLowerCase().includes(search.toLowerCase()) || s.agent_key.toLowerCase().includes(search.toLowerCase()))
    : sorted;

  // Split into active (unread or recently active) and quiet
  const now = Date.now();
  const QUIET_THRESHOLD = 7 * 24 * 60 * 60 * 1000; // 7 days
  const activeSouls = filtered.filter(s => {
    const unread = unreadBySoul[s.id] || unreadBySoul[s.agent_key] || 0;
    const la = lastActive[s.id] || lastActive[s.agent_key] || 0;
    return unread > 0 || (la > 0 && now - la < QUIET_THRESHOLD);
  });
  const quietSouls = filtered.filter(s => !activeSouls.includes(s));
  const [showQuiet, setShowQuiet] = useState(false);

  const SectionHeader = ({ children }: { children: React.ReactNode }) => (
    <div style={{ padding: "12px 20px 4px", fontSize: 11, fontWeight: 600, textTransform: "uppercase", letterSpacing: "0.05em", color: "rgba(255,255,255,0.4)" }}>
      {children}
    </div>
  );

  const SoulItem = ({ soul }: { soul: Soul }) => {
    const active = activeSoulId === soul.id && !activeRoomId;
    const unread = unreadBySoul[soul.id] || unreadBySoul[soul.agent_key] || 0;
    return (
      <div onClick={() => onSelectSoul(soul.id)} style={{
        display: "flex", alignItems: "center", gap: 10, padding: "6px 12px", cursor: "pointer",
        borderRadius: 6, margin: "1px 8px",
        background: active ? "rgba(255,255,255,0.08)" : "transparent",
        transition: "background 0.15s",
      }}
        onMouseEnter={e => { if (!active) e.currentTarget.style.background = "rgba(255,255,255,0.04)"; }}
        onMouseLeave={e => { if (!active) e.currentTarget.style.background = "transparent"; }}>
        <Avatar name={soul.display_name} emoji={getEmoji(soul)} color={getColor(soul.agent_key)} size={28} presence={getPresence(soul)} />
        <span style={{ flex: 1, fontSize: 13, color: active ? "#fff" : "rgba(255,255,255,0.72)", fontWeight: active ? 600 : 400, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
          {soul.display_name}
        </span>
        {unread > 0 && <UnreadBadge count={unread} />}
      </div>
    );
  };

  return (
    <div style={{ display: "flex", flexDirection: "column", height: "100%", overflow: "hidden" }}>
      <div style={{ padding: "10px 12px" }}>
        <input value={search} onChange={e => setSearch(e.target.value)} placeholder="Search..."
          style={{ width: "100%", boxSizing: "border-box", padding: "6px 10px", background: "rgba(255,255,255,0.06)", border: "1px solid rgba(255,255,255,0.08)", borderRadius: 4, color: "#fff", fontSize: 13, outline: "none" }} />
      </div>

      <div style={{ flex: 1, overflowY: "auto", overflowX: "hidden" }}>
        <SectionHeader>Threads</SectionHeader>
        {rooms.map(room => (
          <div key={room.id} onClick={() => onSelectRoom(room.id)} style={{
            display: "flex", alignItems: "center", gap: 8, padding: "5px 12px", cursor: "pointer",
            borderRadius: 4, margin: "0 8px",
            background: activeRoomId === room.id ? "rgba(255,255,255,0.08)" : "transparent",
            fontSize: 13, color: activeRoomId === room.id ? "#fff" : "rgba(255,255,255,0.72)",
            fontWeight: activeRoomId === room.id ? 600 : 400,
          }}
            onMouseEnter={e => { if (activeRoomId !== room.id) e.currentTarget.style.background = "rgba(255,255,255,0.04)"; }}
            onMouseLeave={e => { if (activeRoomId !== room.id) e.currentTarget.style.background = "transparent"; }}>
            <span style={{ color: "rgba(255,255,255,0.4)", fontSize: 14 }}>#</span>
            <span style={{ flex: 1 }}>{room.display_name}</span>
            {(unreadByRoom[room.id] || 0) > 0 && <UnreadBadge count={unreadByRoom[room.id]} />}
          </div>
        ))}

        <SectionHeader>Direct Messages</SectionHeader>
        {primeSoul && <SoulItem soul={primeSoul} />}
        {activeSouls.map(soul => <SoulItem key={soul.id} soul={soul} />)}

        {quietSouls.length > 0 && !search && (
          <>
            <div onClick={() => setShowQuiet(!showQuiet)} style={{ display: "flex", alignItems: "center", gap: 6, padding: "8px 20px 4px", fontSize: 11, fontWeight: 600, textTransform: "uppercase", letterSpacing: "0.05em", color: "rgba(255,255,255,0.3)", cursor: "pointer" }}>
              <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" style={{ transform: showQuiet ? "rotate(90deg)" : "rotate(0)", transition: "transform 0.15s" }}><path d="M9 18l6-6-6-6"/></svg>
              Quiet ({quietSouls.length})
            </div>
            {showQuiet && quietSouls.map(soul => <SoulItem key={soul.id} soul={soul} />)}
          </>
        )}

        {Object.keys(channelCounts).length > 0 && (
          <>
            <SectionHeader>Inbox</SectionHeader>
            {Object.entries(channelCounts).map(([type, count]) => {
              const icons: Record<string, string> = { email: "📧", telegram: "📱", slack: "💬", discord: "🎮", whatsapp: "📲", webhook: "🔗", sms: "📞" };
              return (
                <div key={type} style={{ display: "flex", alignItems: "center", gap: 8, padding: "5px 12px", margin: "0 8px", fontSize: 13, color: "rgba(255,255,255,0.72)", cursor: "pointer", borderRadius: 4 }}>
                  <span style={{ fontSize: 13 }}>{icons[type] || "📨"}</span>
                  <span style={{ flex: 1, textTransform: "capitalize" }}>{type}</span>
                  <span style={{ fontSize: 11, color: "rgba(255,255,255,0.3)" }}>{count}</span>
                </div>
              );
            })}
          </>
        )}
      </div>
    </div>
  );
}
