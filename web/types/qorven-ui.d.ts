// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

// Augments the Window interface with Qorven host bus globals.
// App-developer-facing declarations live in the scaffold's qorven-app.d.ts.
declare global {
  interface Window {
    __QorvenApp?: import('@/components/apps/app-host').QorvenAppBus;
    __QorvenUI?: import('@/components/apps/app-host').QorvenUIBus;
  }
}
export {};
