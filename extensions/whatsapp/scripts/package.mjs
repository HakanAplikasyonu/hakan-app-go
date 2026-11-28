// dist/ → easytrade-whatsapp-<sürüm>.zip (Chrome / Edge'e "paketlenmemiş" yükleme
// için açılır; mağazaya gönderilmez). Önce `npm run build` çalışmış olmalı
// (`npm run package` ikisini birlikte yapar).
import { readdir, readFile, writeFile } from "node:fs/promises";
import { join, relative, sep } from "node:path";
import { dirname } from "node:path";
import { fileURLToPath } from "node:url";

import { zip } from "./zip.mjs";

const root = join(dirname(fileURLToPath(import.meta.url)), "..");
const dist = join(root, "dist");

async function walk(dir) {
  const out = [];
  for (const e of await readdir(dir, { withFileTypes: true })) {
    const p = join(dir, e.name);
    if (e.isDirectory()) out.push(...(await walk(p)));
    else out.push(p);
  }
  return out;
}

const manifest = JSON.parse(await readFile(join(dist, "manifest.json"), "utf8"));
const files = (await walk(dist)).sort();
const entries = [];
for (const f of files) {
  entries.push({ name: relative(dist, f).split(sep).join("/"), data: await readFile(f) });
}
const name = `easytrade-whatsapp-${manifest.version}.zip`;
await writeFile(join(root, name), zip(entries));
console.log(`${name} hazır (${entries.length} dosya)`);
