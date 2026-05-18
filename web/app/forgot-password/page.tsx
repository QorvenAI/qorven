'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useRef, useState } from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { Loader2, KeyRound, MessageCircle, Terminal } from 'lucide-react';
import { authBase } from '@/lib/api-url';
import { extractErrorMessage } from '@/lib/api-core';

type Step = 'request' | 'verify' | 'reset';

export default function ForgotPasswordPage() {
  const router = useRouter();
  const [step, setStep] = useState<Step>('request');

  // Step 1
  const [identifier, setIdentifier] = useState('');
  const [delivery, setDelivery] = useState<'telegram' | 'log' | null>(null);

  // Step 2
  const [otp, setOtp] = useState('');
  const [resetToken, setResetToken] = useState('');
  const [resendCountdown, setResendCountdown] = useState(0);
  const countdownRef = useRef<ReturnType<typeof setInterval> | null>(null);

  // Step 3
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');

  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [success, setSuccess] = useState('');

  useEffect(() => {
    return () => { if (countdownRef.current) clearInterval(countdownRef.current); };
  }, []);

  function startResendCountdown() {
    setResendCountdown(60);
    countdownRef.current = setInterval(() => {
      setResendCountdown(n => {
        if (n <= 1) { clearInterval(countdownRef.current!); return 0; }
        return n - 1;
      });
    }, 1000);
  }

  async function handleRequest(e: React.FormEvent) {
    e.preventDefault();
    if (!identifier.trim() || loading) return;
    setError('');
    setLoading(true);
    try {
      const res = await fetch(`${authBase()}/forgot-password`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username_or_email: identifier.trim() }),
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) {
        setError(extractErrorMessage(data?.error || 'Request failed. Please try again.'));
        return;
      }
      setDelivery(data.delivery ?? 'log');
      setStep('verify');
      startResendCountdown();
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
      const res = await fetch(`${authBase()}/verify-otp`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username_or_email: identifier.trim(), otp: otp.trim() }),
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

  async function handleReset(e: React.FormEvent) {
    e.preventDefault();
    if (!newPassword || loading) return;
    if (newPassword !== confirmPassword) {
      setError('Passwords do not match.');
      return;
    }
    if (newPassword.length < 8) {
      setError('Password must be at least 8 characters.');
      return;
    }
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
    setError('');
    setOtp('');
    setLoading(true);
    try {
      const res = await fetch(`${authBase()}/forgot-password`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username_or_email: identifier.trim() }),
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) {
        setError(extractErrorMessage(data?.error || 'Could not resend code.'));
        return;
      }
      setDelivery(data.delivery ?? 'log');
      startResendCountdown();
    } catch {
      setError('Could not connect to the server.');
    } finally {
      setLoading(false);
    }
  }

  const stepLabels = ['Request code', 'Enter code', 'New password'];
  const stepIndex = step === 'request' ? 0 : step === 'verify' ? 1 : 2;

  return (
    <div className="min-h-screen bg-background flex items-center justify-center p-4">
      <div className="w-full max-w-md space-y-6">

        {/* Header */}
        <div className="text-center space-y-1">
          <div className="mx-auto flex h-12 w-12 items-center justify-center rounded-xl bg-primary/10">
            <KeyRound className="h-5 w-5 text-primary" />
          </div>
          <h1 className="text-lg font-semibold">Reset your password</h1>
          <p className="text-xs text-muted-foreground">
            {step === 'request' && 'Enter your username or email to receive a reset code.'}
            {step === 'verify' && 'Enter the 6-digit code we sent you.'}
            {step === 'reset' && 'Choose a new password for your account.'}
          </p>
        </div>

        {/* Step indicator */}
        <div className="flex items-center gap-2">
          {stepLabels.map((label, i) => (
            <div key={label} className="flex items-center gap-2 flex-1">
              <div className={`flex h-6 w-6 shrink-0 items-center justify-center rounded-full text-xs font-semibold
                ${i < stepIndex ? 'bg-primary text-primary-foreground' :
                  i === stepIndex ? 'bg-primary text-primary-foreground' :
                  'bg-muted text-muted-foreground'}`}>
                {i < stepIndex ? '✓' : i + 1}
              </div>
              <span className={`text-xs ${i === stepIndex ? 'text-foreground font-medium' : 'text-muted-foreground'}`}>
                {label}
              </span>
              {i < 2 && <div className={`flex-1 h-px ${i < stepIndex ? 'bg-primary' : 'bg-border'}`} />}
            </div>
          ))}
        </div>

        {/* Step 1: Request */}
        {step === 'request' && (
          <form onSubmit={handleRequest} className="rounded-xl border border-border bg-card p-5 space-y-4">
            <label className="block">
              <span className="text-xs font-medium text-muted-foreground">Username or email</span>
              <input
                value={identifier}
                onChange={e => setIdentifier(e.target.value)}
                autoFocus
                autoComplete="username"
                className="qr-input" />
            </label>
            {error && <p className="text-xs text-destructive">{error}</p>}
            <button type="submit" disabled={loading || !identifier.trim()}
              className="w-full rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-40">
              {loading ? <Loader2 className="mx-auto h-4 w-4 animate-spin" /> : 'Send reset code'}
            </button>
            <div className="text-center text-xs">
              <Link href="/login" className="text-muted-foreground hover:text-primary hover:underline">
                Back to sign in
              </Link>
            </div>
          </form>
        )}

        {/* Step 2: Verify OTP */}
        {step === 'verify' && (
          <form onSubmit={handleVerify} className="rounded-xl border border-border bg-card p-5 space-y-4">
            {/* Delivery hint */}
            <div className="flex items-start gap-3 rounded-lg bg-muted px-3 py-2.5">
              {delivery === 'telegram'
                ? <MessageCircle className="h-4 w-4 mt-0.5 shrink-0 text-primary" />
                : <Terminal className="h-4 w-4 mt-0.5 shrink-0 text-muted-foreground" />}
              <p className="text-xs text-muted-foreground">
                {delivery === 'telegram'
                  ? 'A 6-digit code was sent to your paired Telegram account.'
                  : 'No Telegram paired. Check the server logs for your 6-digit code.'}
              </p>
            </div>

            <label className="block">
              <span className="text-xs font-medium text-muted-foreground">6-digit code</span>
              <input
                value={otp}
                onChange={e => setOtp(e.target.value.replace(/\D/g, '').slice(0, 6))}
                inputMode="numeric"
                autoFocus
                placeholder="000000"
                className="mt-1 qr-input tracking-widest text-center" />
            </label>
            {error && <p className="text-xs text-destructive">{error}</p>}
            <button type="submit" disabled={loading || otp.length !== 6}
              className="w-full rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-40">
              {loading ? <Loader2 className="mx-auto h-4 w-4 animate-spin" /> : 'Verify code'}
            </button>
            <div className="flex items-center justify-between text-xs">
              <button type="button" onClick={() => setStep('request')}
                className="text-muted-foreground hover:text-primary hover:underline">
                Use a different account
              </button>
              <button type="button" onClick={handleResend}
                disabled={resendCountdown > 0 || loading}
                className="text-muted-foreground hover:text-primary hover:underline disabled:opacity-40">
                {resendCountdown > 0 ? `Resend in ${resendCountdown}s` : 'Resend code'}
              </button>
            </div>
          </form>
        )}

        {/* Step 3: New password */}
        {step === 'reset' && (
          <form onSubmit={handleReset} className="rounded-xl border border-border bg-card p-5 space-y-4">
            <label className="block">
              <span className="text-xs font-medium text-muted-foreground">New password</span>
              <input
                type="password"
                value={newPassword}
                onChange={e => setNewPassword(e.target.value)}
                autoFocus
                autoComplete="new-password"
                className="qr-input" />
            </label>
            <label className="block">
              <span className="text-xs font-medium text-muted-foreground">Confirm password</span>
              <input
                type="password"
                value={confirmPassword}
                onChange={e => setConfirmPassword(e.target.value)}
                autoComplete="new-password"
                className="qr-input" />
            </label>
            {error && <p className="text-xs text-destructive">{error}</p>}
            {success && <p className="text-xs text-green-500">{success}</p>}
            <button type="submit" disabled={loading || !newPassword || !confirmPassword}
              className="w-full rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-40">
              {loading ? <Loader2 className="mx-auto h-4 w-4 animate-spin" /> : 'Set new password'}
            </button>
          </form>
        )}

      </div>
    </div>
  );
}
