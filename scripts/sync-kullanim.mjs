#!/usr/bin/env node
// Kullanım kılavuzlarını (docs/kullanim/*.md, #274) ön yüze taşır:
// frontend/src/features/help/content.generated.ts. Ön yüz Docker bağlamı
// yalnızca frontend/ olduğu için metinler oraya üretilir; ekran görüntüleri
// tek kopya olarak frontend/public/yardim/img/ altındadır.
//
//   node scripts/sync-kullanim.mjs          üret
//   node scripts/sync-kullanim.mjs --check  üretilmiş dosya güncel değilse 1 ile çık
//   --no-images: görseller henüz üretilmediyse varlık denetimini atla (ilk kurulum)
import { readFileSync, readdirSync, writeFileSync, existsSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const srcDir = path.join(root, "docs/kullanim");
const out = path.join(root, "frontend/src/features/help/content.generated.ts");
const imgDir = path.join(root, "frontend/public/yardim/img");

// Sıra: Yardım sayfasındaki sıra. README yalnızca depoda okunur.
const ORDER = ["ofis-yoneticisi", "ofis-calisani", "mizan-hafizasi", "mukellef", "masaustu", "whatsapp-uzantisi"];

const files = readdirSync(srcDir).filter((f) => f.endsWith(".md") && f !== "README.md");
const slugs = files.map((f) => f.replace(/\.md$/, ""));
const unknown = slugs.filter((s) => !ORDER.includes(s));
const missing = ORDER.filter((s) => !slugs.includes(s));
if (unknown.length || missing.length) {
  console.error(`sync-kullanim: ORDER ile docs/kullanim uyuşmuyor (fazla: ${unknown}, eksik: ${missing})`);
  process.exit(1);
}

const problems = [];
const guides = ORDER.map((slug) => {
  const body = readFileSync(path.join(srcDir, `${slug}.md`), "utf8").replace(/\r\n?/g, "\n");
  const h1 = /^#\s+(.+)$/m.exec(body);
  if (!h1) problems.push(`${slug}.md: başlık (# …) yok`);
  const summary = /^>\s*(.+)$/m.exec(body);
  for (const m of body.matchAll(/!\[[^\]]*\]\(([^)]+)\)/g)) {
    const name = path.basename(m[1]);
    if (!m[1].includes("frontend/public/yardim/img/")) {
      problems.push(`${slug}.md: görsel yolu frontend/public/yardim/img/ altında olmalı: ${m[1]}`);
    } else if (!process.argv.includes("--no-images") && !existsSync(path.join(imgDir, name))) {
      problems.push(`${slug}.md: görsel yok: frontend/public/yardim/img/${name} (npm run docs:screenshots, e2e/)`);
    }
  }
  return { slug, title: h1 ? h1[1].trim() : slug, summary: summary ? summary[1].trim() : "", body };
});
if (problems.length) {
  console.error(problems.map((p) => `sync-kullanim: ${p}`).join("\n"));
  process.exit(1);
}

const content = `// Bu dosya üretilmiştir: node scripts/sync-kullanim.mjs (kaynak docs/kullanim/*.md).
// Elle düzenlemeyin.

export type HelpGuide = {
  slug: string;
  title: string;
  summary: string;
  body: string;
};

export const HELP_GUIDES: readonly HelpGuide[] = ${JSON.stringify(guides, null, 2)};
`;

if (process.argv.includes("--check")) {
  const current = existsSync(out) ? readFileSync(out, "utf8") : "";
  if (current !== content) {
    console.error("sync-kullanim: frontend/src/features/help/content.generated.ts güncel değil.\n  düzeltme: node scripts/sync-kullanim.mjs");
    process.exit(1);
  }
  console.log("sync-kullanim: ok");
} else {
  writeFileSync(out, content);
  console.log(`sync-kullanim: ${guides.length} kılavuz → ${path.relative(root, out)}`);
}
