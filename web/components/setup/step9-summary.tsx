'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useEffect, useRef } from 'react';
import { ArrowRight, CheckCircle2, MessageCircle } from 'lucide-react';
import { prettyModel } from './setup-config';

// Lightweight canvas confetti — no external deps
function spawnConfetti(canvas: HTMLCanvasElement) {
  const ctx = canvas.getContext('2d');
  if (!ctx) return;
  canvas.width = canvas.offsetWidth;
  canvas.height = canvas.offsetHeight;

  const COLORS = ['#8b5cf6', '#a78bfa', '#c4b5fd', '#e879f9', '#f0abfc', '#ffffff'];
  const pieces = Array.from({ length: 90 }, () => ({
    x: Math.random() * canvas.width,
    y: Math.random() * -canvas.height * 0.4,
    r: Math.random() * 5 + 3,
    d: Math.random() * 2 + 1,
    color: COLORS[Math.floor(Math.random() * COLORS.length)],
    tilt: Math.random() * 10 - 5,
    tiltSpeed: Math.random() * 0.1 + 0.05,
    t: Math.random() * Math.PI * 2,
  }));

  let frame = 0;
  function draw() {
    ctx!.clearRect(0, 0, canvas.width, canvas.height);
    for (const p of pieces) {
      ctx!.beginPath();
      ctx!.fillStyle = p.color as string;
      ctx!.globalAlpha = Math.max(0, 1 - frame / 140);
      ctx!.ellipse(p.x, p.y, p.r, p.r * 0.5, p.tilt, 0, Math.PI * 2);
      ctx!.fill();
      p.y += p.d;
      p.x += Math.sin(p.t) * 0.8;
      p.t += 0.04;
      p.tilt += p.tiltSpeed;
    }
    ctx!.globalAlpha = 1;
    frame++;
    if (frame < 160) requestAnimationFrame(draw);
  }
  draw();
}

export function Step9Summary(p: {
  workspaceName: string; username: string; primeName: string; region: string;
  selectedProvider: string; primaryModel: string;
  connectedChannels: string[];
  onDone: () => void;
}) {
  const canvasRef = useRef<HTMLCanvasElement>(null);

  useEffect(() => {
    if (canvasRef.current) spawnConfetti(canvasRef.current);
  }, []);

  const hasTelegram = p.connectedChannels.includes('telegram');

  const rows = [
    { k: 'Workspace',  v: p.workspaceName || 'My Workspace' },
    { k: 'Admin',      v: p.username },
    { k: 'Assistant',  v: p.primeName },
    { k: 'Provider',   v: p.selectedProvider },
    { k: 'Model',      v: prettyModel(p.primaryModel) },
  ];

  return (
    <div className="relative space-y-6 text-center overflow-hidden">
      {/* Canvas confetti layer */}
      <canvas
        ref={canvasRef}
        className="pointer-events-none absolute inset-0 w-full h-full"
        style={{ zIndex: 0 }}
      />

      <div className="relative z-10 space-y-6">
        {/* Hero icon + text */}
        <div className="flex flex-col items-center gap-3 pt-4">
          <div className="relative flex h-20 w-20 items-center justify-center">
            {/* Glow rings */}
            <div className="absolute inset-0 rounded-full bg-gradient-to-br from-violet-500 to-fuchsia-500 opacity-20 animate-ping" style={{ animationDuration: '2s' }} />
            <div className="absolute inset-1 rounded-full bg-gradient-to-br from-violet-500 to-fuchsia-500 opacity-30 blur-sm" />
            <div className="relative flex h-16 w-16 items-center justify-center rounded-full bg-gradient-to-br from-violet-500 to-fuchsia-500 shadow-lg shadow-violet-500/40">
              <CheckCircle2 className="h-8 w-8 text-white" />
            </div>
          </div>
          <div>
            <h2 className="text-xl font-bold text-foreground">You&apos;re all set!</h2>
            <p className="text-sm text-muted-foreground mt-1">
              {p.primeName} is ready. Let&apos;s go.
            </p>
          </div>
        </div>

        {/* Summary card */}
        <div className="rounded-xl border border-border bg-muted/20 overflow-hidden text-left">
          {rows.map(({ k, v }) => (
            <div key={k} className="flex items-center justify-between px-4 py-2.5 border-b border-border/40 last:border-b-0">
              <span className="text-xs text-muted-foreground">{k}</span>
              <span className="text-xs font-medium text-foreground max-w-[55%] text-right truncate">{v || '—'}</span>
            </div>
          ))}
          {hasTelegram && (
            <div className="flex items-center gap-2 px-4 py-2.5 bg-emerald-500/5">
              <MessageCircle className="h-3.5 w-3.5 text-emerald-400 shrink-0" />
              <span className="text-xs text-emerald-400 font-medium">Telegram connected</span>
            </div>
          )}
        </div>

        {/* CTA */}
        <button
          onClick={p.onDone}
          className="w-full inline-flex items-center justify-center gap-2 rounded-xl bg-gradient-to-r from-violet-600 to-fuchsia-600 px-5 py-3 text-sm font-semibold text-white shadow-lg shadow-violet-500/30 hover:from-violet-500 hover:to-fuchsia-500 transition-all cursor-pointer"
        >
          Go to Dashboard <ArrowRight className="h-4 w-4" />
        </button>
      </div>
    </div>
  );
}
