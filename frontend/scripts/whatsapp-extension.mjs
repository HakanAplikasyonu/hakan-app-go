// WhatsApp Web uzantısı paketi (#780 kolay kurulum): `pnpm build` önce bunu
// çalıştırır. extensions/whatsapp derlenir, zip'lenir ve panelden sabit
// adresle indirilsin diye public/indir/ altına kopyalanır:
//   public/indir/easytrade-whatsapp.zip          (sabit adres, son sürüm)
//   public/indir/easytrade-whatsapp-<sürüm>.zip  (sürümlü kopya)
//   public/indir/easytrade-whatsapp.json         (sürüm, boyut; sayfada gösterilir)
// Zip depoya commit'lenmez (.gitignore).
//
// Ortam:
//   WHATSAPP_API_URL       pakete gömülecek API adresi (açılır penceredeki
//                          sunucu adresi; gizli değildir). Boşsa kullanıcı
//                          adresi uzantıda "Gelişmiş" altına yazar.
//   WHATSAPP_PANEL_URL     panel adres(ler)i, virgülle (uzantıyı algılama,
//                          externally_connectable); boşsa API adresinden tahmin.
//   WHATSAPP_EXT_DIR       uzantı kaynağı (varsayılan ../extensions/whatsapp;
//                          Docker'da /extensions/whatsapp).
//   WHATSAPP_EXT_REQUIRED  1 ise kaynak yoksa derleme düşer (imaj derlemesi).
//   WHATSAPP_EXT_SKIP      1 ise hiç paketlenmez (yerel hızlı derleme).
import { execFileSync } from "node:child_process";
import {
  copyFileSync,
  existsSync,
  mkdirSync,
  readFileSync,
  statSync,
  writeFileSync,
} from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const src = resolve(
  process.env.WHATSAPP_EXT_DIR || join(here, "../../extensions/whatsapp"),
);
const out = join(here, "../public/indir");

if (process.env.WHATSAPP_EXT_SKIP === "1") {
  console.log("[whatsapp-extension] WHATSAPP_EXT_SKIP=1: paket atlandı");
  process.exit(0);
}
if (!existsSync(join(src, "package.json"))) {
  const msg = `[whatsapp-extension] uzantı kaynağı yok: ${src}`;
  if (process.env.WHATSAPP_EXT_REQUIRED === "1") {
    console.error(msg);
    process.exit(1);
  }
  console.warn(
    `${msg} (paket atlandı; panelde indirme düğmesi "hazır değil" der)`,
  );
  process.exit(0);
}

const run = (cmd, args, env = process.env) =>
  execFileSync(cmd, args, { cwd: src, stdio: "inherit", env });

if (!existsSync(join(src, "node_modules/esbuild"))) {
  run("npm", ["ci", "--no-audit", "--no-fund"]);
}
run("node", ["scripts/build.mjs"], {
  ...process.env,
  HAKAN_API_URL: process.env.WHATSAPP_API_URL ?? "",
  // Panelin uzantıyı algılayabileceği adres(ler); boşsa API adresinden tahmin.
  HAKAN_PANEL_URL: process.env.WHATSAPP_PANEL_URL ?? "",
});
run("node", ["scripts/package.mjs"]);

const version = JSON.parse(
  readFileSync(join(src, "package.json"), "utf8"),
).version;
const zip = join(src, `easytrade-whatsapp-${version}.zip`);
mkdirSync(out, { recursive: true });
copyFileSync(zip, join(out, "easytrade-whatsapp.zip"));
copyFileSync(zip, join(out, `easytrade-whatsapp-${version}.zip`));
const size = statSync(zip).size;
writeFileSync(
  join(out, "easytrade-whatsapp.json"),
  JSON.stringify({
    version,
    size,
    file: "/indir/easytrade-whatsapp.zip",
    api_embedded: Boolean(process.env.WHATSAPP_API_URL),
  }) + "\n",
);
console.log(
  `[whatsapp-extension] public/indir/easytrade-whatsapp.zip hazır (sürüm ${version}, ${size} bayt)`,
);
