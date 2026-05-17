'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useState, useEffect } from 'react';
import { useRouter } from 'next/navigation';
import { Loader2, CheckCircle, ArrowRight, Sparkles, Search, Users, Zap } from 'lucide-react';
import { cn } from '@/lib/utils';
import { EmptyState, emptyStates } from '@/components/empty-state';
import { request } from '@/lib/api-core';

type Template = { id: string; name: string; description: string; icon: string; category: string; agents: { key: string; name: string; role: string }[]; skills: string[]; connectors: string[] };

const CATEGORY_COLORS: Record<string, string> = {
  business: 'from-blue-500/20 to-blue-600/5 border-blue-500/20',
  marketing: 'from-pink-500/20 to-pink-600/5 border-pink-500/20',
  knowledge: 'from-purple-500/20 to-purple-600/5 border-purple-500/20',
  finance: 'from-emerald-500/20 to-emerald-600/5 border-emerald-500/20',
  engineering: 'from-orange-500/20 to-orange-600/5 border-orange-500/20',
};

export default function MarketplacePage() {
  const [templates, setTemplates] = useState<Template[]>([]);
  const [installed, setInstalled] = useState<string[]>([]);
  const [installing, setInstalling] = useState<string | null>(null);
  const [search, setSearch] = useState('');
  const [showSelfBuild, setShowSelfBuild] = useState(false);
  const router = useRouter();

  useEffect(() => {
    request<any>('/templates').then(d => setTemplates(Array.isArray(d) ? d : d.templates || [])).catch(() => {});
    request<any>('/templates/installed').then(d => { const list = Array.isArray(d) ? d : d.installed || []; setInstalled(list.map((w: any) => w.template_id || w.id)); }).catch(() => {});
  }, []);

  const install = async (id: string) => {
    setInstalling(id);
    const result = await request<any>('/templates/install', { method: 'POST', body: JSON.stringify({ template_id: id }) });
    setInstalling(null);
    if (result?.dashboard_id) {
      setInstalled(prev => [...prev, id]);
      router.push(`/dashboard/${result.dashboard_id}`);
    } else {
      setInstalled(prev => [...prev, id]);
    }
  };

  const filtered = templates.filter(t => !search || t.name.toLowerCase().includes(search.toLowerCase()) || t.description.toLowerCase().includes(search.toLowerCase()));

  return (
    <div className="p-6 max-w-5xl mx-auto space-y-6">
      <div className="text-center space-y-2">
        <h1 className="text-lg font-semibold">Blueprints</h1>
        <p className="text-sm text-muted-foreground max-w-lg mx-auto">Pick a Blueprint and deploy a full AI team with dashboard in 30 seconds. Or describe what you need and let AI build it for you.</p>
      </div>

      <div className="flex gap-3 justify-center">
        <div className="relative flex-1 max-w-md">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
          <input placeholder="Search Blueprints..." value={search} onChange={e => setSearch(e.target.value)}
            className="w-full rounded-xl border border-input bg-transparent pl-9 pr-3 py-2.5 text-sm" />
        </div>
        <button onClick={() => setShowSelfBuild(true)}
          className="inline-flex items-center gap-2 rounded-xl bg-gradient-to-r from-primary to-primary/80 px-5 py-2.5 text-sm font-medium text-white hover:from-primary hover:to-primary/70 cursor-pointer">
          <Sparkles className="h-4 w-4" />Build Custom AI Office
        </button>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {filtered.map(t => {
          const isInstalled = installed.includes(t.id);
          const isInstalling = installing === t.id;
          const colors = CATEGORY_COLORS[t.category] || 'from-gray-500/20 to-gray-600/5 border-gray-500/20';
          return (
            <div key={t.id} className={cn('rounded-xl border bg-gradient-to-br p-5 space-y-3 transition-all hover:shadow-lg', colors)}>
              <div className="flex items-start justify-between">
                <div className="text-2xl">{t.icon}</div>
                <span className="rounded-full bg-muted/50 px-2 py-0.5 text-2xs text-muted-foreground">{t.category}</span>
              </div>
              <div>
                <h3 className="font-semibold">{t.name}</h3>
                <p className="text-xs text-muted-foreground mt-1 line-clamp-2">{t.description}</p>
              </div>
              <div className="flex items-center gap-2 text-2xs text-muted-foreground">
                <Users className="h-3 w-3" />{t.agents?.length || 0} agents
                <Zap className="h-3 w-3 ml-2" />{t.skills?.length || 0} skills
              </div>
              <div className="flex flex-wrap gap-1">
                {t.agents?.slice(0, 3).map(a => (
                  <span key={a.key} className="rounded bg-background/50 px-1.5 py-0.5 text-2xs">{a.name}</span>
                ))}
              </div>
              {isInstalled ? (
                <div className="flex items-center gap-2 text-emerald-500 text-xs font-medium">
                  <CheckCircle className="h-4 w-4" />Deployed
                </div>
              ) : (
                <button onClick={() => install(t.id)} disabled={isInstalling}
                  className="w-full flex items-center justify-center gap-2 rounded-lg bg-primary py-2 text-xs font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50 cursor-pointer">
                  {isInstalling ? <><Loader2 className="h-3.5 w-3.5 animate-spin" />Deploying...</> : <><ArrowRight className="h-3.5 w-3.5" />Deploy</>}
                </button>
              )}
            </div>
          );
        })}
      </div>

      {showSelfBuild && <SelfBuildModal onClose={() => setShowSelfBuild(false)} />}
    </div>
  );
}

