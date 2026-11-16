#!/usr/bin/env node
// Tauri güncelleyici manifesti (latest.json) üretir (#183).
//
// desktop-release iş akışı imzalı güncelleme paketlerini (.app.tar.gz +
// .sig, NSIS .exe + .sig) artefakt olarak yükler; bu betik onlardan kanal
// başına bir manifest yazar. Manifest yalnızca artefakt olur: yayımlama
// (GitHub Release / depolama) ayrı bir karardır, bu betik hiçbir yere
// yüklemez.
//
//   node scripts/updater-manifest.mjs --dir <artefakt klasörü> --version 1.2.3 \
//     --channel stable --tag desktop-v1.2.3 [--base-url https://...] [--notes "..."] \
//     [--out latest.json]
//
// Dosya adresleri: --base-url verilirse <base>/<kanal>/<dosya>, verilmezse
// GitHub Release indirme adresi (…/releases/download/<etiket>/<dosya>).
import { readdirSync, readFileSync, statSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

export const REPO = "HakanAplikasyonu/hakan-app-go";
export const CHANNELS = ["stable", "beta"];

/** desktop-v1.2.3 → stable, desktop-v1.2.3-beta.1 → beta. */
export function channelFor(version) {
  if (!/^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$/.test(version)) {
    throw new Error(`invalid version: ${version}`);
  }
  return version.includes("-") ? "beta" : "stable";
}

/** Uygulamanın baktığı manifest adresi (tauri plugins.updater.endpoints). */
export function endpointFor(channel, baseUrl = "") {
  if (!CHANNELS.includes(channel)) throw new Error(`unknown channel: ${channel}`);
  if (baseUrl) return `${baseUrl.replace(/\/+$/, "")}/${channel}/latest.json`;
  return channel === "stable"
    ? `https://github.com/${REPO}/releases/latest/download/latest.json`
    : `https://github.com/${REPO}/releases/download/desktop-beta/latest.json`;
}

function fileUrl({ name, channel, tag, baseUrl }) {
  if (baseUrl) return `${baseUrl.replace(/\/+$/, "")}/${channel}/${encodeURIComponent(name)}`;
  return `https://github.com/${REPO}/releases/download/${encodeURIComponent(tag)}/${encodeURIComponent(name)}`;
}

/**
 * files: [{ name, signature }] (imzası olan güncelleme paketleri).
 * macOS universal paket hem darwin-aarch64 hem darwin-x86_64 için kullanılır.
 */
export function buildManifest({ version, channel, tag, baseUrl = "", notes = "", pubDate, files }) {
  if (channelFor(version) !== channel) {
    throw new Error(`version ${version} does not belong to channel ${channel}`);
  }
  const platforms = {};
  const add = (key, f) => {
    platforms[key] = { signature: f.signature.trim(), url: fileUrl({ name: f.name, channel, tag, baseUrl }) };
  };
  for (const f of files) {
    if (f.name.endsWith(".app.tar.gz")) {
      add("darwin-aarch64", f);
      add("darwin-x86_64", f);
    } else if (/-setup\.exe$/i.test(f.name) || f.name.endsWith(".nsis.zip")) {
      add("windows-x86_64", f);
    }
  }
  if (Object.keys(platforms).length === 0) {
    throw new Error("no signed updater bundles (.app.tar.gz / -setup.exe with .sig)");
  }
  return { version, notes, pub_date: pubDate ?? new Date().toISOString(), platforms };
}

/** Klasördeki (alt klasörler dahil) imzalı paketleri bulur. */
export function collectSigned(dir) {
  const out = [];
  const walk = (d) => {
    for (const e of readdirSync(d)) {
      const p = path.join(d, e);
      if (statSync(p).isDirectory()) walk(p);
      else if (p.endsWith(".sig")) {
        const bundle = p.slice(0, -4);
        try {
          statSync(bundle);
          out.push({ name: path.basename(bundle), signature: readFileSync(p, "utf8") });
        } catch {
          // .sig without its bundle: ignore.
        }
      }
    }
  };
  walk(dir);
  return out;
}

function parseArgs(argv) {
  const args = {};
  for (let i = 0; i < argv.length; i += 2) {
    if (!argv[i].startsWith("--")) throw new Error(`unexpected argument: ${argv[i]}`);
    args[argv[i].slice(2)] = argv[i + 1] ?? "";
  }
  return args;
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const a = parseArgs(process.argv.slice(2));
  for (const k of ["dir", "version", "channel", "tag"]) {
    if (!a[k]) throw new Error(`--${k} is required`);
  }
  const manifest = buildManifest({
    version: a.version,
    channel: a.channel,
    tag: a.tag,
    baseUrl: a["base-url"] ?? "",
    notes: a.notes ?? "",
    files: collectSigned(a.dir),
  });
  const out = a.out ?? "latest.json";
  writeFileSync(out, JSON.stringify(manifest, null, 2) + "\n");
  console.log(`${out}: ${a.channel} ${a.version} → ${Object.keys(manifest.platforms).join(", ")}`);
}
