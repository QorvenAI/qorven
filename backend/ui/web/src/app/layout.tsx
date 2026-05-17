import type { Metadata } from "next";
import { Inter } from "next/font/google";
import { Toaster } from "sonner";
import "./globals.css";

const font = Inter({ subsets: ["latin"], weight: ["400", "500", "600", "700"] });

export const metadata: Metadata = {
  title: "Qorven",
  description: "AI Agent Platform",
  icons: { icon: "/favicon.svg" },
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en" className="dark">
      <body className={font.className} style={{ margin: 0, padding: 0, background: "var(--vs-bg, #0a0a0f)", color: "var(--vs-text, #f0f0f5)", overflow: "hidden", height: "100dvh" }}>
        {children}
        <Toaster theme="dark" position="top-right" visibleToasts={3} toastOptions={{ style: { background: "transparent", border: "none", boxShadow: "none", padding: 0 } }} />
      </body>
    </html>
  );
}
