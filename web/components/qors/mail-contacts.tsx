'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useState, useEffect } from 'react';
import { Plus, Search, X, ChevronRight, Save } from 'lucide-react';
import { contacts, type Contact, type ContactAgentPrefs } from '@/lib/api-agents';
import { SearchableSelect } from '@/components/searchable-select';
import { cn } from '@/lib/utils';
import { toast } from 'sonner';

const TRUST_OPTS = [
  { value: 'unknown',  label: 'Unknown' },
  { value: 'known',    label: 'Known' },
  { value: 'trusted',  label: 'Trusted' },
  { value: 'blocked',  label: 'Blocked' },
];

const ROUTING_OPTS = [
  { value: 'inherit', label: 'Inherit policy' },
  { value: 'auto',    label: 'Auto-reply' },
  { value: 'draft',   label: 'Draft & hold' },
  { value: 'skip',    label: 'Skip (ignore)' },
];

const TRUST_COLORS: Record<string, string> = {
  unknown:  'bg-muted/50 text-muted-foreground',
  known:    'bg-blue-500/10 text-blue-600',
  trusted:  'bg-emerald-500/10 text-emerald-600',
  blocked:  'bg-destructive/10 text-destructive',
};

function initials(name: string, email: string) {
  const src = name || email;
  return src.slice(0, 2).toUpperCase();
}

function ContactDetail({ contact: c, agentId, onClose }: { contact: Contact; agentId: string; onClose: () => void }) {
  const [prefs, setPrefs] = useState<ContactAgentPrefs>({
    contact_id: c.id, agent_id: agentId,
    routing_mode: 'inherit', trust_level: 'unknown', agent_notes: '',
  });
  const [edit, setEdit] = useState<Partial<Contact>>({ display_name: c.display_name, company: c.company, notes: c.notes });
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    contacts.getPrefs(c.id, agentId)
      .then(setPrefs)
      .catch(() => {});
  }, [c.id, agentId]);

  const save = async () => {
    setSaving(true);
    try {
      await Promise.all([
        contacts.patch(c.id, { display_name: edit.display_name, company: edit.company, notes: edit.notes }),
        contacts.putPrefs(c.id, agentId, {
          routing_mode: prefs.routing_mode,
          trust_level: prefs.trust_level,
          agent_notes: prefs.agent_notes,
        }),
      ]);
      toast.success('Contact updated');
    } catch {
      toast.error('Failed to save');
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="flex h-full flex-col">
      <div className="flex items-center gap-2 border-b border-border px-4 py-3">
        <button onClick={onClose} className="rounded p-1 hover:bg-accent">
          <ChevronRight className="h-4 w-4 rotate-180" />
        </button>
        <div className="flex h-8 w-8 items-center justify-center rounded-full bg-primary/20 text-xs font-bold text-primary">
          {initials(c.display_name, c.external_id)}
        </div>
        <div>
          <p className="text-sm font-medium">{c.display_name || c.external_id}</p>
          <p className="text-xs text-muted-foreground">{c.email || c.external_id}</p>
        </div>
      </div>
      <div className="flex-1 overflow-y-auto p-4 space-y-5">
        <section className="rounded-xl border border-border p-4 space-y-3">
          <h4 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">Contact Info</h4>
          <label className="block">
            <span className="text-xs text-muted-foreground">Display name</span>
            <input
              value={edit.display_name ?? ''}
              onChange={(e) => setEdit((p) => ({ ...p, display_name: e.target.value }))}
              className="mt-1 block w-full rounded-lg border border-border bg-background px-3 py-1.5 text-sm"
            />
          </label>
          <label className="block">
            <span className="text-xs text-muted-foreground">Company</span>
            <input
              value={edit.company ?? ''}
              onChange={(e) => setEdit((p) => ({ ...p, company: e.target.value }))}
              className="mt-1 block w-full rounded-lg border border-border bg-background px-3 py-1.5 text-sm"
            />
          </label>
          <label className="block">
            <span className="text-xs text-muted-foreground">Notes</span>
            <textarea
              value={edit.notes ?? ''}
              onChange={(e) => setEdit((p) => ({ ...p, notes: e.target.value }))}
              rows={3}
              className="mt-1 block w-full rounded-lg border border-border bg-background px-3 py-1.5 text-sm resize-none"
            />
          </label>
        </section>

        <section className="rounded-xl border border-border p-4 space-y-3">
          <h4 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">This Agent&apos;s Preferences</h4>
          <label className="block">
            <span className="text-xs text-muted-foreground">Trust level</span>
            <div className="mt-1">
              <SearchableSelect
                value={prefs.trust_level}
                onChange={(v) => setPrefs((p) => ({ ...p, trust_level: v as ContactAgentPrefs['trust_level'] }))}
                options={TRUST_OPTS}
              />
            </div>
          </label>
          <label className="block">
            <span className="text-xs text-muted-foreground">Routing override</span>
            <div className="mt-1">
              <SearchableSelect
                value={prefs.routing_mode}
                onChange={(v) => setPrefs((p) => ({ ...p, routing_mode: v as ContactAgentPrefs['routing_mode'] }))}
                options={ROUTING_OPTS}
              />
            </div>
          </label>
          <label className="block">
            <span className="text-xs text-muted-foreground">Private agent notes (not shared)</span>
            <textarea
              value={prefs.agent_notes}
              onChange={(e) => setPrefs((p) => ({ ...p, agent_notes: e.target.value }))}
              rows={3}
              className="mt-1 block w-full rounded-lg border border-border bg-background px-3 py-1.5 text-sm resize-none"
            />
          </label>
        </section>

        <div className="flex gap-2 pb-4">
          <button
            onClick={save}
            disabled={saving}
            className="flex items-center gap-2 rounded-lg bg-primary px-4 py-2 text-sm text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
          >
            <Save className="h-3.5 w-3.5" />
            {saving ? 'Saving…' : 'Save'}
          </button>
          <button
            onClick={onClose}
            className="rounded-lg border border-border px-4 py-2 text-sm hover:bg-accent"
          >
            Cancel
          </button>
        </div>
      </div>
    </div>
  );
}

