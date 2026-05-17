"use client";

import { toast } from "sonner";

const soulColors = ["#84cc16","#3b82f6","#22c55e","#a855f7","#f97316","#06b6d4","#ec4899","#f59e0b"];
function getColor(key: string) { let h=0; for(let i=0;i<key.length;i++) h=((h<<5)-h+key.charCodeAt(i))|0; return soulColors[Math.abs(h)%soulColors.length]; }

export function soulToast({
  soulName, soulKey, highlight, action, actionLabel,
}: {
  soulName: string;
  soulKey: string;
  highlight: string;
  action?: () => void;
  actionLabel?: string;
}) {
  const color = getColor(soulKey);
  const initial = soulName.charAt(0).toUpperCase();

  toast.custom((id) => (
    <div
      onClick={() => { action?.(); toast.dismiss(id); }}
      style={{
        display: "flex", alignItems: "flex-start", gap: 12, padding: "12px 14px",
        background: "rgba(30,30,30,0.85)", backdropFilter: "blur(12px)", WebkitBackdropFilter: "blur(12px)",
        border: "1px solid rgba(255,255,255,0.08)", borderRadius: 12,
        cursor: action ? "pointer" : "default", maxWidth: 360, width: "100%",
        boxShadow: "0 8px 32px rgba(0,0,0,0.4)",
      }}
    >
      {/* Soul avatar */}
      <div style={{
        width: 32, height: 32, borderRadius: "50%", background: color, flexShrink: 0,
        display: "flex", alignItems: "center", justifyContent: "center",
        fontSize: 14, fontWeight: 700, color: "#fff",
      }}>
        {initial}
      </div>

      {/* Content */}
      <div style={{ flex: 1, minWidth: 0 }}>
        <div style={{ display: "flex", alignItems: "center", gap: 6, marginBottom: 2 }}>
          <span style={{ fontSize: 13, fontWeight: 600, color: "#fff" }}>{soulName}</span>
          <span style={{ fontSize: 11, color: "rgba(255,255,255,0.35)" }}>just now</span>
        </div>
        <div style={{
          fontSize: 12, color: "rgba(255,255,255,0.7)", lineHeight: 1.4,
          overflow: "hidden", display: "-webkit-box", WebkitLineClamp: 2, WebkitBoxOrient: "vertical" as const,
        }}>
          {highlight}
        </div>
        {action && actionLabel && (
          <div style={{ marginTop: 6, fontSize: 11, color: color, fontWeight: 500 }}>
            {actionLabel} →
          </div>
        )}
      </div>

      {/* Close */}
      <button
        onClick={(e) => { e.stopPropagation(); toast.dismiss(id); }}
        style={{
          background: "none", border: "none", color: "rgba(255,255,255,0.3)",
          cursor: "pointer", fontSize: 14, padding: 0, lineHeight: 1, flexShrink: 0,
        }}
      >
        ✕
      </button>
    </div>
  ), {
    duration: 4000,
    position: "top-right",
  });
}
