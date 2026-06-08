'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState } from 'react';
import { useRouter } from 'next/navigation';
import { cn } from '@/lib/utils';
import { isAuthenticated } from '@/lib/api';
import { Check } from 'lucide-react';
import { api } from '@/components/setup/setup-api';
import { QorvenSpinner } from '@/components/setup/setup-atoms';
import { ChatWizard } from '@/components/setup/chat-wizard';

export default function SetupPage() {
  const router = useRouter();
  const [bootstrapping, setBootstrapping] = useState(true);
  const [appVersion,    setAppVersion]    = useState('');
  const [phase,         setPhase]         = useState(0);

  useEffect(() => {
    (async () => {
      try {
        const h = await fetch('/api/health/detailed').then(r => r.json()).catch(() => ({}));
        if (h?.version) setAppVersion(String(h.version));

        const sc = await api<{ setup_required: boolean }>('/auth/setup-check');
        if (!sc.setup_required) {
          // Setup is done on the backend. Only redirect away if the wizard
          // was explicitly completed (qorven_setup_done cookie is set).
          // Without it, the user refreshed mid-wizard — stay on setup.
          const setupDone = document.cookie.includes('qorven_setup_done=1');
          if (setupDone && isAuthenticated()) { router.replace('/'); return; }
          if (setupDone) { router.replace('/login'); return; }
          // Mid-wizard refresh: account exists but wizard not finished.
          // Stay on setup page — the wizard will resume from the current step.
        }
      } catch { /* backend not ready — proceed to wizard */ }
      finally { setBootstrapping(false); }
    })();
  }, []); // eslint-disable-line

  if (bootstrapping) {
    return (
      <div className="h-screen w-full bg-background flex items-center justify-center gap-4 flex-col">
        <QorvenSpinner className="h-10 w-10 opacity-70" />
        <p className="text-sm text-muted-foreground">Starting up…</p>
      </div>
    );
  }

  return (
    <div className="h-screen w-full bg-background flex overflow-hidden">

      {/* ── Sidebar ──────────────────────────────────────────────────────── */}
      <div className="hidden lg:flex w-[300px] shrink-0 flex-col bg-gradient-to-br from-violet-950 via-violet-900 to-fuchsia-900 px-9 py-10 relative overflow-hidden">
        <div className="absolute -top-16 -right-16 w-72 h-72 rounded-full bg-fuchsia-500/20 blur-3xl pointer-events-none" />
        <div className="absolute bottom-0 left-0 w-48 h-48 rounded-full bg-violet-400/10 blur-3xl pointer-events-none" />
        <div className="relative z-10 flex flex-col h-full">
          <div className="mb-12">
            <img src="/logo/qorven-wordmark-white.svg" alt="Qorven" className="h-9" />
          </div>
          <div className="flex-1">
            <p className="text-xs font-semibold uppercase tracking-widest text-white/50 mb-5">Setup phases</p>
            <div className="space-y-4">
              {[
                { n: 0, label: 'Disclaimer'  },
                { n: 1, label: 'Account'     },
                { n: 2, label: 'Workspace'   },
                { n: 3, label: 'AI Provider' },
                { n: 4, label: 'Channels'    },
              ].map(({ n, label }) => {
                const done   = phase > n;
                const active = phase === n;
                return (
                  <div key={n} className="flex items-center gap-3.5">
                    <div className={cn(
                      'flex h-6 w-6 shrink-0 items-center justify-center rounded-full border text-xs font-semibold transition-all duration-200',
                      done   ? 'border-white/70 bg-white/20 text-white'  :
                      active ? 'border-white     bg-white/15 text-white'  :
                               'border-white/40  text-white/60',
                    )}>
                      {done ? <Check className="h-3 w-3" /> : n + 1}
                    </div>
                    <span className={cn(
                      'text-sm font-medium transition-colors duration-200',
                      done   ? 'text-white/60 line-through decoration-white/30' :
                      active ? 'text-white' : 'text-white/75',
                    )}>{label}</span>
                  </div>
                );
              })}
            </div>
          </div>
          <div className="space-y-1">
            <p>
              <a href="https://qorven.ai" target="_blank" rel="noopener noreferrer"
                className="text-sm font-medium text-white/60 hover:text-white/90 transition-colors">
                qorven.ai
              </a>
            </p>
            {appVersion && <p className="text-xs text-white/50">Version {appVersion}</p>}
            <p className="text-xs text-white/40">&copy; 2026 Qorven AI</p>
          </div>
        </div>
      </div>

      {/* ── Right panel — 3 bands ──────────────────────────────────────── */}
      <div className="grow min-w-0 h-screen overflow-hidden flex flex-col">

        {/* Band 1: Header */}
        <div className="shrink-0 border-b border-border px-8 py-4 flex items-center gap-4">
          <div className="flex-1 min-w-0">
            <h1 className="text-base font-semibold text-foreground">Setting up Qorven</h1>
            <p className="text-xs text-muted-foreground mt-0.5">
              Follow Prime&apos;s questions to configure your workspace.
            </p>
          </div>
          {appVersion && (
            <span className="shrink-0 text-xs text-muted-foreground">v{appVersion}</span>
          )}
        </div>

        {/* Band 2: Chat wizard */}
        <div className="flex-1 min-h-0 overflow-hidden">
          <ChatWizard
            appVersion={appVersion}
            onComplete={() => router.replace('/')}
            onPhaseChange={setPhase}
          />
        </div>

        {/* Band 3: Footer — phase progress dots */}
        <div className="shrink-0 border-t border-border px-8 py-3 flex items-center justify-center gap-2">
          {[0, 1, 2, 3, 4].map(i => (
            <div key={i} className={cn(
              'h-1.5 rounded-full transition-all duration-300',
              phase > i   ? 'w-6 bg-primary'      :
              phase === i ? 'w-6 bg-primary/60'    :
                            'w-1.5 bg-border',
            )} />
          ))}
        </div>
      </div>
    </div>
  );
}
