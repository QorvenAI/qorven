'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useState, useEffect, useCallback } from 'react';
import { useRouter } from 'next/navigation';
import { useStore } from '@/store';
import { cn } from '@/lib/utils';
import { soulGradient } from '@/components/soul-card';
import {
  Inbox, Send, FileEdit, Clock, Star, ChevronsUpDown, Users,
  GitBranch, Sparkles, FileText, AtSign, ShieldCheck, Settings,
  ArrowLeft, Plus, Check, Loader2, Trash2, Save, Server, Search,
  Mail,
} from 'lucide-react';
import { SidebarMenuItem, SidebarDivider, SidebarGroupTitle } from './sidebar-primitives';
import { SearchableSelect } from '@/components/searchable-select';
import { mail as mailApi, type MailIdentity, type MailAlias } from '@/lib/api';
import { contacts as contactsApi, type Contact, type ContactAgentPrefs } from '@/lib/api';
import { toast } from 'sonner';

type MailView = 'folders' | 'contacts' | 'mailboxes';

export function MailSidebar() {
  const router = useRouter();
  const souls = useStore((s) => s.souls);
  const mailSoulFilter = useStore((s) => s.mailSoulFilter);
  const setMailSoulFilter = useStore((s) => s.setMailSoulFilter);
  const mailFolder = useStore((s) => s.mailFolder);
  const setMailFolder = useStore((s) => s.setMailFolder);
  const mailView = useStore((s) => s.mailView);
  const setMailView = useStore((s) => s.setMailView);
  const [pickerOpen, setPickerOpen] = useState(false);
  const [approvalCount, setApprovalCount] = useState(0);

  const view: MailView = mailView === 'contacts' ? 'contacts' : mailView === 'mailboxes' ? 'mailboxes' : 'folders';

  const activeSoul = mailSoulFilter ? souls.find((s) => s.id === mailSoulFilter) : null;

  useEffect(() => {
    mailApi.identities().catch(() => []); // warm cache
    // approval badge
    import('@/lib/api').then(({ outbound }) =>
      outbound.mailPending().then(l => setApprovalCount(Array.isArray(l) ? l.length : 0)).catch(() => {})
    );
  }, []);

  const folders = [
    { icon: Inbox,    label: 'Inbox' },
    { icon: Send,     label: 'Sent' },
    { icon: FileEdit, label: 'Drafts' },
    { icon: Clock,    label: 'Pending' },
    { icon: Star,     label: 'Starred' },
  ];
  const groupOptions = [
    { icon: Inbox,     label: 'All Messages' },
    { icon: GitBranch, label: 'By Thread' },
    { icon: Sparkles,  label: 'By Qor' },
    { icon: FileText,  label: 'By Project' },
  ];

  const agentPicker = (
    <div className="relative px-3 pt-4 pb-2">
      <button
        onClick={() => setPickerOpen(!pickerOpen)}
        className="flex w-full items-center gap-2.5 h-8.5 rounded-md border border-input px-3 text-2sm font-medium hover:bg-accent transition-colors"
      >
        {activeSoul ? (
          <>
            <div className={cn('flex h-5 w-5 items-center justify-center rounded-full bg-gradient-to-br text-2xs font-semibold text-white', soulGradient(activeSoul.display_name))}>
              {activeSoul.display_name.charAt(0)}
            </div>
            <span className="flex-1 text-left truncate">{activeSoul.display_name}</span>
          </>
        ) : (
          <>
            <Users className="h-4 w-4 text-muted-foreground" />
            <span className="flex-1 text-left">All Agents</span>
          </>
        )}
        <ChevronsUpDown className="h-3.5 w-3.5 text-muted-foreground" />
      </button>
      {pickerOpen && (
        <div className="fixed z-[100] w-52 max-h-60 overflow-y-auto rounded-lg border border-border bg-popover shadow-lg py-1"
          style={{ left: 'calc(var(--rail-width) + var(--sidebar-default-width) + 4px)', top: 'calc(var(--header-height) + 8px)' }}>
          <button onClick={() => { setMailSoulFilter(null); setPickerOpen(false); }}
            className={cn('flex w-full items-center gap-2.5 px-3 py-2 text-2sm hover:bg-accent', !mailSoulFilter && 'bg-accent font-medium')}>
            <Users className="h-4 w-4 text-muted-foreground" /> All Agents
          </button>
          <div className="my-1 border-t border-border" />
          {souls.map((s) => (
            <button key={s.id} onClick={() => { setMailSoulFilter(s.id); setPickerOpen(false); }}
              className={cn('flex w-full items-center gap-2.5 px-3 py-2 text-2sm hover:bg-accent', mailSoulFilter === s.id && 'bg-accent font-medium')}>
              <div className={cn('flex h-5 w-5 items-center justify-center rounded-full bg-gradient-to-br text-2xs font-semibold text-white', soulGradient(s.display_name))}>
                {s.display_name.charAt(0)}
              </div>
              {s.display_name}
            </button>
          ))}
        </div>
      )}
    </div>
  );

  if (view === 'contacts') {
    return (
      <>
        {agentPicker}
        <ContactsView souls={souls} agentFilter={mailSoulFilter} onBack={() => setMailView(null)} />
      </>
    );
  }

  if (view === 'mailboxes') {
    return (
      <>
        {agentPicker}
        <MailboxesView souls={souls} onBack={() => setMailView(null)} />
      </>
    );
  }

  return (
    <>
      {agentPicker}

      <SidebarGroupTitle>Manage</SidebarGroupTitle>
      <ul className="flex flex-col gap-px px-2.5">
        <SidebarMenuItem icon={Users} label="Contacts" onClick={() => setMailView('contacts')} />
        <SidebarMenuItem icon={AtSign} label="Mailboxes" onClick={() => setMailView('mailboxes')} />
        <SidebarMenuItem
          icon={ShieldCheck}
          label="Approvals"
          badge={approvalCount > 0 ? String(approvalCount) : undefined}
          badgeColor="bg-amber-400/20 text-amber-600"
          onClick={() => router.push('/outbound')}
        />
        {mailSoulFilter && (
          <SidebarMenuItem
            icon={Settings}
            label="Mailbox settings"
            onClick={() => setMailView('mailboxes')}
          />
        )}
      </ul>

      <SidebarDivider />
      <SidebarGroupTitle>Folders</SidebarGroupTitle>
      <ul className="flex flex-col gap-px px-2.5 pb-2">
        {folders.map((f) => (
          <SidebarMenuItem
            key={f.label}
            icon={f.icon}
            label={f.label}
            active={mailFolder === f.label.toLowerCase()}
            onClick={() => { setMailFolder(f.label.toLowerCase()); router.push('/mail'); }}
          />
        ))}
      </ul>
    </>
  );
}

