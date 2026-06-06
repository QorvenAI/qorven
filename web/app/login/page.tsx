'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useState, useEffect, Suspense } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import { auth, setToken, clearToken } from '@/lib/api';
import { Loader2, Eye, EyeOff, MessageCircle, Mail, Share2, Code2, Plug } from 'lucide-react';

type Mode = 'login' | 'setup';

const features = [
  { icon: MessageCircle, title: 'Any AI model',      desc: 'Claude, GPT, Gemini, DeepSeek — switch models per task, no lock-in.' },
  { icon: Mail,          title: 'Email on autopilot', desc: 'Reads, drafts, and sends. Inbox zero without touching it.' },
  { icon: Share2,        title: 'Social media',       desc: 'One post, every platform — Instagram, X, LinkedIn, Facebook.' },
  { icon: Code2,         title: 'Writes your code',   desc: 'Full browser IDE. AI writes, tests, and deploys. Solo or with a team.' },
  { icon: Plug,          title: 'Connects anything',  desc: '150+ integrations. Describe what you need — it builds the connector.' },
];

/** Validate the `next` redirect param — only allow same-origin paths. */
function safeNext(next: string | null | undefined): string {
  if (!next) return '/';
  // Must start with / and not be a protocol-relative URL (//evil.com)
  if (!next.startsWith('/') || next.startsWith('//')) return '/';
  try {
    // Parse relative to current origin — rejects anything with a different host
    const url = new URL(next, typeof window !== 'undefined' ? window.location.origin : 'http://localhost');
    if (url.origin !== (typeof window !== 'undefined' ? window.location.origin : 'http://localhost')) return '/';
    return url.pathname + url.search;
  } catch {
    return '/';
  }
}

function LoginForm() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const [mode, setMode] = useState<Mode>('login');
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
            window.location.href = safeNext(searchParams?.get('next'));
            return;
          }
        } catch {
          // Network error — fall through to show login form
        }
        // Token is stale/invalid — clear localStorage and cookie
        clearToken();
        localStorage.removeItem('qorven_user');
      }

      // Setup check runs silently — form is already visible (mode starts as 'login').
      // Only switch to 'setup' if the backend says first-run setup is needed.
      try {
        const r = await fetch('/api/auth/setup-check', { signal: ctrl.signal });
        const d: { setup_required?: boolean } = await r.json();
        clearTimeout(timeout);
        if (d.setup_required) setMode('setup');
      } catch {
        clearTimeout(timeout);
        // Backend unreachable — login form stays visible, user will see auth error on submit
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
      window.location.href = safeNext(searchParams?.get('next') ?? searchParams?.get('from'));
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
      window.location.href = safeNext(searchParams?.get('next'));
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Setup failed');
    } finally {
      setLoading(false);
    }
  };

  const isSetup = mode === 'setup';

  return (
    <div className="grid min-h-screen w-full lg:grid-cols-2 bg-background">

      {/* ── Left: form panel ─────────────────────────────────────── */}
      <div className="flex flex-col px-8 py-8 sm:px-12 lg:px-16">

        {/* Top: logo */}
        <div>
          <img src="/logo/qorven-wordmark.svg" alt="Qorven" className="h-8" />
        </div>

        {/* Center: form — flex-1 centers content, py-10 keeps spacing on small screens */}
        <div className="flex-1 flex flex-col justify-center">
        <div className="mx-auto w-full max-w-sm py-10">
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
        </div>

        {/* Bottom: footer */}
        <p className="text-center text-xs text-muted-foreground/40">
          &copy; 2026 Qorven. Self-hosted AI workspace.
        </p>
      </div>

      {/* ── Right: feature showcase ───────────────────────────────── */}
      <div className="relative hidden overflow-hidden lg:flex lg:flex-col bg-gradient-to-br from-primary via-primary/90 to-primary/70 px-12 py-8 text-primary-foreground">

        {/* Decorative blobs */}
        <div className="pointer-events-none absolute -top-32 -right-32 h-96 w-96 rounded-full bg-white/5 blur-3xl" />
        <div className="pointer-events-none absolute bottom-0 left-0 h-72 w-72 rounded-full bg-black/10 blur-3xl" />

        {/* Top: wordmark */}
        <div className="relative shrink-0">
          <img src="/logo/qorven-wordmark-white.svg" alt="Qorven" className="h-8" />
        </div>

        {/* Middle: headline + feature list */}
        <div className="relative flex flex-1 flex-col justify-center gap-5 py-4">
          <div className="space-y-2">
            <h2 className="text-3xl font-bold leading-tight">
              Your agents work.<br />You decide what matters.
            </h2>
            <p className="text-sm text-white/70 leading-relaxed max-w-xs">
              Self-hosted. Open source. 42 agents across every department — running 24/7 on your server.
            </p>
          </div>

          <div className="space-y-3">
            {features.map(({ icon: Icon, title, desc }) => (
              <div key={title} className="flex items-center gap-3">
                <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-white/10">
                  <Icon className="h-4 w-4" />
                </div>
                <div className="min-w-0">
                  <p className="text-sm font-semibold leading-none">{title}</p>
                  <p className="text-xs text-white/55 mt-0.5 truncate">{desc}</p>
                </div>
              </div>
            ))}
          </div>
        </div>

        {/* Bottom: stat strip */}
        <div className="relative shrink-0 flex items-center gap-8">
          {[
            { value: '21+', label: 'Channels'     },
            { value: '14+', label: 'AI providers' },
            { value: '42',  label: 'Agents'       },
          ].map(({ value, label }) => (
            <div key={label} className="space-y-0.5">
              <p className="text-xl font-bold">{value}</p>
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
