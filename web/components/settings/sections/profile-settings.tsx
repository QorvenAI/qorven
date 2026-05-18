'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useRef, useState } from 'react';
import { Eye, EyeOff, Loader2, Monitor, Smartphone, Globe, Trash2 } from 'lucide-react';
import { toast } from 'sonner';
import { userApi } from '@/lib/api';
import { Card, Row, Input, SaveBar, Btn } from './primitives';

function parseUserAgent(ua: string): { browser: string; os: string; device: string } {
  const browser = /Edg\//.test(ua) ? 'Edge'
    : /Chrome\//.test(ua) ? 'Chrome'
    : /Firefox\//.test(ua) ? 'Firefox'
    : /Safari\//.test(ua) && !/Chrome/.test(ua) ? 'Safari'
    : /OPR\/|Opera/.test(ua) ? 'Opera'
    : 'Browser';

  const os = /Windows NT/.test(ua) ? 'Windows'
    : /Mac OS X/.test(ua) ? 'macOS'
    : /iPhone/.test(ua) ? 'iPhone'
    : /iPad/.test(ua) ? 'iPad'
    : /Android/.test(ua) ? 'Android'
    : /Linux/.test(ua) ? 'Linux'
    : 'Unknown OS';

  const device = /iPhone|iPad|Android/.test(ua) ? 'mobile' : 'desktop';
  return { browser, os, device };
}

function relativeTime(iso: string): string {
  const diff = Date.now() - new Date(iso).getTime();
  const mins = Math.floor(diff / 60000);
  if (mins < 1) return 'just now';
  if (mins < 60) return `${mins}m ago`;
  const hrs = Math.floor(mins / 60);
  if (hrs < 24) return `${hrs}h ago`;
  const days = Math.floor(hrs / 24);
  return `${days}d ago`;
}

