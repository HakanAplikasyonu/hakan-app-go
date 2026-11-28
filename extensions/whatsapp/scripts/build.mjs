// Uzantıyı dist/ altına derler: TypeScript → esbuild; manifest
// scripts/manifest.mjs ile tamamlanır (sürüm, sabit kimlik anahtarı, API izni,
// panelin uzantıyı algılaması). HAKAN_API_URL verilirse pakete gömülür ve
// açılır penceredeki sunucu adresi olur.
import { build } from "esbuild";
import { cp, mkdir, readFile, rm, writeFile } from "node:fs/promises";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

import { finalizeManifest } from "./manifest.mjs";

const root = join(dirname(fileURLToPath(import.meta.url)), "..");
const dist = join(root, "dist");
const pkg = JSON.parse(await readFile(join(root, "package.json"), "utf8"));

await rm(dist, { recursive: true, force: true });
await mkdir(dist, { recursive: true });

const common = {
  bundle: true,
  minify: false,
  sourcemap: false,
  target: "chrome111",
  legalComments: "none",
  define: { __DEFAULT_API_URL__: JSON.stringify(process.env.HAKAN_API_URL ?? "") },
  logLevel: "warning",
};
await build({ ...common, entryPoints: [join(root, "src/background.ts")], outfile: join(dist, "background.js"), format: "esm" });
for (const name of ["content", "main-world", "popup"]) {
  await build({ ...common, entryPoints: [join(root, `src/${name}.ts`)], outfile: join(dist, `${name}.js`), format: "iife" });
}

await cp(join(root, "static"), dist, { recursive: true });
// HAKAN_PANEL_URL: panel adres(ler)i, virgülle (boşsa API adresinden tahmin).
// HAKAN_NO_KEY=1: sabit kimlik anahtarı konmaz (mağaza paketi).
const manifest = finalizeManifest(JSON.parse(await readFile(join(dist, "manifest.json"), "utf8")), {
  version: pkg.version,
  apiUrl: process.env.HAKAN_API_URL ?? "",
  panelUrls: process.env.HAKAN_PANEL_URL ?? "",
  key: await readFile(join(root, "scripts/extension-key.txt"), "utf8"),
  noKey: process.env.HAKAN_NO_KEY === "1",
});
await writeFile(join(dist, "manifest.json"), JSON.stringify(manifest, null, 2) + "\n");
console.log(`dist/ hazır (sürüm ${pkg.version})`);
