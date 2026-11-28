// Mağaza görsellerinin boyut denetimi (#780; CI: extension-whatsapp-gorseller).
// scripts/screenshots.mjs'den sonra çalışır: en az 3 adet 1280×800 ekran
// görüntüsü, 440×280 tanıtım görseli ve 128×128 simge olmalı.
import { readdirSync, readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const dir = join(dirname(fileURLToPath(import.meta.url)), "../store-gorseller");

/** PNG başlığından genişlik × yükseklik (IHDR). */
function size(file) {
  const b = readFileSync(join(dir, file));
  if (b.toString("latin1", 1, 4) !== "PNG") throw new Error(`${file} PNG değil`);
  return `${b.readUInt32BE(16)}x${b.readUInt32BE(20)}`;
}

const files = readdirSync(dir).filter((f) => f.endsWith(".png"));
const problems = [];
const shots = files.filter((f) => f.startsWith("ekran-"));
for (const f of files) {
  const want = f.match(/(\d+)x(\d+)\.png$/)?.slice(1).join("x") ?? (f === "simge-128.png" ? "128x128" : null);
  const got = size(f);
  console.log(`${f}: ${got}`);
  if (want && got !== want) problems.push(`${f}: ${got}, beklenen ${want}`);
}
if (shots.length < 3) problems.push(`en az 3 ekran görüntüsü gerekli, ${shots.length} var`);
if (!files.includes("tanitim-440x280.png")) problems.push("tanitim-440x280.png yok");
if (!files.includes("simge-128.png")) problems.push("simge-128.png yok");
if (problems.length) {
  console.error("Mağaza görselleri hatalı:\n- " + problems.join("\n- "));
  process.exit(1);
}
console.log("Mağaza görselleri tamam.");