export function MailContacts({ agentId }: { agentId: string }) {
  const [list, setList] = useState<Contact[]>([]);
  const [search, setSearch] = useState('');
  const [trustFilter, setTrustFilter] = useState('');
  const [selected, setSelected] = useState<Contact | null>(null);
  const [showAdd, setShowAdd] = useState(false);
  const [newContact, setNewContact] = useState({ external_id: '', display_name: '', company: '' });
  const [adding, setAdding] = useState(false);

  useEffect(() => {
    contacts.list({ search: search || undefined }).then(setList).catch(() => {});
  }, [search]);

  const addContact = async () => {
    if (!newContact.external_id) { toast.error('Email address required'); return; }
    setAdding(true);
    try {
      await contacts.create({ ...newContact, channel: 'email' });
      const fresh = await contacts.list();
      setList(fresh);
      setShowAdd(false);
      setNewContact({ external_id: '', display_name: '', company: '' });
      toast.success('Contact added');
    } catch {
      toast.error('Failed to add contact');
    } finally {
      setAdding(false);
    }
  };

  if (selected) {
    return <ContactDetail contact={selected} agentId={agentId} onClose={() => setSelected(null)} />;
  }

  const filtered = trustFilter
    ? list.filter((c) => (c as any).trust_level === trustFilter)
    : list;

  return (
    <div className="flex h-full flex-col">
      <div className="flex items-center gap-2 border-b border-border px-4 py-3">
        <div className="relative flex-1">
          <Search className="absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search contacts…"
            className="w-full rounded-lg border border-border bg-background py-1.5 pl-8 pr-3 text-sm"
          />
          {search && (
            <button onClick={() => setSearch('')} className="absolute right-2.5 top-1/2 -translate-y-1/2">
              <X className="h-3 w-3 text-muted-foreground" />
            </button>
          )}
        </div>
        <div className="w-36">
          <SearchableSelect
            value={trustFilter}
            onChange={setTrustFilter}
            options={[{ value: '', label: 'All trust levels' }, ...TRUST_OPTS]}
          />
        </div>
        <button
          onClick={() => setShowAdd(true)}
          className="flex items-center gap-1 rounded-lg bg-primary px-3 py-1.5 text-xs text-primary-foreground hover:bg-primary/90"
        >
          <Plus className="h-3.5 w-3.5" /> Add
        </button>
      </div>

      {showAdd && (
        <div className="border-b border-border bg-card px-4 py-3 space-y-2">
          <p className="text-xs font-semibold">New Contact</p>
          <div className="grid grid-cols-3 gap-2">
            <input
              placeholder="Email address *"
              value={newContact.external_id}
              onChange={(e) => setNewContact((p) => ({ ...p, external_id: e.target.value }))}
              className="col-span-3 sm:col-span-1 rounded-lg border border-border bg-background px-3 py-1.5 text-sm"
            />
            <input
              placeholder="Display name"
              value={newContact.display_name}
              onChange={(e) => setNewContact((p) => ({ ...p, display_name: e.target.value }))}
              className="rounded-lg border border-border bg-background px-3 py-1.5 text-sm"
            />
            <input
              placeholder="Company"
              value={newContact.company}
              onChange={(e) => setNewContact((p) => ({ ...p, company: e.target.value }))}
              className="rounded-lg border border-border bg-background px-3 py-1.5 text-sm"
            />
          </div>
          <div className="flex gap-2">
            <button onClick={addContact} disabled={adding} className="rounded-lg bg-primary px-3 py-1 text-xs text-primary-foreground hover:bg-primary/90 disabled:opacity-50">
              {adding ? 'Adding…' : 'Add Contact'}
            </button>
            <button onClick={() => setShowAdd(false)} className="rounded-lg border border-border px-3 py-1 text-xs hover:bg-accent">
              Cancel
            </button>
          </div>
        </div>
      )}

      {filtered.length === 0 ? (
        <div className="flex flex-1 flex-col items-center justify-center text-sm text-muted-foreground">
          No contacts yet. They appear automatically when emails arrive.
        </div>
      ) : (
        <div className="divide-y divide-border overflow-y-auto">
          {filtered.map((c) => (
            <button
              key={c.id}
              onClick={() => setSelected(c)}
              className="flex w-full items-center gap-3 px-4 py-3 text-left hover:bg-accent/50 transition-colors"
            >
              <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-primary/20 text-[11px] font-bold text-primary">
                {initials(c.display_name, c.external_id)}
              </div>
              <div className="flex-1 min-w-0">
                <p className="truncate text-sm font-medium">{c.display_name || c.external_id}</p>
                <p className="truncate text-xs text-muted-foreground">{c.email || c.external_id}{c.company ? ` · ${c.company}` : ''}</p>
              </div>
              <div className="flex items-center gap-2">
                <span className={cn('rounded-full px-2 py-0.5 text-[10px] font-medium', TRUST_COLORS[(c as any).trust_level ?? 'unknown'])}>
                  {(c as any).trust_level ?? 'unknown'}
                </span>
                <span className="text-[11px] text-muted-foreground">{c.message_count} msg</span>
              </div>
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
