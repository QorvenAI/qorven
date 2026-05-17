'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useRef, useState, useCallback, type KeyboardEvent, type ChangeEvent } from 'react';
import { Paperclip, MonitorUp, Globe, Mic, Send, Square, Loader2, X, MicOff, Brain } from 'lucide-react';
import { cn } from '@/lib/utils';
import { toast } from 'sonner';
import { useVoiceEnabled } from '@/hooks/use-voice-enabled';

interface Attachment {
  name: string;
  type: string;
  url: string;     // data: URL or object URL
  size: number;
}

type ThinkingLevel = 'off' | 'medium' | 'high';

interface ComposerProps {
  input: string;
  isLoading: boolean;
  onInputChange: (v: string) => void;
  onSubmit: (attachments?: Attachment[]) => void;
  onStop: () => void;
  placeholder?: string;
  agentId?: string;
  onTranscript?: (text: string) => void;
  thinkingLevel?: ThinkingLevel;
  onThinkingLevelChange?: (level: ThinkingLevel) => void;
}

const THINKING_LEVELS: { value: ThinkingLevel; label: string; title: string }[] = [
  { value: 'off',    label: 'Think: Off',    title: 'No extended reasoning' },
  { value: 'medium', label: 'Think: Normal', title: 'Balanced reasoning budget' },
  { value: 'high',   label: 'Think: High',   title: 'Maximum reasoning budget' },
];

