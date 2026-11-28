#!/usr/bin/env node
// Tek kavram tek terim denetimi (#780). docs/sozluk.json'daki yasak
// varyantları Türkçe arayüz metinlerinde (frontend/src/locales/tr/*.json
// değerleri) ve kullanım kılavuzlarında (docs/kullanim/*.md) arar.
// Meşru kullanımlar sozluk.json "allow" listesindedir. Okunur sözlük:
// docs/sozluk.md.
//
//   node scripts/check-sozluk.mjs        bulgu varsa 1 ile çık
//   node scripts/check-sozluk.mjs --json makine okunur rapor
import { readFileSync, readdirSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const cfg = JSON.parse(readFileSync(path.join(ROOT, "docs/sozluk.json"), "utf8"));
const rules = cfg.rules.map((r) => ({ ...r, re: new RegExp(r.pattern, `g${r.flags ?? "u"}`) }));
const keyRules = (cfg.keyRules ?? []).map((r) => ({ ...r, re: new RegExp(r.keyPattern, "u") }));
const allow = cfg.allow ?? [];
const usedAllow = new Set();

const findings = [];

function allowed(rule, file, line) {
  const i = allow.findIndex(
    (a) => a.rule === rule && a.file === file && (!a.contains || line.includes(a.contains)),
  );
  if (i !== -1) usedAllow.add(i);
  return i !== -1;
}

function scan(file, lineNo, raw, text) {
  for (const r of rules) {
    r.re.lastIndex = 0;
    for (const m of text.matchAll(r.re)) {
      if (allowed(r.id, file, raw)) break;
      findings.push({ file, line: lineNo, rule: r.id, found: m[0], canonical: r.canonical, hint: r.hint });
    }
  }
}

// 1) tr locale değerleri (anahtarlar denetlenmez: yalnızca görünen metin).
const trDir = path.join(ROOT, "frontend/src/locales/tr");
for (const name of readdirSync(trDir).filter((n) => n.endsWith(".json")).sort()) {
  const file = `frontend/src/locales/tr/${name}`;
  readFileSync(path.join(trDir, name), "utf8")
    .split("\n")
    .forEach((line, i) => {
      const m = /^\s*"((?:[^"\\]|\\.)*)":\s*"((?:[^"\\]|\\.)*)"/.exec(line);
      if (!m) return;
      const [, key, value] = m;
      scan(file, i + 1, line, value);
      for (const k of keyRules) {
        if (k.re.test(key) && value === k.value && !allowed(k.id, file, line)) {
          findings.push({ file, line: i + 1, rule: k.id, found: value, canonical: k.canonical, hint: k.hint });
        }
      }
    });
}

// 2) Kullanım kılavuzları: başlık bloğu, kod blokları, satır içi kod ve
// bağlantı adresleri hariç.
const docDir = path.join(ROOT, "docs/kullanim");
for (const name of readdirSync(docDir).filter((n) => n.endsWith(".md")).sort()) {
  const file = `docs/kullanim/${name}`;
  let fence = false;
  let front = false;
  readFileSync(path.join(docDir, name), "utf8")
    .split("\n")
    .forEach((line, i) => {
      if (i === 0 && line === "---") return void (front = true);
      if (front) {
        if (line === "---") front = false;
        return;
      }
      if (/^\s*```/.test(line)) return void (fence = !fence);
      if (fence) return;
      const text = line
        .replace(/`[^`]*`/g, " ")
        .replace(/\]\([^)]*\)/g, "] ")
        .replace(/https?:\/\/\S+/g, " ");
      scan(file, i + 1, line, text);
    });
}

const stale = allow.filter((_, i) => !usedAllow.has(i));

if (process.argv.includes("--json")) {
  console.log(JSON.stringify({ ok: findings.length === 0, findings, staleAllow: stale }, null, 2));
} else {
  for (const f of findings) {
    console.log(`${f.file}:${f.line}  "${f.found}" → "${f.canonical}"  (${f.rule}) ${f.hint}`);
  }
  for (const a of stale) {
    console.log(`uyarı: kullanılmayan izin: ${a.rule} @ ${a.file}${a.contains ? ` (${a.contains})` : ""}`);
  }
  console.log(
    findings.length
      ? `\nsözlük: ${findings.length} yasak varyant. Kanonik terimler: docs/sozluk.md`
      : "sözlük: ok — yasak varyant yok.",
  );
}
process.exit(findings.length ? 1 : 0);