// ─── Contacts inline view ─────────────────────────────────────────

const TRUST_LABELS: Record<string, string> = {
  unknown: 'Unknown', known: 'Known', trusted: 'Trusted', blocked: 'Blocked',
};
const ROUTING_LABELS: Record<string, string> = {
  inherit: 'Use policy', auto: 'Auto-reply', draft: 'Draft & hold', skip: 'Skip',
};

function ContactsView({ souls, agentFilter, onBack }: { souls: any[]; agentFilter: string | null; onBack: () => void }) {
  const [list, setList] = useState<Contact[]>([]);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState('');
  const [selected, setSelected] = useState<Contact | null>(null);
  const [prefs, setPrefs] = useState<{ routing_mode: string; trust_level: string; agent_notes: string } | null>(null);
  const [saving, setSaving] = useState(false);

  const load = useCallback(() => {
    setLoading(true);
    contactsApi.list({ search: search || undefined })
      .then(d => setList((d as Contact[]).filter(c => c.channel === 'email' || !!c.email)))
      .catch(() => setList([]))
      .finally(() => setLoading(false));
  }, [search]);

  useEffect(() => { load(); }, [load]);

  const openContact = async (c: Contact) => {
    setSelected(c);
    if (agentFilter) {
      const p = await contactsApi.getPrefs(c.id, agentFilter).catch(() => null);
      setPrefs(p ? { routing_mode: p.routing_mode, trust_level: p.trust_level, agent_notes: p.agent_notes } : { routing_mode: 'inherit', trust_level: 'unknown', agent_notes: '' });
    }
  };

  const savePrefs = async () => {
    if (!selected || !agentFilter || !prefs) return;
    setSaving(true);
    await contactsApi.putPrefs(selected.id, agentFilter, prefs as any).then(() => toast.success('Saved')).catch(() => toast.error('Save failed'));
    setSaving(false);
  };

  return (
    <div className="flex flex-col flex-1 min-h-0">
      <div className="flex items-center gap-2 px-3 py-2 border-b border-border shrink-0">
        <button onClick={selected ? () => setSelected(null) : onBack}
          className="h-6 w-6 flex items-center justify-center rounded text-muted-foreground hover:bg-muted hover:text-foreground shrink-0">
          <ArrowLeft className="h-3.5 w-3.5" />
        </button>
        <span className="text-2sm font-medium">{selected ? (selected.display_name || 'Contact') : 'Contacts'}</span>
      </div>

      {selected ? (
        <div className="flex-1 overflow-y-auto p-3 space-y-3">
          <div className="flex items-center gap-2.5">
            <div className="h-9 w-9 rounded-full bg-primary/10 text-primary flex items-center justify-center font-semibold text-2sm shrink-0">
              {(selected.display_name || selected.email || 'U').charAt(0).toUpperCase()}
            </div>
            <div className="min-w-0">
              <p className="text-2sm font-medium truncate">{selected.display_name || '(no name)'}</p>
              <p className="text-xs text-muted-foreground truncate">{selected.email || selected.external_id}</p>
            </div>
          </div>

          {agentFilter && prefs && (
            <div className="space-y-3 text-xs">
              <SidebarGroupTitle>Preferences — {souls.find(s => s.id === agentFilter)?.display_name ?? 'Agent'}</SidebarGroupTitle>
              <div className="px-3">
                <p className="text-muted-foreground mb-1.5">Trust</p>
                <div className="flex flex-wrap gap-1.5">
                  {(['unknown', 'known', 'trusted', 'blocked'] as const).map(lvl => (
                    <button key={lvl} onClick={() => setPrefs(p => p ? { ...p, trust_level: lvl } : p)}
                      className={cn('rounded px-2 py-1 text-xs border transition-colors',
                        prefs.trust_level === lvl ? 'bg-primary/10 border-primary/40 text-primary' : 'border-border text-muted-foreground hover:bg-muted')}>
                      {TRUST_LABELS[lvl]}
                    </button>
                  ))}
                </div>
              </div>
              <div className="px-3">
                <p className="text-muted-foreground mb-1.5">Routing</p>
                <div className="flex flex-wrap gap-1.5">
                  {(['inherit', 'auto', 'draft', 'skip'] as const).map(mode => (
                    <button key={mode} onClick={() => setPrefs(p => p ? { ...p, routing_mode: mode } : p)}
                      className={cn('rounded px-2 py-1 text-xs border transition-colors',
                        prefs.routing_mode === mode ? 'bg-primary/10 border-primary/40 text-primary' : 'border-border text-muted-foreground hover:bg-muted')}>
                      {ROUTING_LABELS[mode]}
                    </button>
                  ))}
                </div>
              </div>
              <div className="px-3">
                <p className="text-muted-foreground mb-1.5">Notes</p>
                <textarea value={prefs.agent_notes} onChange={e => setPrefs(p => p ? { ...p, agent_notes: e.target.value } : p)}
                  rows={3} placeholder="Private notes…"
                  className="qr-textarea text-xs resize-none" />
              </div>
              <div className="px-3">
                <button onClick={savePrefs} disabled={saving}
                  className="flex items-center gap-1.5 rounded-md bg-primary text-primary-foreground px-3 py-1.5 text-xs font-medium hover:bg-primary/90 disabled:opacity-50">
                  {saving ? <Loader2 className="h-3 w-3 animate-spin" /> : <Save className="h-3 w-3" />} Save
                </button>
              </div>
            </div>
          )}
          {!agentFilter && (
            <p className="text-xs text-muted-foreground px-3">Select a Qor filter to edit per-agent preferences.</p>
          )}
        </div>
      ) : (
        <>
          <div className="px-3 py-2 border-b border-border shrink-0">
            <div className="relative">
              <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground pointer-events-none" />
              <input value={search} onChange={e => setSearch(e.target.value)} placeholder="Search…"
                className="qr-input text-xs pl-8" />
            </div>
          </div>
          <div className="flex-1 overflow-y-auto">
            {loading ? (
              <div className="flex items-center gap-2 p-4 text-xs text-muted-foreground">
                <Loader2 className="h-3.5 w-3.5 animate-spin" /> Loading…
              </div>
            ) : list.length === 0 ? (
              <div className="flex flex-col items-center py-10 gap-2 text-muted-foreground">
                <Users className="h-8 w-8 opacity-30" />
                <p className="text-xs text-center px-4">No email contacts yet. They appear once emails arrive.</p>
              </div>
            ) : (
              <ul className="flex flex-col gap-px px-2.5 py-2">
                {list.map(c => (
                  <button key={c.id} onClick={() => openContact(c)}
                    className="flex items-center gap-2.5 h-8.5 px-2.5 rounded-md text-2sm hover:bg-muted transition-colors text-left w-full">
                    <div className="h-5 w-5 rounded-full bg-primary/10 text-primary flex items-center justify-center text-2xs font-semibold shrink-0">
                      {(c.display_name || c.email || 'U').charAt(0).toUpperCase()}
                    </div>
                    <span className="flex-1 truncate">{c.display_name || c.email || c.external_id}</span>
                    <span className="text-xs text-muted-foreground shrink-0">{c.message_count}</span>
                  </button>
                ))}
              </ul>
            )}
          </div>
        </>
      )}
    </div>
  );
}