export function Composer({
  input,
  isLoading,
  onInputChange,
  onSubmit,
  onStop,
  placeholder = 'Message…',
  agentId,
  onTranscript,
  thinkingLevel = 'off',
  onThinkingLevelChange,
}: ComposerProps) {
  const { enabled: voiceEnabled } = useVoiceEnabled();
  const [attachments, setAttachments] = useState<Attachment[]>([]);
  const [micState, setMicState] = useState<'idle' | 'recording' | 'transcribing'>('idle');
  const fileRef = useRef<HTMLInputElement>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const mediaRef = useRef<MediaStream | null>(null);
  const recorderRef = useRef<MediaRecorder | null>(null);
  const chunksRef = useRef<Blob[]>([]);

  const getToken = () =>
    typeof window !== 'undefined' ? (localStorage.getItem('qorven_token') ?? '') : '';

  const resizeTextarea = () => {
    const el = textareaRef.current;
    if (!el) return;
    el.style.height = 'auto';
    el.style.height = `${Math.min(el.scrollHeight, 200)}px`;
  };

  const handleChange = (e: ChangeEvent<HTMLTextAreaElement>) => {
    onInputChange(e.target.value);
    resizeTextarea();
  };

  const handleKey = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      if (!isLoading && (input.trim() || attachments.length)) handleSend();
    }
  };

  const handleSend = () => {
    if (!input.trim() && !attachments.length) return;
    onSubmit(attachments.length ? attachments : undefined);
    setAttachments([]);
    if (textareaRef.current) textareaRef.current.style.height = 'auto';
  };

  const handleFiles = useCallback((files: FileList | null) => {
    if (!files) return;
    Array.from(files).forEach((file) => {
      const reader = new FileReader();
      reader.onload = (e) => {
        const url = e.target?.result as string;
        setAttachments((prev) => [...prev, { name: file.name, type: file.type, url, size: file.size }]);
      };
      reader.readAsDataURL(file);
    });
  }, []);

  const removeAttachment = (i: number) =>
    setAttachments((prev) => prev.filter((_, idx) => idx !== i));

  // STT mic
  const startMic = useCallback(async () => {
    if (!navigator.mediaDevices?.getUserMedia || typeof MediaRecorder === 'undefined') {
      toast.error('Voice input not supported in this browser');
      return;
    }
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
      mediaRef.current = stream;
      const recorder = new MediaRecorder(stream, { mimeType: 'audio/webm;codecs=opus' });
      recorderRef.current = recorder;
      chunksRef.current = [];
      recorder.ondataavailable = (e) => { if (e.data.size > 0) chunksRef.current.push(e.data); };
      recorder.onstop = async () => {
        stream.getTracks().forEach((t) => t.stop());
        if (!chunksRef.current.length) { setMicState('idle'); return; }
        setMicState('transcribing');
        const blob = new Blob(chunksRef.current, { type: 'audio/webm' });
        const fd = new FormData();
        fd.append('file', blob, 'audio.webm');
        try {
          const res = await fetch('/api/transcribe', {
            method: 'POST',
            headers: { Authorization: `Bearer ${getToken()}` },
            body: fd,
          });
          if (!res.ok) {
            toast.error(res.status === 503 ? 'No STT provider configured — check Settings → Voice' : 'Transcription failed');
          } else {
            const data = await res.json();
            if (data.text?.trim()) {
              const newVal = input ? `${input} ${data.text.trim()}` : data.text.trim();
              onInputChange(newVal);
              onTranscript?.(data.text.trim());
            }
          }
        } catch { toast.error('Voice unavailable'); }
        setMicState('idle');
      };
      recorder.start();
      setMicState('recording');
    } catch (err) {
      const name = (err as Error)?.name ?? '';
      toast.error(name === 'NotAllowedError' ? 'Microphone permission denied' : name === 'NotFoundError' ? 'No microphone found' : 'Microphone unavailable');
      setMicState('idle');
    }
  }, [input, onInputChange, onTranscript]);

  const stopMic = useCallback(() => {
    if (recorderRef.current?.state === 'recording') recorderRef.current.stop();
  }, []);

  const toggleMic = () => { if (micState === 'idle') startMic(); else if (micState === 'recording') stopMic(); };

  const canSend = (input.trim().length > 0 || attachments.length > 0) && !isLoading;

  return (
    <div className="border-t border-border bg-background px-6 py-3">
      <div>
        {/* Attachment previews */}
        {attachments.length > 0 && (
          <div className="flex flex-wrap gap-2 mb-2">
            {attachments.map((att, i) => (
              <div key={i} className="flex items-center gap-1.5 rounded-lg border border-border bg-card px-2.5 py-1.5 text-xs">
                <span className="truncate max-w-[120px] text-muted-foreground">{att.name}</span>
                <button onClick={() => removeAttachment(i)} className="text-muted-foreground hover:text-foreground">
                  <X className="h-3 w-3" />
                </button>
              </div>
            ))}
          </div>
        )}

        {/* Feature strip */}
        <div className="flex items-center gap-0.5 mb-1">
          <FeatureButton
            icon={<MonitorUp className="h-3.5 w-3.5" />}
            label="Share screen"
            onClick={() => toast.info('Screen sharing coming soon')}
          />
          <FeatureButton
            icon={<Globe className="h-3.5 w-3.5" />}
            label="Live browser"
            onClick={() => toast.info('Live browser coming soon')}
          />
          <FeatureButton
            icon={!voiceEnabled
              ? <MicOff className="h-3.5 w-3.5" />
              : micState === 'recording'
                ? <MicOff className="h-3.5 w-3.5 text-red-400" />
                : micState === 'transcribing'
                  ? <Loader2 className="h-3.5 w-3.5 animate-spin text-amber-400" />
                  : <Mic className="h-3.5 w-3.5" />}
            label={!voiceEnabled
              ? 'Voice disabled — enable in Settings → Services'
              : micState === 'recording' ? 'Stop recording'
              : micState === 'transcribing' ? 'Transcribing…'
              : 'Voice'}
            onClick={voiceEnabled ? toggleMic : undefined}
            active={micState === 'recording'}
            disabled={!voiceEnabled}
          />
          <FeatureButton
            icon={<Paperclip className="h-3.5 w-3.5" />}
            label="Attach"
            onClick={() => fileRef.current?.click()}
          />
          {onThinkingLevelChange && (
            <button
              type="button"
              title={THINKING_LEVELS.find(t => t.value === thinkingLevel)?.title}
              onClick={() => {
                const idx = THINKING_LEVELS.findIndex(t => t.value === thinkingLevel);
                onThinkingLevelChange(THINKING_LEVELS[(idx + 1) % THINKING_LEVELS.length]!.value);
              }}
              className={cn(
                'flex items-center gap-1 rounded-lg px-2 py-1 text-2xs transition-colors',
                thinkingLevel === 'off'
                  ? 'text-muted-foreground hover:text-foreground hover:bg-accent'
                  : thinkingLevel === 'medium'
                    ? 'text-amber-400 bg-amber-400/10'
                    : 'text-violet-400 bg-violet-400/10',
              )}
            >
              <Brain className="h-3.5 w-3.5" />
              <span className="hidden sm:inline">{THINKING_LEVELS.find(t => t.value === thinkingLevel)?.label}</span>
            </button>
          )}
          <input
            ref={fileRef}
            type="file"
            multiple
            className="hidden"
            onChange={(e) => handleFiles(e.target.files)}
            accept="image/*,application/pdf,text/*,.csv,.json,.md"
          />
        </div>

        {/* Textarea + send — full width, no card wrapper */}
        <div className="flex items-end gap-2">
          <textarea
            ref={textareaRef}
            value={input}
            onChange={handleChange}
            onKeyDown={handleKey}
            placeholder={placeholder}
            rows={1}
            className="flex-1 resize-none bg-transparent text-sm text-foreground placeholder:text-muted-foreground/60 focus:outline-none py-1 min-h-[36px] max-h-[200px]"
          />
          <div className="flex items-center gap-1 pb-0.5">
            {isLoading ? (
              <button
                onClick={onStop}
                className="flex h-8 w-8 items-center justify-center rounded-lg bg-destructive/80 text-destructive-foreground hover:bg-destructive transition-colors"
                title="Stop"
              >
                <Square className="h-3.5 w-3.5 fill-current" />
              </button>
            ) : (
              <button
                onClick={handleSend}
                disabled={!canSend}
                className={cn(
                  'flex h-8 w-8 items-center justify-center rounded-lg transition-colors',
                  canSend
                    ? 'bg-primary text-primary-foreground hover:bg-primary/90'
                    : 'bg-muted text-muted-foreground cursor-not-allowed',
                )}
                title="Send (Enter)"
              >
                <Send className="h-3.5 w-3.5" />
              </button>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

function FeatureButton({
  icon, label, onClick, active, disabled,
}: {
  icon: React.ReactNode;
  label: string;
  onClick?: () => void;
  active?: boolean;
  disabled?: boolean;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      title={label}
      disabled={disabled}
      className={cn(
        'flex items-center gap-1 rounded-lg px-2 py-1 text-2xs transition-colors',
        disabled
          ? 'text-muted-foreground/30 cursor-not-allowed'
          : active
            ? 'text-red-400 bg-red-400/10'
            : 'text-muted-foreground hover:text-foreground hover:bg-accent',
      )}
    >
      {icon}
      <span className="hidden sm:inline">{disabled ? 'Voice off' : label}</span>
    </button>
  );
}
