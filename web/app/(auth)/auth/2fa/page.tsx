'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useState } from 'react';
import Link from 'next/link';
import { Loader2 } from 'lucide-react';

export default function TwoFactorPage() {
  const [code, setCode] = useState(['', '', '', '', '', '']);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  function handleChange(i: number, v: string) {
    if (v.length > 1) return;
    const next = [...code]; next[i] = v; setCode(next);
    if (v && i < 5) (document.getElementById(`otp-${i + 1}`) as HTMLInputElement)?.focus();
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    const pin = code.join('');
    if (pin.length !== 6) { setError('Enter all 6 digits'); return; }
    setLoading(true); setError('');
    try {
      const res = await fetch('/api/auth/2fa', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ code: pin }) });
      if (!res.ok) { setError('Invalid code'); setLoading(false); return; }
      window.location.href = '/';
    } catch { setError('Connection failed'); setLoading(false); }
  }

  return (
    <form onSubmit={handleSubmit} className="flex flex-col gap-5">
      <div className="text-center">
        <h1 className="text-xl font-semibold mb-1">Two-Factor Authentication</h1>
        <p className="text-2sm text-muted-foreground">Enter the 6-digit code from your authenticator app</p>
      </div>
      {error && <div className="rounded-lg border border-destructive/50 bg-destructive/10 px-3 py-2 text-sm text-destructive">{error}</div>}
      <div className="flex justify-center gap-2">
        {code.map((d, i) => (
          <input key={i} id={`otp-${i}`} type="text" inputMode="numeric" maxLength={1} value={d} onChange={e => handleChange(i, e.target.value)}
            className="h-12 w-12 rounded-lg border border-input bg-transparent text-center text-lg font-semibold focus:outline-none focus:ring-2 focus:ring-ring focus:ring-offset-2 focus:ring-offset-background" />
        ))}
      </div>
      <button type="submit" disabled={loading}
        className="h-10 rounded-lg bg-primary text-primary-foreground font-medium text-sm hover:bg-primary/90 disabled:opacity-50 flex items-center justify-center gap-2 transition-colors">
        {loading && <Loader2 className="h-4 w-4 animate-spin" />}
        Verify
      </button>
      <p className="text-center text-2sm text-muted-foreground">
        <Link href="/auth/sign-in" className="text-primary hover:text-primary/80">Use backup code</Link>
      </p>
    </form>
  );
}
