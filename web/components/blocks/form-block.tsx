'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useState } from 'react';

interface FormProps {
  title?: string;
  fields: { name: string; label: string; type: 'text' | 'email' | 'number' | 'select' | 'textarea' | 'date' | 'toggle'; options?: string[]; required?: boolean; placeholder?: string }[];
  submitLabel?: string;
}

export function DynamicForm({ title, fields, submitLabel = 'Submit' }: FormProps) {
  const [values, setValues] = useState<Record<string, any>>({});
  const set = (name: string, value: any) => setValues(v => ({ ...v, [name]: value }));

  return (
    <div className="rounded-lg border border-border p-4">
      {title && <h3 className="text-sm font-medium mb-3">{title}</h3>}
      <div className="space-y-3">
        {fields.map(f => (
          <div key={f.name}>
            <label className="text-xs font-medium text-muted-foreground mb-1 block">{f.label}{f.required && <span className="text-red-400"> *</span>}</label>
            {f.type === 'textarea' ? (
              <textarea value={values[f.name] || ''} onChange={e => set(f.name, e.target.value)} placeholder={f.placeholder} rows={3} className="qr-textarea" />
            ) : f.type === 'select' ? (
              <select value={values[f.name] || ''} onChange={e => set(f.name, e.target.value)} className="qr-select">
                <option value="">Select...</option>
                {f.options?.map(o => <option key={o} value={o}>{o}</option>)}
              </select>
            ) : f.type === 'toggle' ? (
              <button onClick={() => set(f.name, !values[f.name])} className={`h-6 w-11 rounded-full transition-colors ${values[f.name] ? 'bg-primary' : 'bg-muted'}`}>
                <div className={`h-5 w-5 rounded-full bg-background shadow-sm transition-transform ${values[f.name] ? 'translate-x-5' : 'translate-x-0.5'}`} />
              </button>
            ) : (
              <input type={f.type} value={values[f.name] || ''} onChange={e => set(f.name, e.target.value)} placeholder={f.placeholder} className="qr-input" />
            )}
          </div>
        ))}
        <button className="qr-btn qr-btn-primary qr-btn-lg w-full">{submitLabel}</button>
      </div>
    </div>
  );
}
