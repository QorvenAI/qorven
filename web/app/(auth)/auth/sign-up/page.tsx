'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useState } from 'react';
import Link from 'next/link';
import { Eye, EyeOff, Loader2 } from 'lucide-react';

export default function SignUpPage() {
  const [form, setForm] = useState({ name: '', email: '', password: '', confirm: '' });
  const [showPw, setShowPw] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const set = (k: string) => (e: React.ChangeEvent<HTMLInputElement>) => setForm({ ...form, [k]: e.target.value });

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (form.password !== form.confirm) { setError('Passwords do not match'); return; }
    setLoading(true); setError('');
    try {
      const res = await fetch('/api/auth/sign-up', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(form) });
      if (!res.ok) { setError('Registration failed'); setLoading(false); return; }
      window.location.href = '/auth/sign-in?registered=true';
    } catch { setError('Connection failed'); setLoading(false); }
  }

  return (
    <form onSubmit={handleSubmit} className="flex flex-col gap-4">
      <div className="text-center">
        <h1 className="text-xl font-semibold mb-1">Create Account</h1>
        <p className="text-2sm text-muted-foreground">
          Already have an account?{' '}
          <Link href="/auth/sign-in" className="text-primary hover:text-primary/80 font-medium">Sign In</Link>
        </p>
      </div>

      {error && <div className="rounded-lg border border-destructive/50 bg-destructive/10 px-3 py-2 text-sm text-destructive">{error}</div>}

      <div className="flex flex-col gap-1">
        <label className="text-sm font-medium">Full Name</label>
        <input type="text" value={form.name} onChange={set('name')} required placeholder="Jane Doe"
          className="h-10 rounded-lg border border-input bg-transparent px-3 text-sm placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring focus:ring-offset-2 focus:ring-offset-background" />
      </div>

      <div className="flex flex-col gap-1">
        <label className="text-sm font-medium">Email</label>
        <input type="email" value={form.email} onChange={set('email')} required placeholder="you@company.com"
          className="h-10 rounded-lg border border-input bg-transparent px-3 text-sm placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring focus:ring-offset-2 focus:ring-offset-background" />
      </div>

      <div className="flex flex-col gap-1">
        <label className="text-sm font-medium">Password</label>
        <div className="relative">
          <input type={showPw ? 'text' : 'password'} value={form.password} onChange={set('password')} required placeholder="Min 8 characters"
            className="h-10 w-full rounded-lg border border-input bg-transparent px-3 pr-10 text-sm placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring focus:ring-offset-2 focus:ring-offset-background" />
          <button type="button" onClick={() => setShowPw(!showPw)} className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground">
            {showPw ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
          </button>
        </div>
      </div>

      <div className="flex flex-col gap-1">
        <label className="text-sm font-medium">Confirm Password</label>
        <input type="password" value={form.confirm} onChange={set('confirm')} required placeholder="Re-enter password"
          className="h-10 rounded-lg border border-input bg-transparent px-3 text-sm placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring focus:ring-offset-2 focus:ring-offset-background" />
      </div>

      <label className="flex items-start gap-2 text-sm">
        <input type="checkbox" required className="h-4 w-4 mt-0.5 rounded border-input accent-primary" />
        <span className="text-muted-foreground">I agree to the <a href="#" className="text-primary hover:underline">Terms</a> and <a href="#" className="text-primary hover:underline">Privacy Policy</a></span>
      </label>

      <button type="submit" disabled={loading}
        className="h-10 rounded-lg bg-primary text-primary-foreground font-medium text-sm hover:bg-primary/90 disabled:opacity-50 flex items-center justify-center gap-2 transition-colors">
        {loading && <Loader2 className="h-4 w-4 animate-spin" />}
        Create Account
      </button>
    </form>
  );
}
