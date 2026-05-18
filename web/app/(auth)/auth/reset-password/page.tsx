'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useState } from 'react';
import Link from 'next/link';
import { Loader2 } from 'lucide-react';

export default function ResetPasswordPage() {
  const [email, setEmail] = useState('');
  const [loading, setLoading] = useState(false);
  const [sent, setSent] = useState(false);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setLoading(true);
    try {
      await fetch('/api/auth/reset-password', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ email }) });
      setSent(true);
    } catch {} finally { setLoading(false); }
  }

  if (sent) return (
    <div className="flex flex-col gap-5 text-center">
      <div className="mx-auto flex h-12 w-12 items-center justify-center rounded-full bg-primary/10">
        <svg className="h-6 w-6 text-primary" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth="1.5"><path strokeLinecap="round" strokeLinejoin="round" d="M21.75 6.75v10.5a2.25 2.25 0 01-2.25 2.25h-15a2.25 2.25 0 01-2.25-2.25V6.75m19.5 0A2.25 2.25 0 0019.5 4.5h-15a2.25 2.25 0 00-2.25 2.25m19.5 0v.243a2.25 2.25 0 01-1.07 1.916l-7.5 4.615a2.25 2.25 0 01-2.36 0L3.32 8.91a2.25 2.25 0 01-1.07-1.916V6.75" /></svg>
      </div>
      <h1 className="text-xl font-semibold">Check Your Email</h1>
      <p className="text-2sm text-muted-foreground">We sent a reset link to <span className="text-foreground font-medium">{email}</span></p>
      <Link href="/auth/sign-in" className="h-10 rounded-lg bg-primary text-primary-foreground font-medium text-sm hover:bg-primary/90 flex items-center justify-center transition-colors">Back to Sign In</Link>
    </div>
  );

  return (
    <form onSubmit={handleSubmit} className="flex flex-col gap-5">
      <div className="text-center">
        <h1 className="text-xl font-semibold mb-1">Reset Password</h1>
        <p className="text-2sm text-muted-foreground">Enter your email to receive a reset link</p>
      </div>
      <div className="flex flex-col gap-1">
        <label className="text-sm font-medium">Email</label>
        <input type="email" value={email} onChange={e => setEmail(e.target.value)} required placeholder="you@company.com"
          className="h-10 rounded-lg border border-input bg-transparent px-3 text-sm placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring focus:ring-offset-2 focus:ring-offset-background" />
      </div>
      <button type="submit" disabled={loading}
        className="h-10 rounded-lg bg-primary text-primary-foreground font-medium text-sm hover:bg-primary/90 disabled:opacity-50 flex items-center justify-center gap-2 transition-colors">
        {loading && <Loader2 className="h-4 w-4 animate-spin" />}
        Send Reset Link
      </button>
      <p className="text-center text-2sm text-muted-foreground">
        <Link href="/auth/sign-in" className="text-primary hover:text-primary/80">Back to Sign In</Link>
      </p>
    </form>
  );
}
