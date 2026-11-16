#!/usr/bin/env node
// Gece e2e raporu (#304): Playwright JSON çıktısını Markdown özete çevirir.
//
//   node scripts/e2e-report.mjs e2e/results.json [run-url]
//
// stdout: özet (GitHub iş özeti ve issue gövdesi için).
// Çıkış kodu: 0 hepsi geçti, 1 düşen ya da beklenmedik geçen test var,
// 2 rapor okunamadı.
import { readFileSync } from "node:fs";

const [file, runUrl = ""] = process.argv.slice(2);
let report;
try {
  report = JSON.parse(readFileSync(file, "utf8"));
} catch (err) {
  console.log(`## Gece e2e: rapor okunamadı\n\n\`${file}\`: ${err.message}\n`);
  process.exit(2);
}

const tests = [];
function walk(suite, trail) {
  const here = suite.title ? [...trail, suite.title] : trail;
  for (const spec of suite.specs ?? []) {
    for (const t of spec.tests ?? []) {
      const last = t.results?.[t.results.length - 1];
      tests.push({
        file: spec.file ?? suite.file ?? "",
        title: [...here.slice(1), spec.title].filter(Boolean).join(" › "),
        status: t.status, // expected | unexpected | flaky | skipped
        error: (last?.error?.message ?? "").split("\n")[0].replace(/\u001b\[[0-9;]*m/g, "").slice(0, 200),
      });
    }
  }
  for (const child of suite.suites ?? []) walk(child, here);
}
for (const s of report.suites ?? []) walk(s, []);

const count = (st) => tests.filter((t) => t.status === st).length;
const failed = tests.filter((t) => t.status === "unexpected");
const summary = {
  passed: count("expected"),
  failed: failed.length,
  flaky: count("flaky"),
  skipped: count("skipped"),
};
const ok = failed.length === 0 && (report.errors ?? []).length === 0;

const lines = [
  `## Gece e2e: ${ok ? "yeşil" : "KIRMIZI"}`,
  "",
  `Geçen ${summary.passed} · düşen ${summary.failed} · kararsız ${summary.flaky} · atlanan ${summary.skipped}`,
];
if (runUrl) lines.push("", `Koşu ve rapor artefaktı: ${runUrl}`);
if (failed.length) {
  lines.push("", "| Dosya | Test | Hata |", "|---|---|---|");
  for (const t of failed) {
    const cell = (s) => s.replace(/\|/g, "\\|");
    lines.push(`| \`${cell(t.file)}\` | ${cell(t.title)} | ${cell(t.error) || "—"} |`);
  }
}
for (const e of report.errors ?? []) {
  lines.push("", `Genel hata: ${(e.message ?? "").split("\n")[0]}`);
}
console.log(lines.join("\n"));
process.exit(ok ? 0 : 1);
