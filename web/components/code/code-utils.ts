// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).


export const EXT_LANG: Record<string, string> = {
  ts: 'typescript', tsx: 'typescript', js: 'javascript', jsx: 'javascript',
  go: 'go', py: 'python', rs: 'rust', java: 'java', kt: 'kotlin',
  cs: 'csharp', cpp: 'cpp', c: 'c', h: 'c', json: 'json',
  yaml: 'yaml', yml: 'yaml', toml: 'toml', md: 'markdown',
  html: 'html', css: 'css', scss: 'scss', sql: 'sql',
  sh: 'shell', bash: 'shell', dockerfile: 'dockerfile',
};

export const detectLang = (path: string) => {
  const name = (path.split('/').pop() || '').toLowerCase();
  if (name === 'dockerfile') return 'dockerfile';
  return EXT_LANG[name.split('.').pop() || ''] || 'plaintext';
};

export const FILE_COLOR: Record<string, string> = {
  ts: 'text-blue-400', tsx: 'text-blue-400', js: 'text-yellow-400', jsx: 'text-yellow-400',
  go: 'text-cyan-400', py: 'text-emerald-400', rs: 'text-orange-400', java: 'text-red-400',
  json: 'text-amber-400', yaml: 'text-pink-400', yml: 'text-pink-400',
  md: 'text-muted-foreground', css: 'text-purple-400', html: 'text-orange-400',
};

export const fileColor = (name: string) =>
  FILE_COLOR[name.split('.').pop()?.toLowerCase() || ''] || 'text-muted-foreground/60';

export const STACKS = [
  { id: 'nextjs',  label: 'Next.js',  icon: '▲', desc: 'React + TypeScript + Tailwind' },
  { id: 'react',   label: 'React',    icon: '⚛', desc: 'Vite + React + TypeScript' },
  { id: 'go',      label: 'Go',       icon: '🔵', desc: 'Go CLI or HTTP server' },
  { id: 'python',  label: 'Python',   icon: '🐍', desc: 'FastAPI or script' },
  { id: 'node',    label: 'Node.js',  icon: '🟢', desc: 'Express + TypeScript' },
  { id: 'auto',    label: 'Auto',     icon: '✨', desc: 'Agent decides from description' },
];
