"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useChat } from "@ai-sdk/react";
import { DefaultChatTransport } from "ai";
import { AppShell } from "@/components/layout/app-shell";
import { RailBar } from "@/components/layout/rail-bar";
import { ChatSidebar } from "@/components/layout/sidebar";
import { ChannelHeader } from "@/components/layout/channel-header";
import { SoulPanel, ThreadPanel } from "@/components/layout/right-panel";
import { Message, SystemMessage, TypingIndicator, ToolCard, DelegationCard, ChatInput } from "@/components/chat/primitives";
import { useRealtime } from "@/lib/use-realtime";
import { soulToast } from "@/components/soul-toast";

const roleEmoji: Record<string, string> = { Lead: "👑", Engineer: "💻", Researcher: "🔬", Writer: "✍️", Analyst: "📊", DevOps: "🔧" };
const soulColors = ["#84cc16","#3b82f6","#22c55e","#a855f7","#f97316","#06b6d4","#ec4899","#f59e0b"];
function getColor(key: string) { let h=0; for(let i=0;i<key.length;i++) h=((h<<5)-h+key.charCodeAt(i))|0; return soulColors[Math.abs(h)%soulColors.length]; }

function relativeTime(ts: number | undefined) {
  if (!ts) return "";
  const now = Date.now();
  const t = ts < 1e12 ? ts * 1000 : ts;
  const diff = Math.floor((now - t) / 60000);
  if (diff < 1) return "just now";
  if (diff < 60) return `${diff}m ago`;
  return new Date(t).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
}

function detectType(text: string) {
  if (text.match(/📋 Task assigned to @([\w.-]+)/)) return { type: "delegation" as const, soulKey: text.match(/@([\w.-]+)/)?.[1] || "", task: text.match(/Task: (.+?)(?:\n|$)/s)?.[1] || "" };
  if (text.match(/✅ @([\w.-]+) completed:/)) return { type: "result" as const };
  if (text.startsWith("[") && text.endsWith("]")) return { type: "system" as const };
  return { type: "text" as const };
}

function parseToolMarkers(text: string) {
  const regex = /---tool:([^:]+):(\w+)(?::(.+?))?---/g;
  const segs: { type: "text" | "tool"; content?: string; name?: string; status?: string; result?: string }[] = [];
  let last = 0, m;
  while ((m = regex.exec(text)) !== null) {
    if (m.index > last) { const t = text.slice(last, m.index).trim(); if (t) segs.push({ type: "text", content: t }); }
    segs.push({ type: "tool", name: m[1], status: m[2], result: m[3] });
    last = m.index + m[0].length;
  }
  if (last < text.length) { const t = text.slice(last).trim(); if (t) segs.push({ type: "text", content: t }); }
  return segs.length ? segs : [{ type: "text" as const, content: text }];
}


