"use client";

import { type ReactNode, useRef, useState } from "react";
import Markdown from "react-markdown";

/* ═══════════════════════════════════════════
   TYPES
   ═══════════════════════════════════════════ */
type Reaction = { emoji: string; count: number; byMe?: boolean };

/* ═══════════════════════════════════════════
   AVATAR — CometChat style with presence dot
   ═══════════════════════════════════════════ */
export function Avatar({ name, emoji, color, size = 32, presence }: {
  name?: string; emoji?: string; color?: string; size?: number;
  presence?: "online" | "busy" | "offline";
}) {
  const initials = name ? name.split(/[\s_-]+/).map(w => w[0]).join("").slice(0, 2).toUpperCase() : "";
  return (
    <div style={{ position: "relative", width: size, height: size, flexShrink: 0 }}>
      <div style={{ width: size, height: size, borderRadius: "50%", background: color || "var(--vs-primary)", display: "flex", alignItems: "center", justifyContent: "center", fontSize: size * 0.45, color: "#fff", fontWeight: 600 }}>
        {emoji || initials || "?"}
      </div>
      {presence && presence !== "offline" && (
        <div style={{ position: "absolute", bottom: -1, right: -1, width: size * 0.38, height: size * 0.38, borderRadius: "50%", background: presence === "online" ? "var(--vs-success)" : "var(--vs-warning)", border: "2px solid var(--vs-bg)" }} />
      )}
    </div>
  );
}

/* ═══════════════════════════════════════════
   EMOJI PICKER — quick reaction bar
   ═══════════════════════════════════════════ */
const quickEmojis = ["👍", "❤️", "😂", "🎉", "🤔", "👀"];

function EmojiPicker({ onPick, onClose }: { onPick: (e: string) => void; onClose: () => void }) {
  return (
    <div style={{ display: "flex", gap: 2, padding: "4px 6px", background: "var(--vs-surface)", border: "1px solid var(--vs-border)", borderRadius: 20, boxShadow: "0 4px 16px rgba(0,0,0,0.4)" }}>
      {quickEmojis.map(e => (
        <button key={e} onClick={() => { onPick(e); onClose(); }} style={{ width: 32, height: 32, borderRadius: "50%", border: "none", cursor: "pointer", background: "transparent", fontSize: 16, display: "flex", alignItems: "center", justifyContent: "center" }}
          onMouseEnter={ev => (ev.currentTarget.style.background = "rgba(255,255,255,0.08)")}
          onMouseLeave={ev => (ev.currentTarget.style.background = "transparent")}>{e}</button>
      ))}
    </div>
  );
}

/* ═══════════════════════════════════════════
   REACTION PILLS — below bubble
   ═══════════════════════════════════════════ */
function ReactionPills({ reactions, onToggle }: { reactions: Reaction[]; onToggle?: (emoji: string) => void }) {
  if (!reactions.length) return null;
  return (
    <div style={{ display: "flex", gap: 4, flexWrap: "wrap", padding: "2px 0" }}>
      {reactions.map(r => (
        <button key={r.emoji} onClick={() => onToggle?.(r.emoji)} style={{
          display: "inline-flex", alignItems: "center", gap: 4, height: 24, padding: "0 8px", borderRadius: 20, cursor: "pointer", fontSize: 12,
          border: r.byMe ? "1px solid var(--vs-primary)" : "1px solid var(--vs-border)",
          background: r.byMe ? "rgba(104,82,214,0.15)" : "rgba(255,255,255,0.04)", color: "var(--vs-text-secondary)",
        }}>
          <span style={{ fontSize: 14 }}>{r.emoji}</span>
          <span>{r.count}</span>
        </button>
      ))}
    </div>
  );
}

/* ═══════════════════════════════════════════
   MESSAGE — CometChat style bubble
   ═══════════════════════════════════════════ */
