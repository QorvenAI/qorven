'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useRef, useEffect, useState } from 'react';
import { CheckCircle2, ChevronRight, Send } from 'lucide-react';
import { cn } from '@/lib/utils';
import { apiBase as getApiBase } from '@/lib/api-url';
import { prettyModel } from './setup-config';
import { tokenHeader } from './setup-api';
import { QorvenSpinner, SectionTitle } from './setup-atoms';

export function Step8TestChat(p: {
  primeID: string | null; primeName: string; primaryModel: string; onFixProvider: () => void;
}) {
  const [input, setInput] = useState('Say hello and tell me one fun fact.');
  const [log, setLog] = useState<{ role: 'user'|'assistant'|'error'; text: string }[]>([]);
  const [streaming, setStreaming] = useState(false);
  const [ok, setOk] = useState(false);
  const abortRef = useRef<AbortController | null>(null);
  const scroller = useRef<HTMLDivElement>(null);

  useEffect(() => { scroller.current?.scrollTo({ top: 1e9 }); }, [log]);

  async function send() {
    if (!input.trim() || streaming) return;
    if (!p.primeID) { setLog(l => [...l, { role: 'error', text: 'Prime not found — go back and re-run step 4.' }]); return; }

    const userMsg = { role: 'user' as const, text: input };
    setLog(l => [...l, userMsg, { role: 'assistant', text: '' }]);
    setInput(''); setStreaming(true);

    const ctrl = new AbortController(); abortRef.current = ctrl;
    try {
      const res = await fetch(`${getApiBase()}/chat/completions`, {
        method: 'POST', signal: ctrl.signal,
        headers: { 'Content-Type': 'application/json', ...tokenHeader() },
        body: JSON.stringify({
          agent_id: p.primeID,
          messages: [{ role: 'user', content: userMsg.text }],
          stream: true,
        }),
      });
      if (!res.ok || !res.body) throw new Error(`HTTP ${res.status}`);

      const reader = res.body.getReader();
      const decoder = new TextDecoder();
      let buf = '';
      for (;;) {
        const { value, done } = await reader.read();
        if (done) break;
        buf += decoder.decode(value, { stream: true });
        let idx;
        while ((idx = buf.indexOf('\n\n')) >= 0) {
          const raw = buf.slice(0, idx).trim();
          buf = buf.slice(idx + 2);
          if (!raw.startsWith('data:')) continue;
          const payload = raw.slice(5).trim();
          if (payload === '[DONE]') break;
          try {
            const msg = JSON.parse(payload);
            if (msg.type === 'error') {
              setLog(l => {
                const copy = l.slice();
                const last = copy[copy.length - 1];
                if (last && last.role === 'assistant' && !last.text) copy.pop();
                copy.push({ role: 'error', text: msg.data?.error ?? msg.data ?? 'LLM error' });
                return copy;
              });
              continue;
            }
            const delta = msg.choices?.[0]?.delta?.content ?? msg.choices?.[0]?.message?.content;
            if (typeof delta === 'string' && delta) {
              setLog(l => {
                const copy = l.slice();
                const last = copy[copy.length - 1];
                if (last && last.role === 'assistant') last.text += delta;
                return copy;
              });
            }
          } catch { /* non-JSON frame */ }
        }
      }
      setOk(true);
    } catch (e) {
      setLog(l => [...l, { role: 'error', text: e instanceof Error ? e.message : 'stream failed' }]);
    } finally {
      setStreaming(false);
      abortRef.current = null;
    }
  }

  return (
    <div className="space-y-4">
      <SectionTitle icon={Send} title="Test chat"
        subtitle={`Talk to ${p.primeName}. This is a real LLM call against ${prettyModel(p.primaryModel)}.`} />

      <div ref={scroller} className="h-60 overflow-y-auto rounded-lg border border-border bg-muted/20 p-3 space-y-2">
        {log.length === 0 && (
          <div className="text-xs text-muted-foreground">Type a greeting below and press Enter to stream a response.</div>
        )}
        {log.map((m, i) => (
          <div key={i} className={cn('text-xs whitespace-pre-wrap',
            m.role === 'user' ? 'text-foreground' :
            m.role === 'error' ? 'text-destructive' : 'text-muted-foreground')}>
            <span className={cn('font-medium',
              m.role === 'user' ? 'text-primary' :
              m.role === 'error' ? 'text-destructive' : 'text-emerald-400')}>
              {m.role === 'user' ? 'you' : m.role === 'error' ? 'error' : p.primeName.toLowerCase()}:
            </span>{' '}{m.text || (streaming && i === log.length - 1 ? '▊' : '')}
          </div>
        ))}
      </div>

      <div className="flex gap-2">
        <input value={input} onChange={e => setInput(e.target.value)}
          onKeyDown={e => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); send(); } }}
          disabled={streaming}
          placeholder="Say hello to Prime…"
          className="qr-input" />
        <button onClick={send} disabled={streaming || !input.trim()}
          className="rounded-lg bg-primary px-4 py-2 text-xs font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-40 cursor-pointer">
          {streaming ? <QorvenSpinner className="h-3.5 w-3.5" /> : <Send className="h-3.5 w-3.5" />}
        </button>
      </div>

      {ok && (
        <div className="rounded-lg border border-emerald-500/30 bg-emerald-500/10 px-3 py-2 text-xs text-emerald-400 flex items-center gap-1.5">
          <CheckCircle2 className="h-3.5 w-3.5" /> Streaming works. You&apos;re ready to finish setup.
        </div>
      )}
      {log.some(m => m.role === 'error') && !ok && (
        <button onClick={p.onFixProvider}
          className="text-xs text-primary hover:underline flex items-center gap-1">
          <ChevronRight className="h-3 w-3" /> check provider configuration
        </button>
      )}
    </div>
  );
}
