import type { NextConfig } from "next";

// Minimal Next config. The API base is read at runtime from the
// NEXT_PUBLIC_QORVEN_API_BASE env var so the same built artifact can
// point at dev / staging / prod gateways.
const config: NextConfig = {
  reactStrictMode: true,
};

export default config;