export function Message({ id, sender, soulName, soulEmoji, soulColor, time, tokens, replyTo, threadCount, presence, isGroupChat, onReply, onForward, onFeedback, onFollowUp, onMentionClick, soulNames, children }: {
  id?: string; sender: "user" | "soul";
  soulName?: string; soulEmoji?: string; soulColor?: string;
  time?: string; tokens?: number;
  replyTo?: { name: string; text: string };
  threadCount?: number;
  presence?: "online" | "busy" | "offline";
  isGroupChat?: boolean;
  onReply?: (msg: { id: string; name: string; text: string }) => void;
  onForward?: (msgId: string) => void;
  onFeedback?: (msgId: string, type: "like" | "superlike" | "dislike") => void;
  onFollowUp?: (text: string) => void;
  onMentionClick?: (soulKey: string) => void;
  soulNames?: Record<string, string>;
  children: ReactNode;
}) {
  
  const [feedback, setFeedback] = useState<"like" | "superlike" | "dislike" | null>(null);
  const [copied, setCopied] = useState(false);

  const bubbleRef = useRef<HTMLDivElement>(null);
  const isUser = sender === "user";



  const msgText = typeof children === "string" ? children.slice(0, 100) : "message";
  const fullText = typeof children === "string" ? children : msgText;

  const doFeedback = (type: "like" | "superlike" | "dislike") => {
    const next = feedback === type ? null : type;
    setFeedback(next);
    if (next) onFeedback?.(id || "", next);
  };

  const doCopy = () => { navigator.clipboard.writeText(fullText); setCopied(true); setTimeout(() => setCopied(false), 1500); };

  const fbBtn = (type: "like" | "superlike" | "dislike", icon: ReactNode, title: string) => (
    <button onClick={() => doFeedback(type)} title={title}
      style={{ width: 22, height: 20, borderRadius: 4, border: "none", cursor: "pointer", display: "flex", alignItems: "center", justifyContent: "center", background: "transparent", color: feedback === type ? "var(--vs-primary)" : "var(--vs-text-dim)", transition: "all 0.15s" }}>
      {icon}
    </button>
  );

  return (
    <div style={{ display: "flex", gap: 8, padding: isUser ? "4px 16px" : "12px 16px", flexDirection: isUser ? "row-reverse" : "row", alignItems: "flex-start" }}>

      {/* Avatar only for user messages — Soul avatar is in the header */}
      {isUser ? null : null}

      <div style={{ maxWidth: isUser ? "70%" : "100%", display: "flex", flexDirection: "column", alignItems: isUser ? "flex-end" : "flex-start", flex: isUser ? undefined : 1 }}>
        <div ref={bubbleRef} style={{ position: "relative", width: isUser ? undefined : "100%" }}>

          {/* The bubble (user) or plain text (soul — Perplexity style) */}
          <div style={{
            borderRadius: isUser ? "18px 18px 4px 18px" : 0,
            padding: isUser ? "8px 14px 4px" : "4px 0 4px",
            background: isUser ? "var(--vs-bubble-user)" : "transparent",
            color: isUser ? "#fff" : "var(--vs-text)",
          }}>
            {/* Soul name removed from top — shown in footer for group chat */}

            {replyTo && (
              <div style={{ borderLeft: "3px solid var(--vs-primary)", marginBottom: 6, fontSize: 12, color: "var(--vs-text-muted)", borderRadius: 2, background: "rgba(255,255,255,0.04)", padding: "4px 8px" }}>
                <div style={{ fontWeight: 600, color: "var(--vs-primary)", fontSize: 11 }}>{replyTo.name}</div>
                <div style={{ overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>{replyTo.text}</div>
              </div>
            )}

            <div className={isUser ? "vs-msg-user" : "vs-msg"} style={{ fontSize: 14, lineHeight: 1.5, wordBreak: "break-word" }}>
              {typeof children === "string" ? (() => {
                const mentionify = (text: string) => text.split(/(@[\w.-]+)/g).map((part, j) =>
                  part.startsWith("@") ? (
                    <span key={j} onClick={() => onMentionClick?.(part.slice(1))}
                      style={{ color: isUser ? "#fff" : "var(--vs-primary)", fontWeight: 600, cursor: "pointer", borderBottom: isUser ? "1px dotted rgba(255,255,255,0.5)" : "1px dotted var(--vs-primary)" }}>{soulNames?.[part.slice(1)] || part}</span>
                  ) : part
                );
                const processMentions = (c: any): any => {
                  if (typeof c === "string") return c.includes("@") ? mentionify(c) : c;
                  return c;
                };
                const wrapChildren = ({ children: c }: any) => {
                  const items = Array.isArray(c) ? c : [c];
                  return items.map((child: any, i: number) => <span key={i}>{processMentions(child)}</span>);
                };
                return <Markdown components={{
                  p: (props) => <p>{wrapChildren(props)}</p>,
                  strong: (props) => <strong>{wrapChildren(props)}</strong>,
                  em: (props) => <em>{wrapChildren(props)}</em>,
                  li: (props) => <li>{wrapChildren(props)}</li>,
                }}>{children}</Markdown>;
              })() : children}
            </div>

            {/* Divider + Footer */}
            <div style={{ borderTop: isUser ? "1px solid rgba(255,255,255,0.08)" : "none", marginTop: 6, marginLeft: isUser ? -14 : 0, marginRight: isUser ? -14 : 0, paddingTop: 4, paddingLeft: isUser ? 14 : 0, paddingRight: isUser ? 14 : 0 }}>
            <div style={{ display: "flex", alignItems: "center", gap: 2, minHeight: 20 }}>
              {/* Soul name in footer for group chat */}
              {!isUser && isGroupChat && soulName && (() => {
                const key = soulName.startsWith("@") ? soulName.slice(1) : soulName;
                const displayName = soulNames?.[key] || soulName;
                return <span onClick={() => onMentionClick?.(key)}
                  style={{ fontSize: 11, fontWeight: 600, color: soulColor || "var(--vs-primary)", cursor: "pointer", marginRight: 4 }}>{displayName}</span>;
              })()}
              {/* Feedback icons — Soul messages only */}
              {!isUser && <>
                {fbBtn("like", <svg width="12" height="12" viewBox="0 0 24 24" fill={feedback === "like" ? "currentColor" : "none"} stroke="currentColor" strokeWidth="1.5"><path d="M7 10v12"/><path d="M15 5.88L14 10h5.83a2 2 0 011.92 2.56l-2.33 8A2 2 0 0117.5 22H4a2 2 0 01-2-2v-8a2 2 0 012-2h2.76a2 2 0 001.79-1.11L12 2a3.13 3.13 0 013 3.88z"/></svg>, "Like")}
                {fbBtn("superlike", <svg width="12" height="12" viewBox="0 0 24 24" fill={feedback === "superlike" ? "currentColor" : "none"} stroke="currentColor" strokeWidth="1.5"><polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2"/></svg>, "Super Like")}
                {fbBtn("dislike", <svg width="12" height="12" viewBox="0 0 24 24" fill={feedback === "dislike" ? "currentColor" : "none"} stroke="currentColor" strokeWidth="1.5"><path d="M17 14V2"/><path d="M9 18.12L10 14H4.17a2 2 0 01-1.92-2.56l2.33-8A2 2 0 016.5 2H20a2 2 0 012 2v8a2 2 0 01-2 2h-2.76a2 2 0 00-1.79 1.11L12 22a3.13 3.13 0 01-3-3.88z"/></svg>, "Dislike")}
              </>}
              {/* Copy — on all messages */}
              <button onClick={doCopy} title="Copy" style={{ width: 22, height: 20, borderRadius: 4, border: "none", cursor: "pointer", display: "flex", alignItems: "center", justifyContent: "center", background: "transparent", color: copied ? "var(--vs-success)" : isUser ? "rgba(255,255,255,0.5)" : "var(--vs-text-dim)", transition: "all 0.15s" }}>
                {copied ? <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><polyline points="20 6 9 17 4 12"/></svg> : <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5"><rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 01-2-2V4a2 2 0 012-2h9a2 2 0 012 2v1"/></svg>}
              </button>

              {/* Reply + Forward — always visible */}
              {<>
                {onReply && <button onClick={() => onReply({ id: id || "", name: isUser ? "You" : (soulName || "Soul"), text: msgText })} title="Reply"
                  style={{ width: 22, height: 20, borderRadius: 4, border: "none", cursor: "pointer", display: "flex", alignItems: "center", justifyContent: "center", background: "transparent", color: isUser ? "rgba(255,255,255,0.5)" : "var(--vs-text-dim)", transition: "color 0.15s" }}
                  onMouseEnter={e => e.currentTarget.style.color = isUser ? "#fff" : "var(--vs-text-secondary)"} onMouseLeave={e => e.currentTarget.style.color = isUser ? "rgba(255,255,255,0.5)" : "var(--vs-text-dim)"}>
                  <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5"><polyline points="9 17 4 12 9 7"/><path d="M20 18v-2a4 4 0 00-4-4H4"/></svg>
                </button>}
                {onForward && <button onClick={() => onForward(id || "")} title="Forward to Soul"
                  style={{ width: 22, height: 20, borderRadius: 4, border: "none", cursor: "pointer", display: "flex", alignItems: "center", justifyContent: "center", background: "transparent", color: isUser ? "rgba(255,255,255,0.5)" : "var(--vs-text-dim)", transition: "color 0.15s" }}
                  onMouseEnter={e => e.currentTarget.style.color = isUser ? "#fff" : "var(--vs-text-secondary)"} onMouseLeave={e => e.currentTarget.style.color = isUser ? "rgba(255,255,255,0.5)" : "var(--vs-text-dim)"}>
                  <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5"><polyline points="15 17 20 12 15 7"/><path d="M4 18v-2a4 4 0 014-4h12"/></svg>
                </button>}
              </>}

              <div style={{ flex: 1 }} />
              <div style={{ display: "flex", gap: 6, fontSize: 10, color: isUser ? "rgba(255,255,255,0.5)" : "var(--vs-text-dim)" }}>
                {time && <span>{time}</span>}
                {tokens != null && tokens > 0 && <span>~{tokens} tok</span>}
              </div>
            </div>
            </div>
          </div>
        </div>

        {threadCount != null && threadCount > 0 && (
          <button style={{ display: "flex", alignItems: "center", gap: 4, padding: "2px 0", border: "none", background: "none", cursor: "pointer", color: "var(--vs-primary)", fontSize: 12, fontWeight: 500 }}>
            <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M21 15a2 2 0 01-2 2H7l-4 4V5a2 2 0 012-2h14a2 2 0 012 2z"/></svg>
            {threadCount} {threadCount === 1 ? "reply" : "replies"}
          </button>
        )}
      </div>
    </div>
  );
}

/* ═══════════════════════════════════════════
   SYSTEM MESSAGE
   ═══════════════════════════════════════════ */
export function SystemMessage({ icon, text }: { icon?: string; text: string }) {
  return (
    <div style={{ display: "flex", justifyContent: "center", padding: "8px 16px" }}>
      <div style={{ display: "inline-flex", alignItems: "center", gap: 6, padding: "4px 14px", borderRadius: 999, background: "rgba(255,255,255,0.04)", fontSize: 11, color: "var(--vs-text-muted)" }}>
        {icon && <span>{icon}</span>}<span>{text}</span>
      </div>
    </div>
  );
}

/* ═══════════════════════════════════════════
   TYPING INDICATOR
   ═══════════════════════════════════════════ */
export function TypingIndicator({ name, emoji }: { name: string; emoji?: string }) {
  return (
    <div style={{ display: "flex", alignItems: "center", gap: 8, padding: "6px 16px", fontSize: 12, color: "var(--vs-text-muted)" }}>
      <span>{emoji || "🤖"}</span>
      <span>{name} is thinking</span>
      <span className="vs-typing-dots"><span className="vs-typing-dot" /><span className="vs-typing-dot" /><span className="vs-typing-dot" /></span>
    </div>
  );
}

/* ═══════════════════════════════════════════
   TOOL CARD
   ═══════════════════════════════════════════ */
const toolIcons: Record<string, string> = {
  web_search: "🌐", web_fetch: "🌐", exec: "💻", read_file: "📄", write_file: "✏️",
  delegate_to_soul: "👥", email_send: "📧", room_post: "💬", memory_search: "🧠",
};

export function ToolCard({ name, status, result }: {
  name: string; status: "running" | "complete" | "error"; result?: string;
}) {
  const [open, setOpen] = useState(false);
  const icon = toolIcons[name] || "🔧";
  return (
    <div style={{ border: "1px solid var(--vs-border)", borderRadius: 12, margin: "4px 0", overflow: "hidden", background: "var(--vs-surface)" }}>
      <div onClick={() => setOpen(!open)} style={{ display: "flex", alignItems: "center", gap: 8, padding: "8px 12px", fontSize: 12, cursor: "pointer" }}>
        {status === "running" ? <span style={{ animation: "spin 1s linear infinite", display: "inline-block" }}>⟳</span> : <span>{icon}</span>}
        <span style={{ flex: 1, color: "var(--vs-text-secondary)" }}>{name}</span>
        {status === "complete" && <span style={{ color: "var(--vs-success)" }}>✓</span>}
        {status === "error" && <span style={{ color: "var(--vs-error)" }}>✕</span>}
        <span style={{ fontSize: 10, transform: open ? "rotate(0)" : "rotate(-90deg)", transition: "transform 0.2s" }}>▼</span>
      </div>
      {open && result && (
        <div style={{ borderTop: "1px dashed var(--vs-border)", padding: "8px 12px" }}>
          <pre style={{ fontSize: 11, whiteSpace: "pre-wrap", wordBreak: "break-word", maxHeight: 150, overflow: "auto", margin: 0, color: "var(--vs-text-muted)" }}>{result.slice(0, 500)}</pre>
        </div>
      )}
    </div>
  );
}

/* ═══════════════════════════════════════════
   DELEGATION CARD
   ═══════════════════════════════════════════ */
export function DelegationCard({ soulKey, task, status }: { soulKey: string; task: string; status: "working" | "done" | "error" }) {
  const colors = { working: "var(--vs-warning)", done: "var(--vs-success)", error: "var(--vs-error)" };
  return (
    <div style={{ display: "flex", alignItems: "center", gap: 10, padding: "8px 14px", borderRadius: 12, border: `1px solid ${colors[status]}33`, background: `${colors[status]}08`, margin: "4px 16px", fontSize: 12 }}>
      <span>📋</span>
      <div style={{ flex: 1, minWidth: 0 }}>
        <span style={{ fontWeight: 600, color: "var(--vs-primary)" }}>@{soulKey}</span>
        <div style={{ fontSize: 11, color: "var(--vs-text-muted)", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>{task}</div>
      </div>
      <div style={{ width: 8, height: 8, borderRadius: "50%", background: colors[status] }} />
    </div>
  );
}

/* ═══════════════════════════════════════════
   UNREAD BADGE — CometChat style
   ═══════════════════════════════════════════ */
export function UnreadBadge({ count }: { count: number }) {
  if (!count) return null;
  return (
    <span style={{
      display: "inline-flex", alignItems: "center", justifyContent: "center",
      minWidth: 20, height: 20, padding: "0 6px", borderRadius: 10,
      background: "var(--vs-primary)", color: "#fff", fontSize: 11, fontWeight: 600,
    }}>
      {count > 99 ? "99+" : count}
    </span>
  );
}

/* ═══════════════════════════════════════════
   CHAT INPUT — full width, 2 rows
   ═══════════════════════════════════════════ */
export function ChatInput({ value, onChange, onSubmit, placeholder, autocomplete, acIndex, onAcSelect, onAcNav, disabled, replyTo, onCancelReply, inputRef }: {
  value: string; onChange: (v: string) => void; onSubmit: () => void;
  placeholder?: string; disabled?: boolean;
  autocomplete?: { icon: string; label: string; desc: string }[];
  acIndex?: number; onAcSelect?: (i: number) => void; onAcNav?: (dir: "up" | "down") => void;
  replyTo?: { name: string; text: string }; onCancelReply?: () => void;
  inputRef?: React.RefObject<HTMLTextAreaElement | null>;
}) {
  const [showEmoji, setShowEmoji] = useState(false);
  const acOpen = autocomplete && autocomplete.length > 0;
  return (
    <div style={{ position: "relative" }}>
      {acOpen && (
        <div style={{ position: "absolute", bottom: "100%", left: 12, width: "min(280px, 33%)", background: "var(--vs-surface)", border: "1px solid var(--vs-border)", maxHeight: 200, overflowY: "auto", zIndex: 50, borderRadius: 10, boxShadow: "0 4px 16px rgba(0,0,0,0.4)" }}>
          {autocomplete!.map((item, i) => (
            <div key={i} onClick={() => onAcSelect?.(i)} style={{
              display: "flex", alignItems: "center", gap: 8, padding: "7px 12px", fontSize: 13, cursor: "pointer",
              background: i === acIndex ? "rgba(104,82,214,0.15)" : "transparent",
            }}>
              <span>{item.icon}</span>
              <span style={{ fontWeight: 600 }}>{item.label}</span>
              <span style={{ color: "var(--vs-text-dim)", fontSize: 11 }}>{item.desc}</span>
            </div>
          ))}
        </div>
      )}

      {/* Reply preview */}
      {replyTo && (
        <div style={{ display: "flex", alignItems: "center", gap: 8, padding: "6px 16px", borderTop: "1px solid var(--vs-border)", background: "var(--vs-surface)", fontSize: 12 }}>
          <div style={{ borderLeft: "3px solid var(--vs-primary)", paddingLeft: 8, flex: 1 }}>
            <span style={{ color: "var(--vs-primary)", fontWeight: 600 }}>{replyTo.name}</span>
            <span style={{ color: "var(--vs-text-muted)", marginLeft: 8 }}>{replyTo.text.slice(0, 60)}</span>
          </div>
          <button onClick={onCancelReply} style={{ background: "none", border: "none", color: "var(--vs-text-muted)", cursor: "pointer", fontSize: 16 }}>✕</button>
        </div>
      )}

      <div style={{ borderTop: "1px solid var(--vs-border)", background: "var(--vs-surface)" }}>
        <div style={{ display: "flex", alignItems: "flex-end" }}>
          <textarea ref={inputRef} value={value} onChange={e => onChange(e.target.value)} placeholder={placeholder || "Type a message..."}
            disabled={disabled} rows={1}
            onKeyDown={e => {
              if (acOpen) {
                if (e.key === "ArrowDown") { e.preventDefault(); onAcNav?.("down"); }
                else if (e.key === "ArrowUp") { e.preventDefault(); onAcNav?.("up"); }
                else if (e.key === "Enter" || e.key === "Tab") { e.preventDefault(); onAcSelect?.(acIndex ?? 0); }
                else if (e.key === "Escape") { e.preventDefault(); onChange(value); }
                return;
              }
              if (e.key === "Enter" && !e.shiftKey) { e.preventDefault(); onSubmit(); }
            }}
            style={{ flex: 1, boxSizing: "border-box", padding: "12px 16px", background: "none", border: "none", outline: "none", color: "var(--vs-text)", fontSize: 14, resize: "none", minHeight: 44, maxHeight: 120 }} />
          <div style={{ display: "flex", alignItems: "center", gap: 4, padding: "8px 12px 8px 0" }}>
            <button title="Voice" style={{ width: 32, height: 32, borderRadius: 8, border: "none", cursor: "pointer", background: "transparent", color: "var(--vs-text-muted)", display: "flex", alignItems: "center", justifyContent: "center" }}>
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5"><path d="M12 1a3 3 0 00-3 3v8a3 3 0 006 0V4a3 3 0 00-3-3z"/><path d="M19 10v2a7 7 0 01-14 0v-2"/><line x1="12" y1="19" x2="12" y2="23"/></svg>
            </button>
            <button onClick={onSubmit} disabled={disabled || !value.trim()} title="Send" style={{
              width: 36, height: 36, borderRadius: 10, border: "none", cursor: "pointer",
              background: value.trim() ? "var(--vs-primary)" : "rgba(255,255,255,0.06)",
              color: "#fff", display: "flex", alignItems: "center", justifyContent: "center", transition: "background 0.15s",
            }}>
              <svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor"><path d="M2.01 21L23 12 2.01 3 2 10l15 2-15 2z"/></svg>
            </button>
          </div>
        </div>
        <div style={{ display: "flex", alignItems: "center", gap: 2, padding: "0 12px 6px" }}>
          <button title="Attach" style={{ height: 26, padding: "0 8px", borderRadius: 6, border: "none", cursor: "pointer", background: "transparent", color: "var(--vs-text-dim)", fontSize: 11, display: "flex", alignItems: "center", gap: 4 }}>
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5"><path d="M21.44 11.05l-9.19 9.19a6 6 0 01-8.49-8.49l9.19-9.19a4 4 0 015.66 5.66l-9.2 9.19a2 2 0 01-2.83-2.83l8.49-8.48"/></svg>
            Attach
          </button>
          <button title="Emoji" onClick={() => setShowEmoji(!showEmoji)} style={{ height: 26, padding: "0 8px", borderRadius: 6, border: "none", cursor: "pointer", background: "transparent", color: "var(--vs-text-dim)", fontSize: 11, display: "flex", alignItems: "center", gap: 4 }}>
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5"><circle cx="12" cy="12" r="10"/><path d="M8 14s1.5 2 4 2 4-2 4-2"/><line x1="9" y1="9" x2="9.01" y2="9"/><line x1="15" y1="9" x2="15.01" y2="9"/></svg>
            Emoji
          </button>
          <button title="Tools" style={{ height: 26, padding: "0 8px", borderRadius: 6, border: "none", cursor: "pointer", background: "transparent", color: "var(--vs-text-dim)", fontSize: 11, display: "flex", alignItems: "center", gap: 4 }}>
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5"><path d="M14.7 6.3a1 1 0 000 1.4l1.6 1.6a1 1 0 001.4 0l3.77-3.77a6 6 0 01-7.94 7.94l-6.91 6.91a2.12 2.12 0 01-3-3l6.91-6.91a6 6 0 017.94-7.94l-3.76 3.76z"/></svg>
            Tools
          </button>
          <div style={{ flex: 1 }} />
          <span style={{ fontSize: 10, color: "var(--vs-text-dim)" }}>Enter to send</span>
        </div>
        {/* Emoji quick picker in composer */}
        {showEmoji && (
          <div style={{ padding: "4px 12px 8px" }}>
            <EmojiPicker onPick={(e) => { onChange(value + e); setShowEmoji(false); }} onClose={() => setShowEmoji(false)} />
          </div>
        )}
      </div>
    </div>
  );
}
