// Chrome Web Store paketi (#780): `npm run package:store`.
//
//   WHATSAPP_API_URL=https://api.ofisiniz.com npm run package:store
//
// Sunucu adresi pakete gömülür (açılır pencerede yalnızca kod + Eşleştir
// görünür), isteğe bağlı konak izni o sunucuya daraltılır ve manifest mağaza
// kurallarına göre denetlenir. Çıktı: magaza-easytrade-whatsapp-<sürüm>.zip
// (depoya commit'lenmez; CI'da "easytrade-whatsapp-magaza" yapıtı).
// Panelden indirilen paket (npm run package) bundan etkilenmez.
import { execFileSync } from "node:child_process";
import { readdir, readFile, writeFile } from "node:fs/promises";
import { dirname, join, relative, sep } from "node:path";
import { fileURLToPath } from "node:url";

import { storeManifest, storeProblems } from "./store-manifest.mjs";
import { zip } from "./zip.mjs";

const root = join(dirname(fileURLToPath(import.meta.url)), "..");
const dist = join(root, "dist");
const apiUrl = (process.env.WHATSAPP_API_URL || process.env.HAKAN_API_URL || "").trim();
if (!apiUrl) {
  console.error("WHATSAPP_API_URL verilmedi: mağaza paketine sunucu adresi gömülmeli.");
  process.exit(1);
}

execFileSync("node", [join(root, "scripts/build.mjs")], { stdio: "inherit", env: { ...process.env, HAKAN_API_URL: apiUrl } });

const manifest = storeManifest(JSON.parse(await readFile(join(dist, "manifest.json"), "utf8")), apiUrl);
const problems = storeProblems(manifest);
if (problems.length) {
  console.error("Mağaza manifesti geçersiz:\n- " + problems.join("\n- "));
  process.exit(1);
}
await writeFile(join(dist, "manifest.json"), JSON.stringify(manifest, null, 2) + "\n");

async function walk(dir) {
  const out = [];
  for (const e of await readdir(dir, { withFileTypes: true })) {
    const p = join(dir, e.name);
    if (e.isDirectory()) out.push(...(await walk(p)));
    else out.push(p);
  }
  return out;
}

const entries = [];
for (const f of (await walk(dist)).sort()) {
  entries.push({ name: relative(dist, f).split(sep).join("/"), data: await readFile(f) });
}
const name = `magaza-easytrade-whatsapp-${manifest.version}.zip`;
await writeFile(join(root, name), zip(entries));
console.log(`${name} hazır (${entries.length} dosya, sunucu ${manifest.optional_host_permissions[0]})`);
