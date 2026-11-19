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
const ORDER = ["ofis-yoneticisi", "ofis-calisani", "mizan-hafizasi", "mukellef", "masaustu", "whatsapp-uzantisi", "demo-senaryosu"];

// Rol anahtarları (#522): ofis rolleri (`roles.slug`, docs/yetki-matrisi.md),
// mükellef kullanıcısı ve platform yöneticisi.
const ROLES = ["owner", "admin", "accountant", "intern", "member", "taxpayer", "platform"];

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
  const raw = readFileSync(path.join(srcDir, `${slug}.md`), "utf8").replace(/\r\n?/g, "\n");
  // Başlık (frontmatter, #522): `roles: [owner, admin]` kılavuzun kimin için
  // olduğunu, `internal: true` son kullanıcıya gösterilmeyeceğini söyler.
  const fm = /^---\n([\s\S]*?)\n---\n+/.exec(raw);
  if (!fm) problems.push(`${slug}.md: başlık (--- roles: […] ---) yok`);
  const body = fm ? raw.slice(fm[0].length) : raw;
  const meta = fm ? fm[1] : "";
  const rolesLine = /^roles:\s*\[(.*)\]\s*$/m.exec(meta);
  const roles = rolesLine
    ? rolesLine[1].split(",").map((r) => r.trim()).filter(Boolean)
    : [];
  const internal = /^internal:\s*true\s*$/m.test(meta);
  const badRoles = roles.filter((r) => !ROLES.includes(r));
  if (badRoles.length) problems.push(`${slug}.md: bilinmeyen rol: ${badRoles} (geçerli: ${ROLES})`);
  if (!internal && roles.length === 0) problems.push(`${slug}.md: roles boş (iç belge değilse en az bir rol gerekir)`);
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
  return { slug, title: h1 ? h1[1].trim() : slug, summary: summary ? summary[1].trim() : "", roles, internal, body };
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
  /** Kılavuzun kimin için olduğu: ofis rolü anahtarı, \`taxpayer\`, \`platform\`. */
  roles: readonly string[];
  /** İç belge (ör. demo senaryosu): son kullanıcıya gösterilmez. */
  internal: boolean;
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
