import type { Metadata } from "next";
import { Inter, Inter_Tight, JetBrains_Mono } from "next/font/google";
import Script from "next/script";

import { brand } from "@/config/brand";
import { themeBootScript } from "@/config/theme";
import { AppProviders } from "@/providers/app-providers";

import "./globals.css";

// Arayüz v1 (#324): gövde Inter, başlık Inter Tight, kod ve sayı etiketi
// JetBrains Mono. Değişkenler <html> üzerinde; theme.css :root'ta kullanır.
const inter = Inter({
  variable: "--font-inter",
  subsets: ["latin", "latin-ext"],
  display: "swap",
});

const interTight = Inter_Tight({
  variable: "--font-inter-tight",
  subsets: ["latin", "latin-ext"],
  weight: ["600", "700", "800"],
  display: "swap",
});

const jetbrainsMono = JetBrains_Mono({
  variable: "--font-jetbrains-mono",
  subsets: ["latin", "latin-ext"],
  weight: ["500", "600"],
  display: "swap",
});

export const metadata: Metadata = {
  title: {
    default: brand.productName,
    template: `%s · ${brand.name}`,
  },
  description: brand.tagline,
  applicationName: brand.productName,
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html
      lang="tr"
      suppressHydrationWarning
      data-theme="m3-light"
      className={`${inter.variable} ${interTight.variable} ${jetbrainsMono.variable} h-full`}
    >
      <head>
        {/* Tema (m3/n3 × açık/koyu/sistem) boyamadan önce uygulanır. */}
        <script dangerouslySetInnerHTML={{ __html: themeBootScript }} />
      </head>
      <body className="min-h-full font-sans antialiased">
        {/* Runtime browser config (Sentry DSN); must run before hydration. */}
        <Script src="/api/client-env" strategy="beforeInteractive" />
        <AppProviders>{children}</AppProviders>
      </body>
    </html>
  );
}