// ─── Mailboxes inline view ────────────────────────────────────────

const POLL_OPTIONS = [
  { value: '30',  label: '30 seconds' },
  { value: '60',  label: '1 minute' },
  { value: '300', label: '5 minutes' },
];

function MailboxesView({ souls, onBack }: { souls: any[]; onBack: () => void }) {
  const [tab, setTab] = useState<'dedicated' | 'shared'>('dedicated');
  const [identities, setIdentities] = useState<MailIdentity[]>([]);
  const [aliases, setAliases] = useState<MailAlias[]>([]);
  const [loading, setLoading] = useState(true);
  const [editingId, setEditingId] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    setLoading(true);
    const [ids, als] = await Promise.all([
      mailApi.identities().catch(() => [] as MailIdentity[]),
      mailApi.aliases().catch(() => [] as MailAlias[]),
    ]);
    setIdentities(ids);
    setAliases(als);
    setLoading(false);
  }, []);

  useEffect(() => { refresh(); }, [refresh]);

  const dedicated = identities.filter(i => i.identity_type !== 'shared');
  const shared    = identities.filter(i => i.identity_type === 'shared');

  return (
    <div className="flex flex-col flex-1 min-h-0">
      <div className="flex items-center gap-2 px-3 py-2 border-b border-border shrink-0">
        <button onClick={onBack}
          className="h-6 w-6 flex items-center justify-center rounded text-muted-foreground hover:bg-muted hover:text-foreground shrink-0">
          <ArrowLeft className="h-3.5 w-3.5" />
        </button>
        <span className="text-2sm font-medium">Mailboxes</span>
      </div>

      {/* Mode tabs */}
      <div className="flex gap-1 px-3 py-2 border-b border-border shrink-0">
        {(['dedicated', 'shared'] as const).map(t => (
          <button key={t} onClick={() => setTab(t)}
            className={cn('flex-1 rounded-md py-1.5 text-xs font-medium transition-colors',
              tab === t ? 'bg-primary/10 text-primary' : 'text-muted-foreground hover:bg-muted')}>
            {t === 'dedicated' ? 'Dedicated' : 'Shared'}
          </button>
        ))}
      </div>

      <div className="flex-1 overflow-y-auto">
        {loading ? (
          <div className="flex items-center gap-2 p-4 text-xs text-muted-foreground">
            <Loader2 className="h-3.5 w-3.5 animate-spin" /> Loading…
          </div>
        ) : tab === 'dedicated' ? (
          <DedicatedPanel identities={dedicated} souls={souls} editingId={editingId} setEditingId={setEditingId} onRefresh={refresh} />
        ) : (
          <SharedPanel shared={shared} aliases={aliases} souls={souls} onRefresh={refresh} />
        )}
      </div>
    </div>
  );
}

