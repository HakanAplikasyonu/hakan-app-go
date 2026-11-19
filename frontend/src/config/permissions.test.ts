import { readFileSync } from "node:fs";
import { resolve } from "node:path";

import { describe, expect, it } from "vitest";

import { ALL_PERMISSIONS } from "./permissions";

/**
 * #518 E1: backend'in tanımladığı her kiracı izni web kataloğunda da olmalı;
 * yoksa ilgili düğme ya da sekme izne göre gizlenemez.
 */
describe("permission catalog", () => {
  it("covers every tenant permission of the backend", () => {
    const go = readFileSync(
      resolve(__dirname, "../../../backend/internal/platform/rbac/rbac.go"),
      "utf8",
    );
    const backend = [
      ...go.matchAll(/PermTenant\w+\s*=\s*"(tenant\.[a-z_.]+)"/g),
    ]
      .map((m) => m[1])
      .sort();
    expect(backend.length).toBeGreaterThan(40);
    const known = new Set<string>(ALL_PERMISSIONS);
    expect(backend.filter((p) => !known.has(p))).toEqual([]);
  });
});
