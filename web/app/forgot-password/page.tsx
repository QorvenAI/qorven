'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useRef, useState } from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { Loader2, MessageCircle, Terminal, KeyRound, ArrowLeft } from 'lucide-react';
import { authBase } from '@/lib/api-url';
import { extractErrorMessage } from '@/lib/api-core';

type Step = 'request' | 'verify' | 'reset';
type Delivery = 'telegram' | 'no_telegram' | 'no_user' | 'error' | null;

export default function ForgotPasswordPage() {
  const router = useRouter();
  const [step, setStep]         = useState<Step>('request');
  const [delivery, setDelivery] = useState<Delivery>(null);
  const [otp, setOtp]           = useState('');
  const [resetToken, setResetToken] = useState('');
  const [newPassword, setNewPassword]         = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [loading, setLoading]   = useState(false);
  const [error, setError]       = useState('');
  const [success, setSuccess]   = useState('');
  const [resendCountdown, setResendCountdown] = useState(0);
  const countdownRef = useRef<ReturnType<typeof setInterval> | null>(null);

  useEffect(() => {
    return () => { if (countdownRef.current) clearInterval(countdownRef.current); };
  }, []);

  function startCountdown() {
    setResendCountdown(60);
    countdownRef.current = setInterval(() => {
      setResendCountdown(n => {
        if (n <= 1) { clearInterval(countdownRef.current!); return 0; }
        return n - 1;
      });
    }, 1000);
  }

  async function handleRequest() {
    if (loading) return;
    setError('');
    setLoading(true);
    try {
      const res  = await fetch(`${authBase()}/forgot-password`, {
        method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({}),
      });
      const data = await res.json().catch(() => ({}));
      setDelivery(data.delivery ?? 'no_telegram');
      setStep('verify');
      startCountdown();
    } catch {
      setError('Could not connect to the server. Please try again.');
    } finally {
      setLoading(false);
    }
  }

  async function handleVerify(e: React.FormEvent) {
    e.preventDefault();
    if (otp.length !== 6 || loading) return;
    setError('');
    setLoading(true);
    try {
      const res  = await fetch(`${authBase()}/verify-otp`, {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ otp: otp.trim() }),
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) { setError(extractErrorMessage(data?.error || 'Invalid code. Please try again.')); return; }
      setResetToken(data.token);
      setStep('reset');
    } catch {
      setError('Could not connect to the server. Please try again.');
    } finally {
      setLoading(false);
    }
  }

  async function handleReset(e: React.FormEvent) {
    e.preventDefault();
    if (!newPassword || loading) return;
    if (newPassword !== confirmPassword) { setError('Passwords do not match.'); return; }
    if (newPassword.length < 8)          { setError('Password must be at least 8 characters.'); return; }
    setError('');
    setLoading(true);
    try {
      const res  = await fetch(`${authBase()}/reset-password`, {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ token: resetToken, new_password: newPassword }),
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) { setError(extractErrorMessage(data?.error || 'Could not reset password.')); return; }
      setSuccess('Password updated successfully. Redirecting…');
      setTimeout(() => router.push('/login'), 1800);
    } catch {
      setError('Could not connect to the server. Please try again.');
    } finally {
      setLoading(false);
    }
  }

  async function handleResend() {
    if (resendCountdown > 0 || loading) return;
    setOtp(''); setError(''); setLoading(true);
    try {
      const res  = await fetch(`${authBase()}/forgot-password`, {
        method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({}),
      });
      const data = await res.json().catch(() => ({}));
      setDelivery(data.delivery ?? 'no_telegram');
      startCountdown();
    } catch {
      setError('Could not connect to the server.');
    } finally {
      setLoading(false);
    }
  }

  const stepIndex = step === 'request' ? 0 : step === 'verify' ? 1 : 2;

  return (
    <div className="grid min-h-screen w-full lg:grid-cols-2 bg-background">

      {/* ── Left: form panel ── */}
      <div className="flex flex-col justify-between px-8 py-10 sm:px-12 lg:px-16">

        {/* Top: logo */}
        <div>
          <img src="/logo/qorven-wordmark.svg" alt="Qorven" className="h-8" />
        </div>

        {/* Center: form */}
        <div className="mx-auto w-full max-w-sm">

          {/* Back link */}
          <Link href="/login"
            className="mb-6 inline-flex items-center gap-1.5 text-xs text-muted-foreground hover:text-foreground transition-colors">
            <ArrowLeft className="h-3.5 w-3.5" />
            Back to sign in
          </Link>

          <div className="mb-8">
            <h1 className="text-2xl font-bold tracking-tight">Reset your password</h1>
            <p className="mt-1.5 text-sm text-muted-foreground">
              {step === 'request' && "We'll send a reset code to your Telegram."}
              {step === 'verify'  && 'Enter the 6-digit code sent to your Telegram.'}
              {step === 'reset'   && 'Choose a new password for your account.'}
            </p>
          </div>

          {/* Step indicator */}
          <div className="flex items-center gap-2 mb-8">
            {['Send code', 'Enter code', 'New password'].map((label, i) => (
              <div key={label} className="flex items-center gap-2 flex-1 min-w-0">
                <div className={`flex h-6 w-6 shrink-0 items-center justify-center rounded-full text-[11px] font-semibold
                  ${i < stepIndex  ? 'bg-primary text-primary-foreground' :
                    i === stepIndex ? 'bg-primary text-primary-foreground' :
                    'bg-muted text-muted-foreground'}`}>
                  {i < stepIndex ? '✓' : i + 1}
                </div>
                <span className={`text-xs truncate ${i === stepIndex ? 'text-foreground font-medium' : 'text-muted-foreground'}`}>
                  {label}
                </span>
                {i < 2 && <div className={`flex-1 h-px shrink ${i < stepIndex ? 'bg-primary' : 'bg-border'}`} />}
              </div>
            ))}
          </div>

          {/* ── Step 1: Request ── */}
          {step === 'request' && (
            <div className="space-y-4">
              <div className="flex items-start gap-3 rounded-xl border border-border bg-muted/40 px-4 py-3">
                <MessageCircle className="h-4 w-4 mt-0.5 shrink-0 text-primary" />
                <p className="text-sm text-muted-foreground">
                  A 6-digit code will be sent to your paired Telegram account.
                  No Telegram? Use <code className="font-mono text-xs bg-muted rounded px-1 py-px">qorven reset-password</code> via SSH.
                </p>
              </div>
              {error && (
                <div className="flex items-start gap-2.5 rounded-xl border border-destructive/30 bg-destructive/5 px-4 py-3 text-sm text-destructive">
                  <span className="mt-px shrink-0 text-lg leading-none">⚠</span>
                  {error}
                </div>
              )}
              <button onClick={handleRequest} disabled={loading}
                className="w-full rounded-xl bg-primary px-4 py-2.5 text-sm font-semibold text-primary-foreground hover:bg-primary/90 disabled:opacity-50 flex items-center justify-center gap-2 transition-colors">
                {loading
                  ? <><Loader2 className="h-4 w-4 animate-spin" /> Sending…</>
                  : <><MessageCircle className="h-4 w-4" /> Send reset code via Telegram</>}
              </button>
            </div>
          )}

          {/* ── Step 2: Verify ── */}
          {step === 'verify' && (
            <form onSubmit={handleVerify} className="space-y-4">
              {delivery === 'telegram' ? (
                <div className="flex items-start gap-3 rounded-xl border border-border bg-muted/40 px-4 py-3">
                  <MessageCircle className="h-4 w-4 mt-0.5 shrink-0 text-primary" />
                  <p className="text-sm text-muted-foreground">
                    Code sent to your Telegram. Check the chat with your Qorven bot.
                  </p>
                </div>
              ) : (
                <div className="flex items-start gap-3 rounded-xl border border-amber-500/30 bg-amber-500/5 px-4 py-3">
                  <Terminal className="h-4 w-4 mt-0.5 shrink-0 text-amber-400" />
                  <div className="space-y-1">
                    <p className="text-sm font-medium text-amber-300">Telegram not connected</p>
                    <p className="text-xs text-amber-300/70">Reset your password via SSH:</p>
                    <code className="block bg-black/20 rounded px-2 py-1 font-mono text-xs text-amber-200">
                      qorven reset-password
                    </code>
                  </div>
                </div>
              )}

              {delivery === 'telegram' && (
                <>
                  <div>
                    <label className="mb-1.5 block text-sm font-medium">6-digit code</label>
                    <input
                      value={otp}
                      onChange={e => setOtp(e.target.value.replace(/\D/g, '').slice(0, 6))}
                      inputMode="numeric"
                      autoFocus
                      placeholder="000000"
                      className="w-full rounded-xl border border-input bg-background px-4 py-2.5 text-center text-xl font-mono tracking-widest shadow-xs focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring placeholder:text-muted-foreground/40"
                    />
                  </div>
                  {error && (
                    <div className="flex items-start gap-2.5 rounded-xl border border-destructive/30 bg-destructive/5 px-4 py-3 text-sm text-destructive">
                      <span className="mt-px shrink-0 text-lg leading-none">⚠</span>
                      {error}
                    </div>
                  )}
                  <button type="submit" disabled={loading || otp.length !== 6}
                    className="w-full rounded-xl bg-primary px-4 py-2.5 text-sm font-semibold text-primary-foreground hover:bg-primary/90 disabled:opacity-50 flex items-center justify-center transition-colors">
                    {loading ? <Loader2 className="h-4 w-4 animate-spin" /> : 'Verify code'}
                  </button>
                  <div className="flex items-center justify-between text-xs text-muted-foreground">
                    <button type="button" onClick={() => setStep('request')}
                      className="hover:text-foreground transition-colors">
                      Back
                    </button>
                    <button type="button" onClick={handleResend}
                      disabled={resendCountdown > 0 || loading}
                      className="hover:text-foreground transition-colors disabled:opacity-40">
                      {resendCountdown > 0 ? `Resend in ${resendCountdown}s` : 'Resend code'}
                    </button>
                  </div>
                </>
              )}

              {delivery !== 'telegram' && (
                <p className="text-center text-xs text-muted-foreground">
                  <Link href="/login" className="hover:text-foreground transition-colors underline underline-offset-4">
                    Back to sign in
                  </Link>
                </p>
              )}
            </form>
          )}

          {/* ── Step 3: New password ── */}
          {step === 'reset' && (
            <form onSubmit={handleReset} className="space-y-4">
              <div>
                <label className="mb-1.5 block text-sm font-medium">New password</label>
                <input type="password" value={newPassword}
                  onChange={e => setNewPassword(e.target.value)}
                  autoFocus autoComplete="new-password"
                  className="w-full rounded-xl border border-input bg-background px-4 py-2.5 text-sm shadow-xs focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
                />
              </div>
              <div>
                <label className="mb-1.5 block text-sm font-medium">Confirm password</label>
                <input type="password" value={confirmPassword}
                  onChange={e => setConfirmPassword(e.target.value)}
                  autoComplete="new-password"
                  className="w-full rounded-xl border border-input bg-background px-4 py-2.5 text-sm shadow-xs focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
                />
              </div>
              {error && (
                <div className="flex items-start gap-2.5 rounded-xl border border-destructive/30 bg-destructive/5 px-4 py-3 text-sm text-destructive">
                  <span className="mt-px shrink-0 text-lg leading-none">⚠</span>
                  {error}
                </div>
              )}
              {success && (
                <div className="flex items-start gap-2.5 rounded-xl border border-green-500/30 bg-green-500/5 px-4 py-3 text-sm text-green-500">
                  <span className="mt-px shrink-0 text-lg leading-none">✓</span>
                  {success}
                </div>
              )}
              <button type="submit" disabled={loading || !newPassword || !confirmPassword}
                className="w-full rounded-xl bg-primary px-4 py-2.5 text-sm font-semibold text-primary-foreground hover:bg-primary/90 disabled:opacity-50 flex items-center justify-center transition-colors">
                {loading ? <Loader2 className="h-4 w-4 animate-spin" /> : 'Set new password'}
              </button>
            </form>
          )}
        </div>

        {/* Bottom: footer */}
        <p className="text-center text-xs text-muted-foreground/40">
          &copy; 2026 Qorven. Self-hosted AI workspace.
        </p>
      </div>

      {/* ── Right: purple showcase panel (same as login) ── */}
      <div className="relative hidden overflow-hidden lg:flex lg:flex-col lg:justify-between bg-gradient-to-br from-primary via-primary/90 to-primary/70 p-12 text-primary-foreground">
        <div className="pointer-events-none absolute -top-32 -right-32 h-96 w-96 rounded-full bg-white/5 blur-3xl" />
        <div className="pointer-events-none absolute bottom-0 left-0 h-72 w-72 rounded-full bg-black/10 blur-3xl" />

        <div className="relative">
          <img src="/logo/qorven-wordmark-white.svg" alt="Qorven" className="h-8" />
        </div>

        <div className="relative space-y-6">
          <div className="flex h-16 w-16 items-center justify-center rounded-2xl bg-white/10 backdrop-blur-sm">
            <KeyRound className="h-8 w-8" />
          </div>
          <div className="space-y-3">
            <h2 className="text-3xl font-bold leading-tight">
              Back in<br />60 seconds.
            </h2>
            <p className="text-base text-white/70 leading-relaxed max-w-xs">
              OTP to Telegram. No email, no third-party. Everything stays on your server.
            </p>
          </div>
          <div className="space-y-3">
            {[
              { icon: MessageCircle, text: 'Code sent to Telegram — not email' },
              { icon: KeyRound,      text: 'All sessions revoked on reset'     },
              { icon: Terminal,      text: 'No Telegram? SSH: qorven reset-password' },
            ].map(({ icon: Icon, text }) => (
              <div key={text} className="flex items-center gap-3">
                <div className="flex h-7 w-7 shrink-0 items-center justify-center rounded-lg bg-white/10">
                  <Icon className="h-3.5 w-3.5" />
                </div>
                <p className="text-sm text-white/70">{text}</p>
              </div>
            ))}
          </div>
        </div>

        <div className="relative flex items-center gap-8">
          {[
            { value: '15m',    label: 'Code expires'    },
            { value: '7d',     label: 'Sessions last'   },
            { value: 'bcrypt', label: 'Passwords hashed' },
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