function DedicatedPanel({ identities, souls, editingId, setEditingId, onRefresh }: {
  identities: MailIdentity[]; souls: any[];
  editingId: string | null; setEditingId: (id: string | null) => void;
  onRefresh: () => void;
}) {
  const [creating, setCreating] = useState(false);
  const [form, setForm] = useState({ agent_id: '', address: '', display_name: '' });
  const [busy, setBusy] = useState(false);

  const create = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    try {
      await mailApi.createIdentity({ ...form, agent_id: form.agent_id || undefined, identity_type: 'dedicated' });
      toast.success('Mailbox created');
      setCreating(false);
      setForm({ agent_id: '', address: '', display_name: '' });
      onRefresh();
    } catch { toast.error('Create failed'); } finally { setBusy(false); }
  };

  return (
    <div className="p-3 space-y-2">
      <p className="text-xs text-muted-foreground px-1">Each Qor has its own SMTP/IMAP credentials.</p>
      {identities.length === 0 && !creating && (
        <p className="py-4 text-center text-xs text-muted-foreground">No dedicated mailboxes yet.</p>
      )}
      {identities.map(id =>
        editingId === id.id
          ? <IdentityEditInline key={id.id} identity={id} onSaved={() => { setEditingId(null); onRefresh(); }} onCancel={() => setEditingId(null)} />
          : (
            <div key={id.id} className={cn('rounded-lg border border-border p-2.5 text-xs', !id.is_active && 'opacity-60')}>
              <div className="flex items-center gap-2">
                <Mail className="h-3.5 w-3.5 text-primary shrink-0" />
                <span className="font-medium truncate flex-1">{id.display_name}</span>
                <button onClick={() => setEditingId(id.id)}
                  className="h-5 w-5 flex items-center justify-center rounded text-muted-foreground hover:bg-muted hover:text-foreground shrink-0">
                  <Settings className="h-3 w-3" />
                </button>
              </div>
              <p className="text-muted-foreground truncate mt-0.5">{id.address}</p>
              <div className="mt-1 flex gap-3 text-muted-foreground">
                <span className={id.smtp_host ? '' : 'text-amber-500'}>SMTP {id.smtp_host ? '✓' : '–'}</span>
                <span className={id.imap_host ? '' : 'text-amber-500'}>IMAP {id.imap_host ? '✓' : '–'}</span>
              </div>
            </div>
          )
      )}
      {creating ? (
        <form onSubmit={create} className="rounded-lg border border-primary/30 bg-primary/5 p-3 space-y-2 text-xs">
          <p className="font-medium text-primary">New mailbox</p>
          <div>
            <label className="block text-muted-foreground mb-0.5">Bind to Qor</label>
            <select value={form.agent_id} onChange={e => setForm(p => ({ ...p, agent_id: e.target.value }))}
              className="qr-input text-xs">
              <option value="">— unbound —</option>
              {souls.map(s => <option key={s.id} value={s.id}>{s.display_name}</option>)}
            </select>
          </div>
          <div>
            <label className="block text-muted-foreground mb-0.5">Email address</label>
            <input type="email" value={form.address} onChange={e => setForm(p => ({ ...p, address: e.target.value }))} required
              placeholder="qor@yourdomain.com"
              className="qr-input text-xs" />
          </div>
          <div>
            <label className="block text-muted-foreground mb-0.5">Display name</label>
            <input value={form.display_name} onChange={e => setForm(p => ({ ...p, display_name: e.target.value }))} required
              placeholder="Sara from Support"
              className="qr-input text-xs" />
          </div>
          <div className="flex gap-2 pt-1">
            <button type="submit" disabled={busy}
              className="flex items-center gap-1 rounded-md bg-primary text-primary-foreground px-3 py-1.5 text-xs font-medium hover:bg-primary/90 disabled:opacity-50">
              {busy ? <Loader2 className="h-3 w-3 animate-spin" /> : <Check className="h-3 w-3" />} Create
            </button>
            <button type="button" onClick={() => setCreating(false)} className="text-xs text-muted-foreground hover:text-foreground">cancel</button>
          </div>
        </form>
      ) : (
        <button onClick={() => setCreating(true)}
          className="flex w-full items-center gap-2 rounded-md border border-dashed border-border px-3 py-2 text-xs text-muted-foreground hover:border-primary/40 hover:text-foreground">
          <Plus className="h-3.5 w-3.5" /> New mailbox
        </button>
      )}
    </div>
  );
}

