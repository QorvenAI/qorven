// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

// Global registry of active voice stop functions — keyed by agent ID.
// Every mounted SidebarAgentRow registers its stop function here on mount
// and removes it on unmount. VoiceButtonInline (in composer.tsx) reads
// from here to stop any previously-active agent before starting a new one.
export const agentVoiceRegistry = new Map<string, () => Promise<void>>();