export function ProfileSettings() {
  // ── profile state ──
  const [user, setUser] = useState<any>(null);
  const [loading, setLoading] = useState(true);
  const [displayName, setDisplayName] = useState('');
  const [email, setEmail] = useState('');
  const [avatarURL, setAvatarURL] = useState('');
  const [avatarPreview, setAvatarPreview] = useState('');
  const [saving, setSaving] = useState(false);
  const fileRef = useRef<HTMLInputElement>(null);

  // ── password state ──
  const [pwd, setPwd] = useState({ current: '', next: '', confirm: '' });
  const [savingPwd, setSavingPwd] = useState(false);
  const [showPwd, setShowPwd] = useState(false);

  // ── sessions state ──
  const [sessions, setSessions] = useState<any[]>([]);
  const [loadingSessions, setLoadingSessions] = useState(true);
  const [revoking, setRevoking] = useState<string | null>(null);

  useEffect(() => {
    userApi.me()
      .then((u: any) => {
        setUser(u);
        setDisplayName(u?.display_name ?? '');
        setEmail(u?.email ?? '');
        setAvatarURL(u?.avatar_url ?? '');
        setAvatarPreview(u?.avatar_url ?? '');
      })
      .catch(() => {
        try {
          const stored = localStorage.getItem('qorven_user');
          if (stored) {
            const u = JSON.parse(stored);
            setUser(u);
            setDisplayName(u?.display_name ?? '');
            setEmail(u?.email ?? '');
          }
        } catch {}
      })
      .finally(() => setLoading(false));

    userApi.listSessions()
      .then(setSessions)
      .catch(() => {})
      .finally(() => setLoadingSessions(false));
  }, []);

  const profileDirty = user && (
    displayName !== (user.display_name ?? '') ||
    email !== (user.email ?? '') ||
    avatarURL !== (user.avatar_url ?? '')
  );

  const handleAvatarFile = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    const reader = new FileReader();
    reader.onload = (ev) => {
      const dataUrl = ev.target?.result as string;
      setAvatarPreview(dataUrl);
      setAvatarURL(dataUrl);
    };
    reader.readAsDataURL(file);
  };

  const handleSaveProfile = async () => {
    setSaving(true);
    try {
      const body: { email?: string; display_name?: string; avatar_url?: string } = {};
      if (email !== (user?.email ?? '')) body.email = email;
      if (displayName !== (user?.display_name ?? '')) body.display_name = displayName;
      if (avatarURL !== (user?.avatar_url ?? '')) body.avatar_url = avatarURL;
      const updated = await userApi.patchProfile(body);
      if (updated?.error) throw new Error(updated.error);
      setUser(updated);
      toast.success('Profile saved');
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Save failed');
    } finally { setSaving(false); }
  };

  const handleChangePassword = async () => {
    if (!pwd.current) { toast.error('Enter your current password'); return; }
    if (pwd.next.length < 8) { toast.error('New password must be at least 8 characters'); return; }
    if (pwd.next !== pwd.confirm) { toast.error('Passwords do not match'); return; }
    setSavingPwd(true);
    try {
      const res = await userApi.changePassword({ current_password: pwd.current, new_password: pwd.next });
      if (res?.error) throw new Error(res.error);
      toast.success('Password updated');
      setPwd({ current: '', next: '', confirm: '' });
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Failed to update password');
    } finally { setSavingPwd(false); }
  };

  const handleRevokeSession = async (id: string) => {
    setRevoking(id);
    try {
      await userApi.revokeSession(id);
      setSessions((prev) => prev.filter((s) => s.id !== id));
      toast.success('Session terminated');
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Failed to revoke session');
    } finally { setRevoking(null); }
  };

  const pwdSuffix = (
    <button type="button" onClick={() => setShowPwd((s) => !s)}
      className="text-muted-foreground hover:text-foreground transition-colors cursor-pointer">
      {showPwd ? <EyeOff className="h-3.5 w-3.5" /> : <Eye className="h-3.5 w-3.5" />}
    </button>
  );

  if (loading) return (
    <div className="rounded-xl border border-border bg-card p-12 flex justify-center">
      <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
    </div>
  );

  const initial = user?.display_name?.charAt(0)?.toUpperCase() || user?.username?.charAt(0)?.toUpperCase() || '?';

  return (
    <div className="space-y-4">
      {/* Profile */}
      <Card id="profile_info" title="Profile" description="Update your name, email and profile picture.">
        <div className="flex items-center gap-4 pb-4 mb-2 border-b border-border/70">
          <button onClick={() => fileRef.current?.click()} className="relative shrink-0 group cursor-pointer">
            {avatarPreview ? (
              <img src={avatarPreview} alt="avatar" className="h-16 w-16 rounded-full object-cover border border-border" />
            ) : (
              <div className="h-16 w-16 rounded-full bg-primary/10 text-primary text-2xl font-bold flex items-center justify-center border border-border">
                {initial}
              </div>
            )}
            <span className="absolute inset-0 rounded-full bg-black/40 opacity-0 group-hover:opacity-100 flex items-center justify-center text-white text-2xs font-medium transition-opacity">
              Change
            </span>
          </button>
          <div>
            <p className="text-base font-semibold">{user?.username ?? '—'}</p>
            {user?.email && <p className="text-sm text-muted-foreground">{user.email}</p>}
          </div>
          <input ref={fileRef} type="file" accept="image/*" className="hidden" onChange={handleAvatarFile} />
        </div>

        <Row label="Name">
          <Input value={displayName} onChange={setDisplayName} placeholder="Your name" />
        </Row>
        <Row label="Email">
          <Input value={email} onChange={setEmail} type="email" placeholder="you@example.com" />
        </Row>
        <Row label="Username">
          <Input value={user?.username ?? ''} readOnly />
        </Row>

        {profileDirty && <SaveBar saving={saving} onSave={handleSaveProfile} label="Save Profile" />}
      </Card>

      {/* Password */}
      <Card id="change_password" title="Password" description="Use a strong password of at least 8 characters.">
        <Row label="Current Password">
          <Input type={showPwd ? 'text' : 'password'} value={pwd.current} onChange={(v) => setPwd((p) => ({ ...p, current: v }))} placeholder="••••••••" suffix={pwdSuffix} />
        </Row>
        <Row label="New Password" hint="Min. 8 characters">
          <Input type={showPwd ? 'text' : 'password'} value={pwd.next} onChange={(v) => setPwd((p) => ({ ...p, next: v }))} placeholder="••••••••" suffix={pwdSuffix} />
        </Row>
        <Row label="Confirm Password">
          <Input type={showPwd ? 'text' : 'password'} value={pwd.confirm} onChange={(v) => setPwd((p) => ({ ...p, confirm: v }))} placeholder="••••••••" suffix={pwdSuffix} />
        </Row>
        <div className="pt-1">
          <Btn onClick={handleChangePassword} loading={savingPwd} disabled={!pwd.current || !pwd.next || !pwd.confirm}>
            Update Password
          </Btn>
        </div>
      </Card>

      {/* Active Sessions */}
      <Card id="sessions" title="Active Sessions" description="Devices and browsers currently signed in to your account.">
        {loadingSessions ? (
          <div className="flex justify-center py-6"><Loader2 className="h-4 w-4 animate-spin text-muted-foreground" /></div>
        ) : sessions.length === 0 ? (
          <p className="text-sm text-muted-foreground py-3">No active sessions found.</p>
        ) : (
          <div className="space-y-2">
            {sessions.map((s) => {
              const { browser, os, device } = parseUserAgent(s.user_agent || '');
              const DeviceIcon = device === 'mobile' ? Smartphone : Monitor;
              const lastActive = s.last_used_at || s.created_at;
              return (
                <div key={s.id} className="flex items-center gap-3 rounded-xl border border-border px-4 py-3">
                  <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-muted">
                    <DeviceIcon className="h-4 w-4 text-muted-foreground" />
                  </div>
                  <div className="flex-1 min-w-0">
                    <p className="text-sm font-medium">{browser} on {os}</p>
                    <div className="flex items-center gap-2 text-xs text-muted-foreground">
                      {s.ip_address && (
                        <span className="flex items-center gap-1">
                          <Globe className="h-3 w-3" />{s.ip_address}
                        </span>
                      )}
                      <span>Active {relativeTime(lastActive)}</span>
                      <span>Since {new Date(s.created_at).toLocaleDateString()}</span>
                    </div>
                  </div>
                  <button
                    onClick={() => handleRevokeSession(s.id)}
                    disabled={revoking === s.id}
                    className="flex h-7 w-7 items-center justify-center rounded-md text-muted-foreground hover:text-destructive hover:bg-destructive/10 transition-colors disabled:opacity-40"
                    title="Terminate session"
                  >
                    {revoking === s.id ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Trash2 className="h-3.5 w-3.5" />}
                  </button>
                </div>
              );
            })}
          </div>
        )}
      </Card>
    </div>
  );
}