function IdentityEditInline({ identity, onSaved, onCancel }: {
  identity: MailIdentity; onSaved: () => void; onCancel: () => void;
}) {
  const [form, setForm] = useState({
    display_name: identity.display_name,
    smtp_host: identity.smtp_host ?? '',
    smtp_port: identity.smtp_port ?? 465,
    smtp_user: identity.smtp_user ?? '',
    smtp_pass: '',
    imap_host: identity.imap_host ?? '',
    imap_port: identity.imap_port ?? 993,
    imap_user: identity.imap_user ?? '',
    imap_pass: '',
    poll_interval_seconds: identity.poll_interval_seconds ?? 60,
  });
  const [saving, setSaving] = useState(false);
  const f = (k: string) => (e: React.ChangeEvent<HTMLInputElement>) => setForm(p => ({ ...p, [k]: e.target.value }));
  const n = (k: string) => (e: React.ChangeEvent<HTMLInputElement>) => setForm(p => ({ ...p, [k]: parseInt(e.target.value, 10) || 0 }));

  const save = async () => {
    setSaving(true);
    await mailApi.updateIdentity(identity.id, form as any).then(() => { toast.success('Saved'); onSaved(); }).catch(() => toast.error('Save failed'));
    setSaving(false);
  };

  return (
    <div className="rounded-lg border border-primary/30 bg-primary/5 p-3 space-y-2.5 text-xs">
      <div className="flex items-center justify-between">
        <span className="font-medium text-primary truncate">{identity.address}</span>
        <button onClick={onCancel} className="h-5 w-5 flex items-center justify-center rounded text-muted-foreground hover:bg-muted"><ArrowLeft className="h-3 w-3" /></button>
      </div>
      <div>
        <label className="block text-muted-foreground mb-0.5">Display name</label>
        <input value={form.display_name} onChange={f('display_name')} className="qr-input text-xs" />
      </div>
      <p className="text-xs font-medium text-muted-foreground pt-1">IMAP</p>
      <div className="grid grid-cols-3 gap-1.5">
        <div className="col-span-2">
          <input value={form.imap_host} onChange={f('imap_host')} placeholder="imap.host" className="qr-input text-xs" />
        </div>
        <input type="number" value={form.imap_port} onChange={n('imap_port')} className="qr-input text-xs" />
      </div>
      <input value={form.imap_user} onChange={f('imap_user')} placeholder="username" className="qr-input text-xs" />
      <input type="password" value={form.imap_pass} onChange={f('imap_pass')} placeholder="password (blank = keep)" className="qr-input text-xs" />
      <div>
        <label className="block text-muted-foreground mb-0.5">Poll interval</label>
        <SearchableSelect value={String(form.poll_interval_seconds)} onChange={v => setForm(p => ({ ...p, poll_interval_seconds: parseInt(v, 10) }))} options={POLL_OPTIONS} />
      </div>
      <p className="text-xs font-medium text-muted-foreground pt-1">SMTP</p>
      <div className="grid grid-cols-3 gap-1.5">
        <div className="col-span-2">
          <input value={form.smtp_host} onChange={f('smtp_host')} placeholder="smtp.host" className="qr-input text-xs" />
        </div>
        <input type="number" value={form.smtp_port} onChange={n('smtp_port')} className="qr-input text-xs" />
      </div>
      <input value={form.smtp_user} onChange={f('smtp_user')} placeholder="username" className="qr-input text-xs" />
      <input type="password" value={form.smtp_pass} onChange={f('smtp_pass')} placeholder="password (blank = keep)" className="qr-input text-xs" />
      <button onClick={save} disabled={saving}
        className="flex items-center gap-1 rounded-md bg-primary text-primary-foreground px-3 py-1.5 text-xs font-medium hover:bg-primary/90 disabled:opacity-50">
        {saving ? <Loader2 className="h-3 w-3 animate-spin" /> : <Save className="h-3 w-3" />} Save
      </button>
    </div>
  );
}

