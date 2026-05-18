'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

// Reset page — step 2 of the password reset flow. The user lands here
// from a magic link (/reset?token=...) and sets a new password. The
// token is single-use and expires 15 minutes after it was issued.

import { useState, useEffect, Suspense } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import Link from 'next/link';
import { CheckCircle2, Eye, EyeOff, KeyRound, Loader2 } from 'lucide-react';
import { authBase } from '@/lib/api-url';

function ResetForm() {
  const router = useRouter();
  const params = useSearchParams();
  const [token, setToken] = useState('');
  const [password, setPassword] = useState('');
  const [confirm, setConfirm] = useState('');
  const [showPw, setShowPw] = useState(false);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);
  const [done, setDone] = useState(false);

  useEffect(() => {
    setToken(params?.get('token') ?? '');
  }, [params]);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');
    if (!token) { setError('Reset token missing from URL'); return; }
    if (password.length < 8) { setError('Password must be at least 8 characters'); return; }
    if (password !== confirm) { setError('Passwords do not match'); return; }
    setLoading(true);
    try {
      const r = await fetch(`${authBase()}/reset-password`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ token, new_password: password }),
      });
      const data = await r.json();
      if (!r.ok) throw new Error(data?.error || 'Reset failed');
      setDone(true);
      // Auto-redirect to login after 3s so the user doesn't have to
      // hunt for the link.
      setTimeout(() => router.push('/login'), 3000);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Reset failed');
    } finally {
      setLoading(false);
    }
  };

  if (done) {
    return (
      <div className="rounded-xl border border-emerald-500/30 bg-emerald-500/10 p-5 text-center space-y-2">
        <CheckCircle2 className="mx-auto h-8 w-8 text-emerald-400" />
        <p className="text-sm font-medium text-emerald-300">Password reset.</p>
        <p className="text-xs text-muted-foreground">Redirecting to sign in…</p>
      </div>
    );
  }

  return (
    <form onSubmit={submit} className="rounded-xl border border-border bg-card p-5 space-y-3">
      {error && (
        <div className="rounded-lg border border-destructive/30 bg-destructive/10 px-3 py-2 text-xs text-destructive">
          {error}
        </div>
      )}

      <label className="block">
        <span className="text-xs font-medium text-muted-foreground">New password</span>
        <div className="relative mt-1">
          <input
            type={showPw ? 'text' : 'password'}
            value={password}
            onChange={e => setPassword(e.target.value)}
            autoFocus
            className="qr-input pr-9" />
          <button type="button" onClick={() => setShowPw(!showPw)}
            className="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground">
            {showPw ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
          </button>
        </div>
      </label>

      <label className="block">
        <span className="text-xs font-medium text-muted-foreground">Confirm new password</span>
        <input
          type={showPw ? 'text' : 'password'}
          value={confirm}
          onChange={e => setConfirm(e.target.value)}
          className="qr-input" />
      </label>

      <button type="submit" disabled={loading || !password || !confirm}
        className="w-full rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-40">
        {loading ? <Loader2 className="mx-auto h-4 w-4 animate-spin" /> : 'Reset password'}
      </button>

      <div className="pt-1 text-center text-xs">
        <Link href="/login" className="text-muted-foreground hover:text-primary hover:underline">
          Back to sign in
        </Link>
      </div>
    </form>
  );
}

export default function ResetPage() {
  return (
    <div className="min-h-screen bg-background flex items-center justify-center p-4">
      <div className="w-full max-w-md space-y-6">
        <div className="text-center space-y-1">
          <div className="mx-auto flex h-12 w-12 items-center justify-center rounded-xl bg-primary/10">
            <KeyRound className="h-5 w-5 text-primary" />
          </div>
          <h1 className="text-lg font-semibold">Set a new password</h1>
          <p className="text-xs text-muted-foreground">Link expires 15 minutes after it was issued.</p>
        </div>
        <Suspense fallback={<div className="text-xs text-muted-foreground text-center">loading…</div>}>
          <ResetForm />
        </Suspense>
      </div>
    </div>
  );
}