function SelfBuildModal({ onClose }: { onClose: () => void }) {
  const [prompt, setPrompt] = useState('');
  const [building, setBuilding] = useState(false);
  const [result, setResult] = useState<string | null>(null);
  const router = useRouter();

  const build = async () => {
    setBuilding(true);
    try {
      const r = await request<any>('/templates/self-build', { method: 'POST', body: JSON.stringify({ description: prompt }) });
      if (r?.dashboard_id) {
        setResult('success');
        setTimeout(() => router.push(`/dashboard/${r.dashboard_id}`), 1500);
      } else {
        setResult(r?.message || 'Building your AI office...');
      }
    } catch {
      setResult('error');
    }
    setBuilding(false);
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60" onClick={onClose}>
      <div className="w-full max-w-lg rounded-2xl border border-border bg-card p-6 space-y-4" onClick={e => e.stopPropagation()}>
        <div className="text-center">
          <Sparkles className="h-8 w-8 mx-auto text-primary/70 mb-2" />
          <h2 className="text-lg font-semibold">Build Your AI Office</h2>
          <p className="text-xs text-muted-foreground mt-1">Describe what you need. Qorven will create agents, assign roles, build a dashboard, and connect services.</p>
        </div>
        <textarea
          placeholder="Example: I run a small e-commerce store on Shopify. I need help with customer support, order tracking, social media posting on Twitter and Instagram, and weekly sales reports sent to my email."
          value={prompt} onChange={e => setPrompt(e.target.value)}
          className="w-full rounded-xl border border-input bg-transparent px-4 py-3 text-sm h-32 resize-none"
          autoFocus
        />
        {result === 'success' && (
          <div className="text-center text-emerald-400 text-sm font-medium">✅ AI Office created! Redirecting to dashboard...</div>
        )}
        {result && result !== 'success' && result !== 'error' && (
          <div className="text-center text-blue-400 text-sm">{result}</div>
        )}
        <div className="flex justify-end gap-2">
          <button onClick={onClose} className="rounded-lg border border-input px-4 py-2 text-xs hover:bg-accent cursor-pointer">Cancel</button>
          <button onClick={build} disabled={!prompt.trim() || building}
            className="rounded-lg bg-gradient-to-r from-primary to-primary/80 px-5 py-2 text-xs font-medium text-white hover:from-primary hover:to-primary/70 disabled:opacity-50 cursor-pointer">
            {building ? <><Loader2 className="inline h-3.5 w-3.5 animate-spin mr-1" />Building...</> : <><Sparkles className="inline h-3.5 w-3.5 mr-1" />Build My AI Office</>}
          </button>
        </div>
      </div>
    </div>
  );
}
