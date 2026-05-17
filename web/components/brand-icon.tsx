'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import {
  siTelegram, siGmail, siWhatsapp, siGithub, siNotion, siJira, siStripe,
  siGooglecalendar, siGooglesheets, siDiscord, siSignal, siLine, siZalo,
  siMessenger, siFacebook, siInstagram, siMatrix, siMattermost, siWechat, siApple,
} from 'simple-icons';

const icons: Record<string, { path: string; color: string; title: string }> = {
  telegram:          { path: siTelegram.path,        color: '#26A5E4', title: 'Telegram' },
  gmail:             { path: siGmail.path,            color: '#EA4335', title: 'Gmail' },
  email:             { path: siGmail.path,            color: '#EA4335', title: 'Email' },
  whatsapp:          { path: siWhatsapp.path,         color: '#25D366', title: 'WhatsApp' },
  discord:           { path: siDiscord.path,          color: '#5865F2', title: 'Discord' },
  signal:            { path: siSignal.path,           color: '#3A76F0', title: 'Signal' },
  line:              { path: siLine.path,             color: '#00B900', title: 'LINE' },
  zalo:              { path: siZalo.path,             color: '#0068FF', title: 'Zalo' },
  messenger:         { path: siMessenger.path,        color: '#0099FF', title: 'Messenger' },
  facebook:          { path: siFacebook.path,         color: '#1877F2', title: 'Facebook' },
  instagram:         { path: siInstagram.path,        color: '#E4405F', title: 'Instagram' },
  github:            { path: siGithub.path,           color: '#181717', title: 'GitHub' },
  notion:            { path: siNotion.path,           color: '#000000', title: 'Notion' },
  jira:              { path: siJira.path,             color: '#0052CC', title: 'Jira' },
  stripe:            { path: siStripe.path,           color: '#635BFF', title: 'Stripe' },
  'google-calendar': { path: siGooglecalendar.path,   color: '#4285F4', title: 'Google Calendar' },
  'google-sheets':   { path: siGooglesheets.path,     color: '#34A853', title: 'Google Sheets' },
  matrix:            { path: siMatrix.path,           color: '#000000', title: 'Matrix' },
  mattermost:        { path: siMattermost.path,       color: '#0058CC', title: 'Mattermost' },
  // iMessage — use Apple icon with iMessage blue
  imessage:          { path: siApple.path,            color: '#147EFB', title: 'iMessage' },
  // WeCom uses WeChat icon with WeCom's brand green
  wecom:             { path: siWechat.path,           color: '#07C160', title: 'WeCom' },
  // No simple-icons equivalent — use branded color circle fallback
  slack:             { path: '', color: '#4A154B', title: 'Slack' },
  teams:             { path: '', color: '#6264A7', title: 'Teams' },
  feishu:            { path: '', color: '#1456F0', title: 'Feishu' },
  dingtalk:          { path: '', color: '#FF6200', title: 'DingTalk' },
  sms:               { path: '', color: '#00BFA5', title: 'SMS' },
  webchat:           { path: '', color: '#6852D6', title: 'Webchat' },
  web:               { path: '', color: '#6852D6', title: 'Web' },
  webhook:           { path: '', color: '#F59E0B', title: 'Webhook' },
  weather:           { path: '', color: '#EB6E4B', title: 'Weather' },
  tui:               { path: '', color: '#1a1a2e', title: 'Terminal' },
};

interface Props {
  name: string;
  size?: number;
  className?: string;
  showLabel?: boolean;
  useBrandColor?: boolean;
}

export function BrandIcon({ name, size = 16, className, showLabel, useBrandColor = true }: Props) {
  const icon = icons[name.toLowerCase()];
  const color = icon?.color || '#6852D6';
  const title = icon?.title || name;

  if (!icon?.path) {
    return (
      <span className={className} style={{ display: 'inline-flex', alignItems: 'center', gap: 4 }}>
        <span style={{ width: size, height: size, borderRadius: '50%', background: color, color: '#fff', display: 'inline-flex', alignItems: 'center', justifyContent: 'center', fontSize: size * 0.5, fontWeight: 700 }}>
          {title.charAt(0).toUpperCase()}
        </span>
        {showLabel && <span>{title}</span>}
      </span>
    );
  }

  return (
    <span className={className} style={{ display: 'inline-flex', alignItems: 'center', gap: 4 }}>
      <svg width={size} height={size} viewBox="0 0 24 24" fill={useBrandColor ? color : 'currentColor'} role="img">
        <path d={icon.path} />
      </svg>
      {showLabel && <span>{title}</span>}
    </span>
  );
}

export function getBrandColor(name: string): string {
  return icons[name.toLowerCase()]?.color || '#6852D6';
}

export function getBrandTitle(name: string): string {
  return icons[name.toLowerCase()]?.title || name;
}
