import type { NextConfig } from "next";
const config: NextConfig = {
  typescript: { ignoreBuildErrors: true },
  devIndicators: false,
  images: {
    remotePatterns: [
      { protocol: "https", hostname: "images.unsplash.com" },
    ],
  },
  async rewrites() {
    return [
      { source: "/v1/:path*", destination: "http://localhost:4200/v1/:path*" },
      { source: "/health", destination: "http://localhost:4200/health" },
    ];
  },
};
export default config;