function SharedPanel({ shared, aliases, souls, onRefresh }: {
  shared: MailIdentity[]; aliases: MailAlias[]; souls: any[]; onRefresh: () => void;
}) {
  const [creatingId, setCreatingId] = useState(false);
  const [creatingAlias, setCreatingAlias] = useState(false);
  const [idForm, setIdForm] = useState({ address: '', display_name: '' });
  const [aliasForm, setAliasForm] = useState({ alias_address: '', target_agent_id: '', can_send_as: true, can_receive: true });
  const [busy, setBusy] = useState(false);

  const createId = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true);
    try { await mailApi.createIdentity({ ...idForm, identity_type: 'shared' }); toast.success('Created'); setCreatingId(false); setIdForm({ address: '', display_name: '' }); onRefresh(); }
    catch { toast.error('Failed'); } finally { setBusy(false); }
  };
  const createAlias = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true);
    try { await mailApi.createAlias(aliasForm); toast.success('Alias added'); setCreatingAlias(false); setAliasForm({ alias_address: '', target_agent_id: '', can_send_as: true, can_receive: true }); onRefresh(); }
    catch { toast.error('Failed'); } finally { setBusy(false); }
  };

  return (
    <div className="p-3 space-y-4">
      <div>
        <SidebarGroupTitle>Shared mailbox</SidebarGroupTitle>
        <div className="px-1 space-y-1.5 mt-1">
          {shared.map(id => (
            <div key={id.id} className="rounded-lg border border-border p-2.5 text-xs">
              <div className="flex items-center gap-2"><Mail className="h-3.5 w-3.5 text-primary shrink-0" /><span className="font-medium truncate">{id.display_name}</span></div>
              <p className="text-muted-foreground truncate">{id.address}</p>
            </div>
          ))}
          {creatingId ? (
            <form onSubmit={createId} className="rounded-lg border border-primary/30 bg-primary/5 p-3 space-y-2 text-xs">
              <input type="email" value={idForm.address} onChange={e => setIdForm(p => ({ ...p, address: e.target.value }))} required placeholder="support@yourdomain.com"
                className="qr-input text-xs" />
              <input value={idForm.display_name} onChange={e => setIdForm(p => ({ ...p, display_name: e.target.value }))} required placeholder="Support Team"
                className="qr-input text-xs" />
              <div className="flex gap-2">
                <button type="submit" disabled={busy} className="flex items-center gap-1 rounded-md bg-primary text-primary-foreground px-3 py-1.5 text-xs font-medium hover:bg-primary/90 disabled:opacity-50">
                  {busy ? <Loader2 className="h-3 w-3 animate-spin" /> : <Check className="h-3 w-3" />} Create
                </button>
                <button type="button" onClick={() => setCreatingId(false)} className="text-xs text-muted-foreground hover:text-foreground">cancel</button>
              </div>
            </form>
          ) : (
            <button onClick={() => setCreatingId(true)}
              className="flex w-full items-center gap-2 rounded-md border border-dashed border-border px-3 py-2 text-xs text-muted-foreground hover:border-primary/40 hover:text-foreground">
              <Plus className="h-3.5 w-3.5" /> Create shared mailbox
            </button>
          )}
        </div>
      </div>

      <div>
        <SidebarGroupTitle>Agent aliases</SidebarGroupTitle>
        <div className="px-1 space-y-1.5 mt-1">
          {aliases.map(a => {
            const soul = souls.find(s => s.id === a.target_agent_id);
            return (
              <div key={a.id} className="flex items-center gap-2 rounded-lg border border-border px-2.5 py-2 text-xs">
                <AtSign className="h-3 w-3 text-muted-foreground shrink-0" />
                <span className="flex-1 truncate font-medium">{a.alias_address}</span>
                <span className="text-muted-foreground shrink-0 truncate">{soul?.display_name ?? '–'}</span>
                <button onClick={() => mailApi.deleteAlias(a.id).then(onRefresh).catch(() => {})}
                  className="h-5 w-5 flex items-center justify-center rounded text-muted-foreground hover:text-destructive hover:bg-destructive/10 shrink-0">
                  <Trash2 className="h-3 w-3" />
                </button>
              </div>
            );
          })}
          {creatingAlias ? (
            <form onSubmit={createAlias} className="rounded-lg border border-primary/30 bg-primary/5 p-3 space-y-2 text-xs">
              <input type="email" value={aliasForm.alias_address} onChange={e => setAliasForm(p => ({ ...p, alias_address: e.target.value }))} required placeholder="sara@yourdomain.com"
                className="qr-input text-xs" />
              <select value={aliasForm.target_agent_id} onChange={e => setAliasForm(p => ({ ...p, target_agent_id: e.target.value }))} required
                className="qr-input text-xs">
                <option value="">— select Qor —</option>
                {souls.map(s => <option key={s.id} value={s.id}>{s.display_name}</option>)}
              </select>
              <div className="flex gap-3">
                <label className="flex items-center gap-1.5 cursor-pointer"><input type="checkbox" checked={aliasForm.can_receive} onChange={e => setAliasForm(p => ({ ...p, can_receive: e.target.checked }))} /> Receives</label>
                <label className="flex items-center gap-1.5 cursor-pointer"><input type="checkbox" checked={aliasForm.can_send_as} onChange={e => setAliasForm(p => ({ ...p, can_send_as: e.target.checked }))} /> Sends as</label>
              </div>
              <div className="flex gap-2">
                <button type="submit" disabled={busy || !aliasForm.target_agent_id}
                  className="flex items-center gap-1 rounded-md bg-primary text-primary-foreground px-3 py-1.5 text-xs font-medium hover:bg-primary/90 disabled:opacity-50">
                  {busy ? <Loader2 className="h-3 w-3 animate-spin" /> : <Check className="h-3 w-3" />} Add
                </button>
                <button type="button" onClick={() => setCreatingAlias(false)} className="text-xs text-muted-foreground hover:text-foreground">cancel</button>
              </div>
            </form>
          ) : (
            <button onClick={() => setCreatingAlias(true)}
              className="flex w-full items-center gap-2 rounded-md border border-dashed border-border px-3 py-2 text-xs text-muted-foreground hover:border-primary/40 hover:text-foreground">
              <Plus className="h-3.5 w-3.5" /> Add alias
            </button>
          )}
        </div>
      </div>
    </div>
  );
}