export default function ChatPage() {
  const [activeSoulId, setActiveSoulId] = useState("");
  const [activeRoomId, setActiveRoomId] = useState<string | null>(null);
  const [soulInfo, setSoulInfo] = useState<any>(null);
  const [roomInfo, setRoomInfo] = useState<any>(null);
  const [mentionPanelId, setMentionPanelId] = useState<string | null>(null);
  const [text, setText] = useState("");
  const [replyTo, setReplyTo] = useState<{ id: string; name: string; text: string } | null>(null);
  const inputRef = useRef<HTMLTextAreaElement>(null);

  const handleReply = (msg: { id: string; name: string; text: string }) => {
    setReplyTo(msg);
    setTimeout(() => inputRef.current?.focus(), 50);
  };
  const [loading, setLoading] = useState(true);
  const [reloadKey, setReloadKey] = useState(0);
  const [soulStatuses, setSoulStatuses] = useState<Record<string, "online" | "working" | "idle">>({});
  const [roomMessages, setRoomMessages] = useState<any[]>([]);
  const [streamingMsgs, setStreamingMsgs] = useState<Record<string, { soulKey: string; soulName: string; text: string }>>({});
  const [typingSouls, setTypingSouls] = useState<string[]>([]);
  const [unreadSouls, setUnreadSouls] = useState<Record<string, number>>({});
  const [unreadRooms, setUnreadRooms] = useState<Record<string, number>>({});
  const [lastActive, setLastActive] = useState<Record<string, number>>({});
  const scrollRef = useRef<HTMLDivElement>(null);

  // Autocomplete — different lists for DM vs Thread
  const [soulsList, setSoulsList] = useState<{ icon: string; label: string; desc: string; value: string; id: string }[]>([]);
  const [threadMembers, setThreadMembers] = useState<typeof soulsList>([]);
  const [autocomplete, setAutocomplete] = useState<typeof soulsList>([]);
  const [acIndex, setAcIndex] = useState(0);

  useEffect(() => {
    fetch("/v1/agents", { headers: { Authorization: "Bearer test123" } }).then(r => r.json()).then(d => {
      const agents = d.agents || [];
      setSoulsList(agents.map((a: any) => ({ icon: roleEmoji[a.role || ""] || "🤖", label: `@${a.agent_key}`, desc: a.display_name, value: `@${a.agent_key} `, id: a.id })));
      if (!activeSoulId && agents.length) { setActiveSoulId(agents[0].id); setSoulInfo(agents[0]); }
    }).catch(() => {});
  }, []);

  // Load thread members when room changes
  useEffect(() => {
    if (!activeRoomId) { setThreadMembers([]); return; }
    fetch(`/v1/rooms/${activeRoomId}`, { headers: { Authorization: "Bearer test123" } })
      .then(r => r.json()).then(d => {
        const members = d.members || [];
        setThreadMembers(members.map((m: any) => ({ icon: roleEmoji[m.role || ""] || "🤖", label: `@${m.agent_key}`, desc: m.display_name, value: `@${m.agent_key} `, id: m.id })));
      }).catch(() => {});
  }, [activeRoomId]);

  // Autocomplete — only compute when @ is typed, not every keystroke
  const acRef = useRef<typeof soulsList>([]);
  const updateAutocomplete = useCallback((val: string) => {
    const last = val.split(/\s/).pop() || "";
    if (!last.startsWith("@")) {
      if (acRef.current.length) { acRef.current = []; setAutocomplete([]); }
      return;
    }
    const pool = activeRoomId ? threadMembers : soulsList.filter(s => s.id !== activeSoulId);
    const filtered = last === "@" ? pool : pool.filter(s => s.label.toLowerCase().includes(last.toLowerCase()));
    acRef.current = filtered;
    setAutocomplete(filtered);
    setAcIndex(0);
  }, [activeRoomId, threadMembers, soulsList, activeSoulId]);

  const acNav = (dir: "up" | "down") => { setAcIndex(i => { const len = autocomplete.length; return dir === "down" ? (i + 1) % len : (i - 1 + len) % len; }); };
  // Map @handle → display name for mention rendering
  const soulNames = useMemo(() => {
    const map: Record<string, string> = {};
    soulsList.forEach(s => { map[s.label.slice(1)] = s.desc; });
    return map;
  }, [soulsList]);

  const applyAc = (i: number) => { const item = autocomplete[i]; if (!item) return; const w = text.split(/\s/); w.pop(); setText(w.join(" ") + (w.length ? " " : "") + item.value); setAutocomplete([]); };

  useEffect(() => {
    if (!activeSoulId || activeRoomId) return;
    fetch(`/v1/agents/${activeSoulId}`, { headers: { Authorization: "Bearer test123" } }).then(r => r.json()).then(d => { if (d.id) setSoulInfo(d); }).catch(() => {});
  }, [activeSoulId, activeRoomId]);

  useEffect(() => {
    if (!activeRoomId) return;
    fetch(`/v1/rooms/${activeRoomId}`, { headers: { Authorization: "Bearer test123" } }).then(r => r.json()).then(setRoomInfo).catch(() => {});
    loadRoomMessages();
  }, [activeRoomId]);

  const loadRoomMessages = () => {
    if (!activeRoomId) return;
    fetch(`/v1/rooms/${activeRoomId}/messages`, { headers: { Authorization: "Bearer test123" } })
      .then(r => r.json()).then(d => setRoomMessages((d.messages || []).reverse())).catch(() => {});
  };

  useRealtime({ onMessage: (e) => {
    // Notifications + unread counts
    const nd = e.data as any;

    // DM notifications
    if (e.type === "new_message" && nd?.role === "assistant") {
      const aid = nd?.agent_id || "";
      // If this is for the currently active Soul, reload messages
      if (aid && aid === activeSoulId && !activeRoomId) {
        setReloadKey(k => k + 1);
      }
      if (aid && aid !== activeSoulId) {
        setUnreadSouls(p => ({ ...p, [aid]: (p[aid] || 0) + 1 }));
      }
      const dmName = soulNames[nd?.soul_key] || nd?.soul_name || "Soul";
        const dmHighlight = nd?.highlight || (nd?.content || "").slice(0, 120);
        const dmAgentId = nd?.agent_id || "";
        soulToast({
          soulName: dmName, soulKey: nd?.soul_key || "",
          highlight: dmHighlight,
          action: () => { if (dmAgentId) { setActiveSoulId(dmAgentId); setActiveRoomId(null); setReloadKey(k => k + 1); setUnreadSouls(p => { const n = {...p}; delete n[dmAgentId]; return n; }); } },
          actionLabel: "View DM",
        });
        if (nd?.agent_id) setLastActive(p => ({ ...p, [nd.agent_id]: Date.now() }));
    }

    // Room: stream finished — notify + badge
    if (e.type === "stream_end" && nd?.room_id) {
      const soulName = soulNames[nd.soul_key] || nd.soul_name || nd.soul_key || "Soul";
      if (nd.room_id !== activeRoomId) {
        setUnreadRooms(p => ({ ...p, [nd.room_id]: (p[nd.room_id] || 0) + 1 }));
      }
      const highlight = nd?.highlight || "New response in thread";
      soulToast({
        soulName, soulKey: nd?.soul_key || "",
        highlight,
        action: activeRoomId !== nd.room_id ? () => { setActiveRoomId(nd.room_id); setActiveSoulId(""); setUnreadRooms(p => { const n = {...p}; delete n[nd.room_id]; return n; }); } : undefined,
        actionLabel: activeRoomId !== nd.room_id ? "View Thread" : undefined,
      });
      if (nd?.soul_key) setLastActive(p => ({ ...p, [nd.soul_key]: Date.now() }));
    }

    // Room: final message saved — badge only (no duplicate toast)
    if (e.type === "room_message" && nd?.room_id && nd?.sender !== "user") {
      if (nd.room_id !== activeRoomId) {
        setUnreadRooms(p => ({ ...p, [nd.room_id]: (p[nd.room_id] || 0) + 1 }));
      }
    }
    // Notification doorbell
    if (e.type === "notification") {
      const n2 = e.data as any;
      soulToast({
        soulName: n2?.agent_name || n2?.agent_key || "Soul",
        soulKey: n2?.agent_key || "",
        highlight: n2?.highlight || "New notification",
        action: n2?.source === "dm" ? () => { if (n2?.agent_id) { setActiveSoulId(n2.agent_id); setActiveRoomId(null); setReloadKey(k => k + 1); } } : undefined,
        actionLabel: n2?.source === "dm" ? "View DM" : undefined,
      });
    }
    if (e.type === "soul_activity") {
      const d = e.data as any;
      setSoulStatuses(p => ({ ...p, [d.soul_key]: d.status === "working" ? "working" : "online" }));
    }
    if (e.type === "room_message" && (e.data as any)?.room_id === activeRoomId) {
      loadRoomMessages();
      setTypingSouls([]);
    }
    // Stream events — live token rendering
    const sd = e.data as any;
    if (e.type === "stream_start" && sd?.room_id === activeRoomId) {
      setStreamingMsgs(prev => ({ ...prev, [sd.msg_id]: { soulKey: sd.soul_key, soulName: sd.soul_name || sd.soul_key, text: "" } }));
      setTypingSouls(prev => prev.includes(sd.soul_name || sd.soul_key) ? prev : [...prev, sd.soul_name || sd.soul_key]);
    }
    if (e.type === "stream_delta" && sd?.room_id === activeRoomId) {
      setStreamingMsgs(prev => {
        const msg = prev[sd.msg_id];
        if (!msg) return prev;
        return { ...prev, [sd.msg_id]: { ...msg, text: msg.text + sd.delta } };
      });
    }
    if (e.type === "stream_end" && sd?.room_id === activeRoomId) {
      setStreamingMsgs(prev => { const next = { ...prev }; delete next[sd.msg_id]; return next; });
      setTypingSouls(prev => prev.filter(n => n !== (sd.soul_name || sd.soul_key)));
      loadRoomMessages(); // load final persisted message
    }
    if (e.type === "soul_completed") setReloadKey(k => k + 1);
  }});

  const { messages, sendMessage, status, setMessages } = useChat({
    transport: new DefaultChatTransport({ api: "/api/chat", body: { model: "kimi-k2.5", agent_id: activeSoulId, session_id: "" } }),
  });

  useEffect(() => {
    if (!activeSoulId || activeRoomId) return;
    setLoading(true);
    fetch(`/api/sessions?agentId=${activeSoulId}`).then(r => r.json()).then(d => {
      const list = d.sessions || [];
      if (list.length > 0) {
        fetch("/api/sessions", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ action: "get", id: list[0].id }) })
          .then(r => r.json()).then(session => {
            if (session?.messages?.length > 0) {
              setMessages(session.messages.slice(-30).map((m: any, i: number) => ({
                id: `msg-${i}`, role: m.role, parts: [{ type: "text" as const, text: m.content || "" }],
                createdAt: m.timestamp ? new Date(m.timestamp * 1000) : new Date(),
              })));
            } else setMessages([]);
            setLoading(false);
          }).catch(() => { setMessages([]); setLoading(false); });
      } else { setMessages([]); setLoading(false); }
    }).catch(() => { setMessages([]); setLoading(false); });
  }, [activeSoulId, setMessages, reloadKey, activeRoomId]);

  useEffect(() => { scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight, behavior: "smooth" }); }, [messages, roomMessages]);

  const handleSubmit = async () => {
    if (!text.trim()) return;
    // Prepend reply context so the Soul knows what's being referenced
    let msg = text;
    if (replyTo) {
      msg = `> Replying to ${replyTo.name}: "${replyTo.text}"\n\n${text}`;
    }
    setText(""); setReplyTo(null);
    if (activeRoomId) {
      await fetch(`/v1/rooms/${activeRoomId}/messages`, { method: "POST", headers: { "Content-Type": "application/json", Authorization: "Bearer test123" }, body: JSON.stringify({ content: msg, reply_to: replyTo?.id }) });
      loadRoomMessages();
      // Streaming events handle the rest — no polling needed
    } else {
      await sendMessage({ text: msg });
    }
  };

  // Toggle reaction on a message

  // Forward: open @autocomplete with last 10 messages as context
  const [forwardMsgId, setForwardMsgId] = useState<string | null>(null);
  const handleForward = (msgId: string) => {
    setForwardMsgId(msgId);
    // Collect last 10 messages as context
    const recent = messages.slice(-10).map(m => {
      const t = getMessageText(m.parts || []);
      return `[${m.role}] ${t.slice(0, 200)}`;
    }).join("\n");
    setText("@");  // trigger autocomplete — user picks a Soul, then we send context
    // Store context for when they select
    (window as any).__forwardCtx = recent;
  };

  // Feedback: like/superlike/dislike → POST to backend for learning
  const handleFeedback = (msgId: string, type: "like" | "superlike" | "dislike") => {
    fetch("/v1/feedback", {
      method: "POST",
      headers: { "Content-Type": "application/json", Authorization: "Bearer test123" },
      body: JSON.stringify({ message_id: msgId, agent_id: activeSoulId, rating: type }),
    }).catch(() => {});
  };

  // Click @mention → show Soul profile in right panel
  const handleMentionClick = (soulKey: string) => {
    const soul = soulsList.find(s => s.label === `@${soulKey}`);
    if (soul) {
      setMentionPanelId(soul.id);
    }
  };

  const getMessageText = (parts: any[]) => parts?.filter(p => p.type === "text").map(p => p.text).join(" ") || "";
  const soulEmoji = roleEmoji[soulInfo?.role || ""] || "🤖";
  const soulColor = soulInfo ? getColor(soulInfo.agent_key || "") : "#84cc16";
  const soulPresence = soulStatuses[activeSoulId] === "working" ? "busy" as const : "online" as const;

  const renderedMessages = useMemo(() => {
    if (activeRoomId) {
      return roomMessages.map(msg => {
        const id = msg.id;
        const isUser = msg.sender_type === "user" || msg.sender_id === "user";
        return (
          <Message key={id} id={id} sender={isUser ? "user" : "soul"} isGroupChat={true}
            soulName={!isUser ? `@${msg.sender_id}` : undefined}
            soulEmoji={!isUser ? (roleEmoji[msg.role || ""] || "🤖") : undefined}
            soulColor={!isUser ? getColor(msg.sender_id) : undefined}
            presence={!isUser ? "online" : undefined}
            time={new Date(msg.created_at).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}
            tokens={!isUser ? Math.round(msg.content.length / 4) : undefined}
            onReply={handleReply} onForward={handleForward} onFeedback={handleFeedback}
            onFollowUp={(t) => setText(t)} onMentionClick={handleMentionClick} soulNames={soulNames}>
            {msg.content}
          </Message>
        );
      });
    }

    return messages.map((msg) => {
      const fullText = getMessageText(msg.parts || []);
      const detected = detectType(fullText);
      if (msg.role === "system" || detected.type === "system") return <SystemMessage key={msg.id} icon="📨" text={fullText.replace(/^\[|\]$/g, "").slice(0, 60)} />;
      if (detected.type === "delegation" && msg.role === "assistant") return <DelegationCard key={msg.id} soulKey={detected.soulKey!} task={detected.task!} status="working" />;

      if (msg.role === "user") {
        return (
          <Message key={msg.id} id={msg.id} sender="user" isGroupChat={false}
            time={relativeTime((msg as any).createdAt?.getTime?.())}
            onReply={handleReply} onForward={handleForward} onFeedback={handleFeedback} onFollowUp={(t) => setText(t)} onMentionClick={handleMentionClick} soulNames={soulNames}>
            {fullText}
          </Message>
        );
      }

      const segments = parseToolMarkers(fullText);
      const textParts = segments.filter(s => s.type === "text").map(s => s.content).join("\n");
      const toolParts = segments.filter(s => s.type === "tool");
      return (
        <div key={msg.id}>
          {toolParts.length > 0 && (
            <div style={{ padding: "2px 56px" }}>
              {toolParts.map((seg, j) => <ToolCard key={j} name={seg.name!} status={seg.status === "running" ? "running" : "complete"} result={seg.result} />)}
            </div>
          )}
          {textParts.trim() && (
            <Message id={msg.id} sender="soul" isGroupChat={false}
              soulName={soulInfo?.display_name} soulEmoji={soulEmoji} soulColor={soulColor}
              presence={soulPresence}
              time={relativeTime((msg as any).createdAt?.getTime?.())}
              tokens={Math.round(textParts.length / 4)}
                onReply={handleReply} onForward={handleForward} onFeedback={handleFeedback} onFollowUp={(t) => setText(t)} onMentionClick={handleMentionClick} soulNames={soulNames}>
              {textParts}
            </Message>
          )}
        </div>
      );
    });
  }, [messages, roomMessages, activeRoomId, soulInfo, soulEmoji, soulColor, soulPresence, soulNames]);

  return (
    <AppShell
      rail={<RailBar />}
      sidebar={
        <ChatSidebar
          activeSoulId={activeSoulId}
          activeRoomId={activeRoomId || undefined}
          onSelectSoul={(id) => { setActiveSoulId(id); setActiveRoomId(null); setReloadKey(k => k + 1); setUnreadSouls(p => { const n = {...p}; delete n[id]; return n; }); }}
          onSelectRoom={(id) => { setActiveRoomId(id); setActiveSoulId(""); setUnreadRooms(p => { const n = {...p}; delete n[id]; return n; }); }}
          soulStatuses={soulStatuses}
          unreadBySoul={unreadSouls}
          unreadByRoom={unreadRooms}
          lastActive={lastActive}
        />
      }
      header={<ChannelHeader soul={!activeRoomId ? soulInfo : undefined} room={activeRoomId ? roomInfo : undefined} typingSouls={typingSouls} />}
      panel={mentionPanelId ? <SoulPanel soulId={mentionPanelId} onClose={() => setMentionPanelId(null)} /> : activeRoomId ? <ThreadPanel roomId={activeRoomId} /> : activeSoulId ? <SoulPanel soulId={activeSoulId} /> : undefined}
    >
      <div ref={scrollRef} style={{ flex: 1, overflowY: "auto", overflowX: "hidden", minHeight: 0 }}>
        <div style={{ maxWidth: 720, margin: "0 auto", padding: "16px 0" }}>
          {loading && !activeRoomId ? (
            <div style={{ display: "flex", alignItems: "center", justifyContent: "center", height: "50vh" }}>
              <TypingIndicator name="Loading" emoji="⏳" />
            </div>
          ) : (messages.length === 0 && roomMessages.length === 0) ? (
            <div style={{ display: "flex", flexDirection: "column", alignItems: "center", justifyContent: "center", height: "50vh", gap: 8 }}>
              <span style={{ fontSize: 40 }}>{activeRoomId ? "#" : soulEmoji}</span>
              <span style={{ fontSize: 16, fontWeight: 600 }}>{activeRoomId ? roomInfo?.display_name : soulInfo?.display_name || "Qorven"}</span>
              <span style={{ fontSize: 13, color: "rgba(255,255,255,0.4)" }}>Start a conversation</span>
            </div>
          ) : <>{renderedMessages}</>}
          {status === "submitted" && !activeRoomId && <TypingIndicator name={soulInfo?.display_name || "Qorven"} emoji={soulEmoji} />}
        </div>
      </div>

      <ChatInput value={text} replyTo={replyTo ? { name: replyTo.name, text: replyTo.text } : undefined} onCancelReply={() => setReplyTo(null)}
        onChange={(v) => { setText(v); updateAutocomplete(v); }} onSubmit={handleSubmit}
        placeholder={activeRoomId ? `Message #${roomInfo?.name || "thread"}...` : `Message ${soulInfo?.display_name || "Qorven"}...`}
        disabled={status === "streaming" || status === "submitted"}
        autocomplete={autocomplete} acIndex={acIndex} onAcSelect={applyAc} onAcNav={acNav} inputRef={inputRef} />
    </AppShell>
  );
}
