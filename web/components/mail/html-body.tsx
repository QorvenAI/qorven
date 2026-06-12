'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

/**
 * HtmlBody — renders a sanitized HTML email body.
 *
 * Security boundary: all inbound HTML is passed through DOMPurify before it
 * touches the DOM.  A malicious inbound email must not execute script in the
 * operator's browser.
 *
 * Forbidden: <script>, <iframe>, <object>, <embed>, <form>, <input>, all on*
 * event attributes, javascript: / data: / vbscript: URLs in href/src/action,
 * CSS expressions, import(), and SVG foreignObject.
 *
 * Allowed: common email markup — paragraphs, divs, spans, links, images,
 * lists, tables, headings, blockquotes, pre/code, horizontal rules, inline
 * formatting, and style attributes on elements (stripped of dangerous tokens
 * by the FORBID_CSS_PROPERTIES pass).
 *
 * All <a> tags get target="_blank" and rel="noopener noreferrer" injected
 * via an afterSanitizeAttributes hook so phishing links don't navigate the
 * app frame.
 *
 * <style> blocks are stripped entirely — they can contain CSS expressions
 * and break the host page's layout.  Inline style= attributes are kept but
 * scrubbed of expression() / url() / import tokens.
 *
 * Remote images are rendered as-is (good-enough for v1); a future iteration
 * can add a "block remote images / load images" toggle on top of this
 * component without changing the sanitizer contract.
 */

import { useEffect, useRef } from 'react';
import DOMPurify, { type Config as DOMPurifyConfig } from 'dompurify';

// Tags completely forbidden in email rendering.
const FORBID_TAGS = [
  'script', 'noscript',
  'iframe', 'frame', 'frameset',
  'object', 'embed', 'applet',
  'form', 'input', 'button', 'select', 'textarea', 'datalist', 'output',
  'style',                    // strips entire <style> blocks — keeps inline style=
  'meta', 'link', 'base',
  'svg',                      // SVG can carry JS; whitelist-only would be safer but email rarely needs it
  'math',
  'template', 'slot',
  'canvas', 'video', 'audio', 'source', 'track',
];

// Attributes forbidden on every element.
const FORBID_ATTR = [
  // Event handlers
  'onerror', 'onload', 'onmouseover', 'onfocus', 'onblur', 'onclick',
  'onchange', 'onsubmit', 'onreset', 'onselect', 'onkeydown', 'onkeyup',
  'onkeypress', 'onmousedown', 'onmouseup', 'onmousemove', 'onmouseout',
  'ondblclick', 'oncontextmenu', 'onwheel', 'ondrag', 'ondrop',
  'onpointerdown', 'onpointerup', 'onpointermove', 'onpointerover',
  'onpointerout', 'onpointerenter', 'onpointerleave', 'onpointercancel',
  'ontouchstart', 'ontouchend', 'ontouchmove', 'ontouchcancel',
  'onanimationstart', 'onanimationend', 'onanimationiteration',
  'ontransitionend', 'onscroll', 'onresize', 'onhashchange',
  'onpageshow', 'onpagehide', 'onunload', 'onbeforeunload',
  'onmessage', 'onoffline', 'ononline', 'onstorage', 'onpopstate',
  // Dangerous attributes
  'srcdoc', 'formaction', 'ping', 'xlink:href',
];

// Dangerous CSS property value patterns to strip from style= attributes.
// These are applied after DOMPurify runs via the afterSanitizeAttributes hook.
const DANGEROUS_STYLE_PATTERNS = [
  /expression\s*\(/gi,
  /url\s*\(\s*["']?\s*javascript:/gi,
  /url\s*\(\s*["']?\s*data:/gi,
  /url\s*\(\s*["']?\s*vbscript:/gi,
  /-moz-binding\s*:/gi,
  /behavior\s*:/gi,
  /@import/gi,
];

function buildPurifyConfig(): DOMPurifyConfig {
  return {
    FORBID_TAGS,
    FORBID_ATTR,
    // Force all protocols not in the allowlist to be stripped.
    ALLOWED_URI_REGEXP: /^(?:(?:https?|mailto|tel|ftp):|[^a-z]|[a-z+.-]+(?:[^a-z+.\-:]|$))/i,
    // Keep HTML entities intact.
    ALLOW_UNKNOWN_PROTOCOLS: false,
    // Prevent DOM clobbering.
    SANITIZE_DOM: true,
    // Keep data attributes (harmless, used by many email clients for tracking — no exec risk).
    ADD_ATTR: ['data-*'],
    WHOLE_DOCUMENT: false,
    RETURN_DOM: false,
    RETURN_DOM_FRAGMENT: false,
  };
}

function scrubInlineStyles(node: Element) {
  const style = node.getAttribute('style');
  if (!style) return;
  let clean = style;
  for (const pattern of DANGEROUS_STYLE_PATTERNS) {
    clean = clean.replace(pattern, '');
  }
  if (clean !== style) node.setAttribute('style', clean);
}

// Register DOMPurify hooks once (idempotent — DOMPurify deduplicates by hook fn identity… not
// guaranteed, so we gate on a module-level flag instead).
let hooksRegistered = false;

function ensureHooks() {
  if (hooksRegistered) return;
  hooksRegistered = true;

  // Force all <a> tags to open in a new tab and strip referrer information.
  DOMPurify.addHook('afterSanitizeAttributes', (node) => {
    if (node.tagName === 'A') {
      node.setAttribute('target', '_blank');
      node.setAttribute('rel', 'noopener noreferrer');
    }
    // Scrub remaining inline style expressions.
    scrubInlineStyles(node);
  });
}

interface HtmlBodyProps {
  /** Raw HTML string from the mail server — will be sanitized before render. */
  html?: string | null;
  /** Plain-text fallback when html is empty. Rendered whitespace-pre-wrap. */
  text?: string | null;
  className?: string;
}

/**
 * Renders sanitized HTML email body, or falls back to escaped plain text.
 * This component is 'use client' — DOMPurify requires a DOM environment.
 */
export function HtmlBody({ html, text, className }: HtmlBodyProps) {
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    ensureHooks();
  }, []);

  // Prefer HTML if available.
  const hasHtml = html && html.trim().length > 0;

  if (hasHtml) {
    // DOMPurify.sanitize is synchronous and safe to call on every render
    // because it creates a fresh DOM fragment each time.
    const clean = DOMPurify.sanitize(html, buildPurifyConfig());

    return (
      <div
        ref={ref}
        className={className}
        // eslint-disable-next-line react/no-danger
        dangerouslySetInnerHTML={{ __html: clean }}
      />
    );
  }

  // Plain-text fallback — escape is handled by React (no dangerouslySetInnerHTML).
  const displayText = text?.trim() ?? '(no content)';
  return (
    <div ref={ref} className={className}>
      <pre className="whitespace-pre-wrap font-sans text-sm leading-relaxed break-words">
        {displayText}
      </pre>
    </div>
  );
}
