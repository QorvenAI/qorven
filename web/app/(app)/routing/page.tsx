'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useEffect, useState } from 'react';
import { routing, providers as providersApi } from '@/lib/api';
import { useSelectedModels } from '@/hooks/use-selected-models';
import { cn } from '@/lib/utils';
import { modelDisplayName } from '@/lib/model-names';
import { CheckCircle, XCircle, Zap, Plus, X } from 'lucide-react';
import { EmptyState, emptyStates } from '@/components/empty-state';

export default function RoutingPage() {
  const [categories, setCategories] = useState<any[]>([]);
  const [assignments, setAssignments] = useState<Record<string, string[]>>({});
  const [decisions, setDecisions] = useState<any[]>([]);
  const [testQuery, setTestQuery] = useState('');
  const [testResult, setTestResult] = useState<any>(null);
  const { models } = useSelectedModels();

  const refresh = () => {
    routing.categories().then((d) => setCategories(Array.isArray(d) ? d : []));
    routing.assignments().then((d) => setAssignments(d && typeof d === 'object' ? d : {}));
    routing.decisions().then((d) => setDecisions(Array.isArray(d) ? d : []));
  };

  useEffect(() => { refresh(); }, []);

  const handleAssign = async (slug: string, modelId: string) => {
    await routing.assign(slug, modelId);
    refresh();
  };

  const handleUnassign = async (slug: string, modelId: string) => {
    await routing.unassign(slug, modelId);
    refresh();
  };

  const handleTest = async () => {
    if (!testQuery) return;
    const result = await routing.classify(testQuery);
    setTestResult(result);
    refresh();
  };

  // Empty state for fresh install
  // if (rules.length === 0) return <EmptyState {...emptyStates.routing} />;
  return (
    <div className="space-y-8">
      <div>
        <h1 className="text-lg font-semibold">Smart Routing</h1>
        <p className="text-sm text-muted-foreground">Assign models to work categories. Every message is classified and routed automatically.</p>
      </div>

      {/* Test classifier */}
      <div className="qr-card p-4">
        <p className="text-sm font-medium mb-3">Test the Router</p>
        <div className="flex gap-2">
          <input value={testQuery} onChange={(e) => setTestQuery(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && handleTest()}
            placeholder="Type a message to see how it gets classified..."
            className="qr-input flex-1" />
          <button onClick={handleTest} className="flex items-center gap-1.5 rounded-lg bg-primary px-4 py-2 text-xs font-medium text-primary-foreground hover:bg-primary/90">
            <Zap className="h-3.5 w-3.5" /> Classify
          </button>
        </div>
        {testResult && (
          <div className="mt-3 rounded-lg bg-muted p-3 text-sm">
            <span className="font-medium">{testResult.reason}</span>
            {testResult.model_id && <span className="text-muted-foreground"> → {testResult.model_id}</span>}
            {!testResult.model_id && <span className="text-amber-400"> → No model assigned for this category</span>}
          </div>
        )}
      </div>

      {/* Categories + assignments */}
      <div>
        <p className="text-sm font-medium mb-3">Work Categories & Model Assignments</p>
        <div className="grid gap-3 sm:grid-cols-2">
          {categories.map((cat) => {
            const assigned = assignments[cat.slug] || [];
            return (
              <div key={cat.slug} className="qr-card p-4">
                <div className="flex items-center gap-2 mb-3">
                  <span className="text-lg">{cat.icon}</span>
                  <div>
                    <p className="text-sm font-medium">{cat.name}</p>
                    <p className="text-2xs text-muted-foreground">{cat.description}</p>
                  </div>
                </div>

                {/* Assigned models */}
                {assigned.length > 0 ? (
                  <div className="space-y-1 mb-2">
                    {assigned.map((modelId) => (
                      <div key={modelId} className="flex items-center justify-between rounded-lg bg-muted px-2.5 py-1.5">
                        <span className="text-xs font-medium truncate">{modelDisplayName(modelId)}</span>
                        <button onClick={() => handleUnassign(cat.slug, modelId)} className="text-muted-foreground hover:text-destructive">
                          <X className="h-3 w-3" />
                        </button>
                      </div>
                    ))}
                  </div>
                ) : (
                  <p className="text-2xs text-muted-foreground mb-2">No model assigned — will use default</p>
                )}

                {/* Add model dropdown */}
                {models.length > 0 && (
                  <select onChange={(e) => { if (e.target.value) { handleAssign(cat.slug, e.target.value); e.target.value = ''; } }}
                    className="w-full rounded-lg border border-dashed border-border bg-background px-2.5 py-1.5 text-xs text-muted-foreground">
                    <option value="">+ Assign model...</option>
                    {models.filter((m) => !assigned.includes(m.model_id)).map((m) => (
                      <option key={m.model_id} value={m.model_id}>{modelDisplayName(m.model_id)}</option>
                    ))}
                  </select>
                )}
              </div>
            );
          })}
        </div>
      </div>

      {/* Recent decisions */}
      <div>
        <p className="text-sm font-medium mb-3">Recent Routing Decisions</p>
        {decisions.length === 0 ? (
          <p className="text-sm text-muted-foreground">No routing decisions yet — send a message to see them here</p>
        ) : (
          <div className="rounded-xl border border-border divide-y divide-border">
            {decisions.slice(0, 20).map((d: any) => (
              <div key={d.id} className="flex items-center gap-3 px-4 py-2.5">
                <span className={cn('h-1.5 w-1.5 rounded-full', d.was_correct ? 'bg-emerald-400' : 'bg-amber-400')} />
                <div className="flex-1 min-w-0">
                  <p className="text-xs truncate">{d.query_preview}</p>
                  <p className="text-2xs text-muted-foreground">
                    {categories.find((c) => c.slug === d.category)?.icon} {d.category} ({Math.round(d.confidence * 100)}%) → {modelDisplayName(d.model) || 'no model'}
                    {d.override_model && <span className="text-amber-400"> → corrected to {modelDisplayName(d.override_model)}</span>}
                  </p>
                </div>
                {d.was_correct ? (
                  <CheckCircle className="h-3.5 w-3.5 text-emerald-400 shrink-0" />
                ) : (
                  <XCircle className="h-3.5 w-3.5 text-amber-400 shrink-0" />
                )}
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
