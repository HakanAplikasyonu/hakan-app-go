import path from "node:path";

import { defineConfig } from "vitest/config";

// Birim testleri (#381): saf yardımcılar, node ortamı. Bileşen/e2e testleri
// e2e/ altında Playwright ile koşar.
export default defineConfig({
  resolve: {
    alias: { "@": path.resolve(import.meta.dirname, "src") },
  },
  test: {
    environment: "node",
    include: ["src/**/*.test.{ts,tsx}"],
    env: { TZ: "Europe/Istanbul" },
  },
});
