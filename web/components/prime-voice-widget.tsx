'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState, useCallback } from 'react';
import { useRouter } from 'next/navigation';
import { useVoice } from '@/hooks/use-voice';
import { useVoiceEnabled } from '@/hooks/use-voice-enabled';
import { agents } from '@/lib/api';
import { cn } from '@/lib/utils';
import { Mic } from 'lucide-react';

function stripMd(s: string): string {
  return s.replace(/\*\*(.*?)\*\*/g, '$1').replace(/\*(.*?)\*/g, '$1')
    .replace(/`(.*?)`/g, '$1').replace(/#{1,6}\s/g, '')
    .replace(/\[([^\]]+)\]\([^)]+\)/g, '$1').replace(/\n{2,}/g, ' ').replace(/\n/g, ' ').trim();
}

const BYE_WORDS = ['bye', 'goodbye', 'good bye', 'see you', 'that\'s all', 'thank you bye', 'thanks bye'];

// PrimeVoiceWidget gates on voiceEnabled so that PrimeVoiceInner
// (which calls useVoice → useMicVAD → loads ONNX) is never mounted
// when voice is disabled. Unconditionally mounting useMicVAD on every
// page causes React error #185 when the ONNX assets are missing.
export function PrimeVoiceWidget() {
  const { enabled: voiceEnabled, loading: voiceLoading } = useVoiceEnabled();
  if (voiceLoading || !voiceEnabled) return null;
  return <PrimeVoiceInner />;
}

function PrimeVoiceInner() {
  const [primeId, setPrimeId] = useState('chief');
  const [open, setOpen] = useState(false);
  const [reply, setReply] = useState('');
  const [replyFading, setReplyFading] = useState(false);
  const router = useRouter();
  const voiceEnabled = true; // only mounted when enabled
  const voiceLoading = false;

  useEffect(() => {
    agents.chief().then((c) => { if (c?.id) setPrimeId(c.id); }).catch(() => {});
  }, []);

  useEffect(() => {
    const handler = (e: any) => router.push(`/qors/${e.detail.agentId}`);
    window.addEventListener('voice-redirect', handler);
    return () => window.removeEventListener('voice-redirect', handler);
  }, [router]);

  const { state, toggle, stop } = useVoice({
    agentId: primeId,
    onTranscript: (t) => {
      const lower = t.toLowerCase().trim().replace(/[.,!?]/g, '');
      if (BYE_WORDS.some(w => lower.includes(w))) {
        setTimeout(() => { stop(); setOpen(false); }, 2000);
      }
    },
    onResponse: (r) => {
      setReply(stripMd(r));
      setReplyFading(false);
      setTimeout(() => setReplyFading(true), 7000);
      setTimeout(() => setReply(''), 8000);
    },
    onError: (e) => {
      setReply(`⚠️ ${e}`);
      setTimeout(() => setReply(''), 5000);
    },
  });

  const active = state !== 'idle';
  const configured = !voiceLoading && voiceEnabled;

  const handleClick = useCallback(async () => {
    if (!configured) {
      router.push('/settings?tab=voice');
      return;
    }
    if (!open) setOpen(true);
    await toggle();
  }, [configured, open, toggle, router]);

  return (
    <div className="fixed bottom-5 right-5 z-50 flex flex-col items-end">
      {/* Reply bubble */}
      {reply && (
        <div className={cn(
          'max-w-72 mb-4 rounded-2xl rounded-br-sm bg-card border border-border px-4 py-3 text-sm text-foreground shadow-lg',
          'animate-in fade-in slide-in-from-bottom-2 duration-200 transition-opacity',
          replyFading && 'opacity-0',
        )}>
          {reply}
        </div>
      )}

      <div className="h-2" />

      {/* Floating button — always visible; disabled appearance when not configured */}
      <div className="relative group/voice">
        <button
          onClick={handleClick}
          className={cn(
            'h-14 w-14 rounded-full shadow-lg transition-all relative flex items-center justify-center',
            configured
              ? cn('cursor-pointer', active ? 'bg-primary' : 'bg-card border border-border hover:shadow-xl')
              : 'cursor-pointer bg-card border border-border/50 opacity-50 hover:opacity-70',
          )}
        >
          {configured && active && (
            <>
              <span className="absolute inset-0 rounded-full bg-primary/20 animate-ping" />
              <span className="absolute inset-[-4px] rounded-full border-2 border-primary/40 animate-pulse" />
            </>
          )}
          {configured && state === 'listening' ? (
            <svg className="h-6 w-6 text-white relative z-10" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
              <path d="M12 2a3 3 0 0 0-3 3v7a3 3 0 0 0 6 0V5a3 3 0 0 0-3-3Z" className="animate-pulse" />
              <path d="M19 10v2a7 7 0 0 1-14 0v-2" /><line x1="12" x2="12" y1="19" y2="22" />
            </svg>
          ) : configured && state === 'processing' ? (
            <svg className="h-6 w-6 text-white animate-spin relative z-10" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
              <circle cx="12" cy="12" r="10" strokeDasharray="60" strokeDashoffset="20" />
              <path d="M12 6v6l4 2" />
            </svg>
          ) : configured && state === 'speaking' ? (
            <svg className="h-6 w-6 text-white relative z-10" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
              <path d="M2 10v3" className="animate-pulse" style={{animationDelay:'0ms'}} />
              <path d="M6 6v11" className="animate-pulse" style={{animationDelay:'100ms'}} />
              <path d="M10 3v18" className="animate-pulse" style={{animationDelay:'200ms'}} />
              <path d="M14 8v7" className="animate-pulse" style={{animationDelay:'300ms'}} />
              <path d="M18 5v13" className="animate-pulse" style={{animationDelay:'400ms'}} />
              <path d="M22 10v3" className="animate-pulse" style={{animationDelay:'500ms'}} />
            </svg>
          ) : configured ? (
            <img src="/logo/qorven-mark.svg" alt="Prime" className="h-8 w-8 opacity-80 relative z-10" />
          ) : (
            <Mic className="h-6 w-6 text-muted-foreground relative z-10" />
          )}
        </button>

        {/* Tooltip — only shown when not configured */}
        {!configured && !voiceLoading && (
          <div className="absolute bottom-full right-0 mb-2 w-max max-w-[200px] rounded-lg border border-border bg-popover px-3 py-2 text-xs text-popover-foreground shadow-md opacity-0 group-hover/voice:opacity-100 transition-opacity pointer-events-none">
            Voice not set up — click to configure
            <div className="absolute bottom-[-5px] right-5 h-2.5 w-2.5 rotate-45 border-b border-r border-border bg-popover" />
          </div>
        )}
      </div>
    </div>
  );
}
