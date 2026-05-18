'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useState, useEffect, Suspense } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import { auth, setToken, clearToken } from '@/lib/api';
import { Loader2, Eye, EyeOff, MessageCircle, Mail, Share2, Code2, Plug } from 'lucide-react';

type Mode = 'loading' | 'login' | 'setup';

const features = [
  { icon: MessageCircle, title: 'Chat with any AI',           desc: 'Ask questions, write content, get answers. Works with ChatGPT, Gemini, Claude, DeepSeek — your choice.' },
  { icon: Mail,          title: 'Handles your email',         desc: 'Reads your inbox, drafts replies, sends follow-ups. Your email runs itself while you focus on what matters.' },
  { icon: Share2,        title: 'Posts to social media',      desc: 'Write once, publish everywhere — Instagram, Twitter/X, LinkedIn, Facebook. Scheduled or instant.' },
  { icon: Code2,         title: 'Writes and runs code',       desc: 'Your AI writes, tests, and fixes code on its own. Full IDE in the browser. No developer needed.' },
  { icon: Plug,          title: 'Connects to anything',       desc: 'Tell it what service you need — it figures out the integration, builds the connection, and it\'s ready to use.' },
];

function LoginForm() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const [mode, setMode] = useState<Mode>('loading');
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [email, setEmail] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);
  const [showPassword, setShowPassword] = useState(false);

  useEffect(() => {
    const ctrl = new AbortController();
    const timeout = setTimeout(() => ctrl.abort(), 6000);

    const run = async () => {
      // If a token exists in localStorage, validate it before trusting it.
      const storedToken = typeof window !== 'undefined' ? localStorage.getItem('qorven_token') : null;
      if (storedToken) {
        try {
          const r = await fetch('/api/v1/user/preferences', {
            headers: { Authorization: `Bearer ${storedToken}` },
            signal: ctrl.signal,
          });
          if (r.ok) {
            // Re-set the cookie (may be absent if user cleared cookies but kept localStorage)
            // then hard-navigate so the server sees the cookie on the first request.
            setToken(storedToken);
            window.location.href = searchParams?.get('next') || '/';
            return;
          }
        } catch {
          // Network error — fall through to show login form
        }
        // Token is stale/invalid — clear localStorage and cookie
        clearToken();
        localStorage.removeItem('qorven_user');
      }

      try {
        const r = await fetch('/api/auth/setup-check', { signal: ctrl.signal });
        const d: { setup_required?: boolean } = await r.json();
        clearTimeout(timeout);
        if (d.setup_required) router.replace('/setup');
        else setMode('login');
      } catch {
        clearTimeout(timeout);
        setMode('login');
      }
    };

    run();
    return () => { clearTimeout(timeout); ctrl.abort(); };
  }, [router, searchParams]);

  const handleLogin = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');
    setLoading(true);
    try {
      const data = await auth.login({ username, password });
      setToken(data.token);
      localStorage.setItem('qorven_user', JSON.stringify(data.user));
      // Hard navigation so the browser sends the newly-set cookie on the
      // first request to the server (proxy.ts checks cookies server-side).
      window.location.href = searchParams?.get('next') || searchParams?.get('from') || '/';
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Invalid username or password');
    } finally {
      setLoading(false);
    }
  };

  const handleSetup = async (e: React.FormEvent) => {
    e.preventDefault();
    if (password.length < 8) { setError('Password must be at least 8 characters'); return; }
    setError('');
    setLoading(true);
    try {
      await auth.setup({ username, password, email: email || undefined });
      const data = await auth.login({ username, password });
      setToken(data.token);
      window.location.href = searchParams?.get('next') || '/';
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Setup failed');
    } finally {
      setLoading(false);
    }
  };

  if (mode === 'loading') {
    return (
      <div className="flex h-screen w-full flex-col items-center justify-center gap-3 bg-background">
        <Loader2 className="h-6 w-6 animate-spin text-primary" />
        <p className="text-sm text-muted-foreground">Connecting to backend…</p>
      </div>
    );
  }

  const isSetup = mode === 'setup';

  return (
    <div className="grid min-h-screen w-full lg:grid-cols-2 bg-background">

      {/* ── Left: form panel ─────────────────────────────────────── */}
      <div className="flex flex-col justify-between px-8 py-10 sm:px-12 lg:px-16">

        {/* Top: logo */}
        <div>
          <img src="/logo/qorven-wordmark.svg" alt="Qorven" className="h-8" />
        </div>

        {/* Center: form */}
        <div className="mx-auto w-full max-w-sm">
          <div className="mb-8">
            <h1 className="text-2xl font-bold tracking-tight">
              {isSetup ? 'Create your account' : 'Welcome back'}
            </h1>
            <p className="mt-1.5 text-sm text-muted-foreground">
              {isSetup
                ? 'Set up your admin account to get started with Qorven.'
                : 'Sign in to your AI workspace.'}
            </p>
          </div>

          <form onSubmit={isSetup ? handleSetup : handleLogin} className="space-y-4">
            {error && (
              <div className="flex items-start gap-2.5 rounded-xl border border-destructive/30 bg-destructive/5 px-4 py-3 text-sm text-destructive">
                <span className="mt-px shrink-0 text-lg leading-none">⚠</span>
                {error}
              </div>
            )}

            {/* Username */}
            <div className="space-y-1.5">
              <label className="text-sm font-medium">Username</label>
              <input
                value={username}
                onChange={e => setUsername(e.target.value)}
                autoFocus
                required
                autoComplete="username"
                placeholder="admin"
                className="h-11 w-full rounded-xl border border-border bg-card px-4 text-sm outline-none ring-0 transition-colors placeholder:text-muted-foreground/40 focus:border-primary"
              />
            </div>

            {/* Email (setup only) */}
            {isSetup && (
              <div className="space-y-1.5">
                <label className="text-sm font-medium">
                  Email <span className="text-muted-foreground font-normal">(optional)</span>
                </label>
                <input
                  value={email}
                  onChange={e => setEmail(e.target.value)}
                  type="email"
                  autoComplete="email"
                  placeholder="you@example.com"
                  className="h-11 w-full rounded-xl border border-border bg-card px-4 text-sm outline-none transition-colors placeholder:text-muted-foreground/40 focus:border-primary"
                />
              </div>
            )}

            {/* Password */}
            <div className="space-y-1.5">
              <div className="flex items-center justify-between">
                <label className="text-sm font-medium">Password</label>
                {!isSetup && (
                  <span className="text-xs text-muted-foreground cursor-default select-none">
                    Forgot password?
                  </span>
                )}
              </div>
              <div className="relative">
                <input
                  value={password}
                  onChange={e => setPassword(e.target.value)}
                  type={showPassword ? 'text' : 'password'}
                  required
                  autoComplete={isSetup ? 'new-password' : 'current-password'}
                  placeholder="••••••••"
                  className="h-11 w-full rounded-xl border border-border bg-card pl-4 pr-11 text-sm outline-none transition-colors placeholder:text-muted-foreground/40 focus:border-primary"
                />
                <button
                  type="button"
                  onClick={() => setShowPassword(v => !v)}
                  className="absolute right-3.5 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground transition-colors"
                >
                  {showPassword
                    ? <EyeOff className="h-4 w-4" />
                    : <Eye className="h-4 w-4" />}
                </button>
              </div>
              {isSetup && (
                <p className="text-xs text-muted-foreground">At least 8 characters</p>
              )}
            </div>

            {/* Submit */}
            <button
              type="submit"
              disabled={loading || !username || !password}
              className="relative mt-2 h-11 w-full overflow-hidden rounded-xl bg-primary text-sm font-semibold text-primary-foreground transition-opacity hover:opacity-90 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {loading ? (
                <Loader2 className="mx-auto h-4 w-4 animate-spin" />
              ) : isSetup ? (
                'Create Account & Sign In'
              ) : (
                'Sign In'
              )}
            </button>

            {/* Forgot password — shown on login mode only. Setup mode
                is first-admin creation, there's no account to recover. */}
            {!isSetup && (
              <div className="pt-1 text-center">
                <a href="/forgot-password" className="text-xs text-muted-foreground hover:text-primary underline-offset-4 hover:underline">
                  Forgot password?
                </a>
              </div>
            )}
          </form>
        </div>

        {/* Bottom: footer */}
        <p className="text-center text-xs text-muted-foreground/40">
          &copy; 2026 Qorven. Self-hosted AI workspace.
        </p>
      </div>

      {/* ── Right: feature showcase ───────────────────────────────── */}
      <div className="relative hidden overflow-hidden lg:flex lg:flex-col lg:justify-between bg-gradient-to-br from-primary via-primary/90 to-primary/70 p-12 text-primary-foreground">

        {/* Decorative blobs */}
        <div className="pointer-events-none absolute -top-32 -right-32 h-96 w-96 rounded-full bg-white/5 blur-3xl" />
        <div className="pointer-events-none absolute bottom-0 left-0 h-72 w-72 rounded-full bg-black/10 blur-3xl" />

        {/* Top: wordmark */}
        <div className="relative">
          <img src="/logo/qorven-wordmark-white.svg" alt="Qorven" className="h-8" />
        </div>

        {/* Middle: headline + feature list */}
        <div className="relative space-y-10">
          <div className="space-y-3">
            <h2 className="text-3xl font-bold leading-tight">
              Your AI handles the work.<br />You handle what matters.
            </h2>
            <p className="text-base text-white/70 leading-relaxed max-w-xs">
              Chat, email, social media, code — one AI workspace that runs on your own server, around the clock.
            </p>
          </div>

          <div className="space-y-5">
            {features.map(({ icon: Icon, title, desc }) => (
              <div key={title} className="flex items-start gap-3.5">
                <div className="mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-white/10 backdrop-blur-sm">
                  <Icon className="h-4.5 w-4.5" />
                </div>
                <div>
                  <p className="text-sm font-semibold">{title}</p>
                  <p className="text-xs text-white/60 mt-0.5 leading-relaxed">{desc}</p>
                </div>
              </div>
            ))}
          </div>
        </div>

        {/* Bottom: stat strip */}
        <div className="relative flex items-center gap-8">
          {[
            { value: '18+',  label: 'Channels'        },
            { value: '14+',  label: 'AI providers'    },
            { value: '70+',  label: 'Built-in tools'  },
          ].map(({ value, label }) => (
            <div key={label} className="space-y-0.5">
              <p className="text-2xl font-bold">{value}</p>
              <p className="text-xs text-white/50">{label}</p>
            </div>
          ))}
        </div>
      </div>

    </div>
  );
}

export default function LoginPage() {
  return (
    <Suspense fallback={
      <div className="flex h-screen w-full items-center justify-center bg-background">
        <div className="h-6 w-6 animate-spin rounded-full border-2 border-primary border-t-transparent" />
      </div>
    }>
      <LoginForm />
    </Suspense>
  );
}
