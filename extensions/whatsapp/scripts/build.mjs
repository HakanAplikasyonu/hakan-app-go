// Uzantıyı dist/ altına derler: TypeScript → esbuild, manifest sürümü
// package.json'dan. HAKAN_API_URL verilirse açılır penceredeki sunucu adresi
// alanının varsayılanı olur (ofis kendi kurulumuna göre paketleyebilir).
import { build } from "esbuild";
import { cp, mkdir, readFile, rm, writeFile } from "node:fs/promises";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

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
const manifest = JSON.parse(await readFile(join(dist, "manifest.json"), "utf8"));
manifest.version = pkg.version;
await writeFile(join(dist, "manifest.json"), JSON.stringify(manifest, null, 2) + "\n");
console.log(`dist/ hazır (sürüm ${pkg.version})`);
