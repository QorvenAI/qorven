'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useRef, useState } from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { Loader2, KeyRound, MessageCircle, Terminal } from 'lucide-react';
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
  const [newPassword, setNewPassword]       = useState('');
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

  // ── Step 1: Request OTP ────────────────────────────────────────────────────
  async function handleRequest() {
    if (loading) return;
    setError('');
    setLoading(true);
    try {
      const res = await fetch(`${authBase()}/forgot-password`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({}), // no username — auto-resolves admin user
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

  // ── Step 2: Verify OTP ─────────────────────────────────────────────────────
  async function handleVerify(e: React.FormEvent) {
    e.preventDefault();
    if (otp.length !== 6 || loading) return;
    setError('');
    setLoading(true);
    try {
      const res = await fetch(`${authBase()}/verify-otp`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ otp: otp.trim() }), // no username needed
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) {
        setError(extractErrorMessage(data?.error || 'Invalid code. Please try again.'));
        return;
      }
      setResetToken(data.token);
      setStep('reset');
    } catch {
      setError('Could not connect to the server. Please try again.');
    } finally {
      setLoading(false);
    }
  }

  // ── Step 3: Set new password ───────────────────────────────────────────────
  async function handleReset(e: React.FormEvent) {
    e.preventDefault();
    if (!newPassword || loading) return;
    if (newPassword !== confirmPassword) { setError('Passwords do not match.'); return; }
    if (newPassword.length < 8) { setError('Password must be at least 8 characters.'); return; }
    setError('');
    setLoading(true);
    try {
      const res = await fetch(`${authBase()}/reset-password`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ token: resetToken, new_password: newPassword }),
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) {
        setError(extractErrorMessage(data?.error || 'Could not reset password. Please try again.'));
        return;
      }
      setSuccess('Password updated. Redirecting…');
      setTimeout(() => router.push('/login'), 1800);
    } catch {
      setError('Could not connect to the server. Please try again.');
    } finally {
      setLoading(false);
    }
  }

  async function handleResend() {
    if (resendCountdown > 0 || loading) return;
    setOtp('');
    setError('');
    setLoading(true);
    try {
      const res = await fetch(`${authBase()}/forgot-password`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({}),
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

  return (
    <div className="min-h-screen bg-background flex items-center justify-center p-4">
      <div className="w-full max-w-md space-y-6">

        {/* Header */}
        <div className="text-center space-y-2">
          <div className="mx-auto flex h-12 w-12 items-center justify-center rounded-xl bg-primary/10">
            <KeyRound className="h-5 w-5 text-primary" />
          </div>
          <h1 className="text-lg font-semibold">Reset your password</h1>
          <p className="text-xs text-muted-foreground">
            {step === 'request' && 'We\'ll send a reset code to your paired Telegram account.'}
            {step === 'verify'  && 'Enter the 6-digit code sent to your Telegram.'}
            {step === 'reset'   && 'Choose a new password for your account.'}
          </p>
        </div>

        {/* Step indicator */}
        <div className="flex items-center gap-2">
          {['Send code', 'Enter code', 'New password'].map((label, i) => {
            const current = step === 'request' ? 0 : step === 'verify' ? 1 : 2;
            return (
              <div key={label} className="flex items-center gap-2 flex-1">
                <div className={`flex h-6 w-6 shrink-0 items-center justify-center rounded-full text-xs font-semibold
                  ${i <= current ? 'bg-primary text-primary-foreground' : 'bg-muted text-muted-foreground'}`}>
                  {i < current ? '✓' : i + 1}
                </div>
                <span className={`text-xs ${i === current ? 'text-foreground font-medium' : 'text-muted-foreground'}`}>
                  {label}
                </span>
                {i < 2 && <div className={`flex-1 h-px ${i < current ? 'bg-primary' : 'bg-border'}`} />}
              </div>
            );
          })}
        </div>

        {/* ── Step 1: Request ── */}
        {step === 'request' && (
          <div className="rounded-xl border border-border bg-card p-5 space-y-4">
            <div className="flex items-start gap-3 rounded-lg bg-muted px-3 py-2.5">
              <MessageCircle className="h-4 w-4 mt-0.5 shrink-0 text-primary" />
              <p className="text-xs text-muted-foreground">
                A 6-digit reset code will be sent to your paired Telegram account.
                If you haven&apos;t paired Telegram, use <code className="font-mono text-[10px] bg-background px-1 py-0.5 rounded">qorven reset-password</code> via SSH instead.
              </p>
            </div>
            {error && <p className="text-xs text-destructive">{error}</p>}
            <button
              onClick={handleRequest}
              disabled={loading}
              className="w-full rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-40 flex items-center justify-center gap-2"
            >
              {loading ? <Loader2 className="h-4 w-4 animate-spin" /> : <MessageCircle className="h-4 w-4" />}
              {loading ? 'Sending…' : 'Send reset code via Telegram'}
            </button>
            <div className="text-center text-xs">
              <Link href="/login" className="text-muted-foreground hover:text-primary hover:underline">
                Back to sign in
              </Link>
            </div>
          </div>
        )}

        {/* ── Step 2: Verify ── */}
        {step === 'verify' && (
          <form onSubmit={handleVerify} className="rounded-xl border border-border bg-card p-5 space-y-4">

            {/* Delivery status */}
            {delivery === 'telegram' ? (
              <div className="flex items-start gap-3 rounded-lg bg-muted px-3 py-2.5">
                <MessageCircle className="h-4 w-4 mt-0.5 shrink-0 text-primary" />
                <p className="text-xs text-muted-foreground">
                  A 6-digit code was sent to your paired Telegram account.
                </p>
              </div>
            ) : (
              <div className="flex items-start gap-3 rounded-lg bg-amber-500/10 border border-amber-500/20 px-3 py-2.5">
                <Terminal className="h-4 w-4 mt-0.5 shrink-0 text-amber-400" />
                <div className="text-xs text-amber-300 space-y-1">
                  <p className="font-medium">Telegram not connected</p>
                  <p className="text-amber-300/70">Use the CLI on your server to reset directly:</p>
                  <code className="block bg-black/30 rounded px-2 py-1 font-mono text-[11px] text-amber-200">
                    qorven reset-password
                  </code>
                </div>
              </div>
            )}

            {delivery === 'telegram' && (
              <>
                <label className="block">
                  <span className="text-xs font-medium text-muted-foreground">6-digit code</span>
                  <input
                    value={otp}
                    onChange={e => setOtp(e.target.value.replace(/\D/g, '').slice(0, 6))}
                    inputMode="numeric"
                    autoFocus
                    placeholder="000000"
                    className="mt-1 qr-input tracking-widest text-center text-lg"
                  />
                </label>
                {error && <p className="text-xs text-destructive">{error}</p>}
                <button
                  type="submit"
                  disabled={loading || otp.length !== 6}
                  className="w-full rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-40"
                >
                  {loading ? <Loader2 className="mx-auto h-4 w-4 animate-spin" /> : 'Verify code'}
                </button>
                <div className="flex items-center justify-between text-xs">
                  <button type="button" onClick={() => setStep('request')}
                    className="text-muted-foreground hover:text-primary hover:underline">
                    Back
                  </button>
                  <button type="button" onClick={handleResend}
                    disabled={resendCountdown > 0 || loading}
                    className="text-muted-foreground hover:text-primary hover:underline disabled:opacity-40">
                    {resendCountdown > 0 ? `Resend in ${resendCountdown}s` : 'Resend code'}
                  </button>
                </div>
              </>
            )}

            {delivery !== 'telegram' && (
              <div className="text-center text-xs">
                <Link href="/login" className="text-muted-foreground hover:text-primary hover:underline">
                  Back to sign in
                </Link>
              </div>
            )}
          </form>
        )}

        {/* ── Step 3: New password ── */}
        {step === 'reset' && (
          <form onSubmit={handleReset} className="rounded-xl border border-border bg-card p-5 space-y-4">
            <label className="block">
              <span className="text-xs font-medium text-muted-foreground">New password</span>
              <input type="password" value={newPassword}
                onChange={e => setNewPassword(e.target.value)}
                autoFocus autoComplete="new-password" className="mt-1 qr-input" />
            </label>
            <label className="block">
              <span className="text-xs font-medium text-muted-foreground">Confirm password</span>
              <input type="password" value={confirmPassword}
                onChange={e => setConfirmPassword(e.target.value)}
                autoComplete="new-password" className="mt-1 qr-input" />
            </label>
            {error   && <p className="text-xs text-destructive">{error}</p>}
            {success && <p className="text-xs text-green-500">{success}</p>}
            <button type="submit"
              disabled={loading || !newPassword || !confirmPassword}
              className="w-full rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-40">
              {loading ? <Loader2 className="mx-auto h-4 w-4 animate-spin" /> : 'Set new password'}
            </button>
          </form>
        )}

      </div>
    </div>
  );
}
