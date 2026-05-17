'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useState } from 'react';
import { X } from 'lucide-react';

export function ComposeSheet({ open, onClose, agentId }: { open: boolean; onClose: () => void; agentId: string }) {
  const [to, setTo] = useState('');
  const [subject, setSubject] = useState('');
  const [body, setBody] = useState('');
  const [sending, setSending] = useState(false);
  const getToken = () => typeof window !== 'undefined' ? (localStorage.getItem('qorven_token') || '') : '';

  if (!open) return null;

  const handleSend = async () => {
    setSending(true);
    try {
      await fetch('/api/v1/mail/send', {
        method: 'POST', headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${getToken()}` },
        body: JSON.stringify({ agent_id: agentId, to: [to], subject, body }),
      });
      onClose(); setTo(''); setSubject(''); setBody('');
    } catch { alert('Failed to send'); }
    setSending(false);
  };

  return (
    <div className="fixed inset-x-0 bottom-0 z-50 bg-background border-t border-border rounded-t-xl shadow-lg p-4 max-w-2xl mx-auto" style={{ maxHeight: '60vh' }}>
      <div className="flex items-center justify-between mb-3">
        <h3 className="text-sm font-semibold">Compose</h3>
        <button onClick={onClose}><X className="h-4 w-4" /></button>
      </div>
      <input value={to} onChange={(e) => setTo(e.target.value)} placeholder="To" className="qr-input mb-2" />
      <input value={subject} onChange={(e) => setSubject(e.target.value)} placeholder="Subject" className="qr-input mb-2" />
      <textarea value={body} onChange={(e) => setBody(e.target.value)} placeholder="Write your message..." rows={5} className="qr-textarea resize-none mb-3" />
      <button onClick={handleSend} disabled={sending || !to} className="qr-btn qr-btn-primary">
        {sending ? 'Sending...' : 'Send (requires approval)'}
      </button>
    </div>
  );
}
