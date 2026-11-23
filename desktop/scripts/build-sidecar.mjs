#!/usr/bin/env node
// Go sidecar'ı Tauri'nin beklediği adla derler:
//   src-tauri/binaries/hakan-sidecar-<rust-target-triple>[.exe]
//
// Hedef sırası: komut satırı argümanı > TAURI_ENV_TARGET_TRIPLE > `rustc -vV`
// host'u. `universal-apple-darwin` için iki mimari derlenip lipo ile birleşir.
//
// Sürüm bilgisi: HAKAN_DESKTOP_VERSION (yoksa package.json) ve git commit'i
// ldflags ile gömülür. HAKAN_API_BASE_URL verilirse (sürüm iş akışı) sunucu
// adresi varsayılan olarak gömülür; kullanıcı uygulamada değiştirebilir.

import { execFileSync } from "node:child_process";
import { mkdirSync, readFileSync, rmSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const sidecarDir = path.join(root, "sidecar");
const outDir = path.join(root, "src-tauri", "binaries");

const goTargets = {
  "x86_64-apple-darwin": ["darwin", "amd64"],
  "aarch64-apple-darwin": ["darwin", "arm64"],
  "x86_64-pc-windows-msvc": ["windows", "amd64"],
  "aarch64-pc-windows-msvc": ["windows", "arm64"],
  "x86_64-unknown-linux-gnu": ["linux", "amd64"],
  "aarch64-unknown-linux-gnu": ["linux", "arm64"],
};

function hostTriple() {
  const out = execFileSync("rustc", ["-vV"], { encoding: "utf8" });
  const m = out.match(/^host:\s*(\S+)/m);
  if (!m) throw new Error("rustc -vV çıktısında host yok");
  return m[1];
}

function gitCommit() {
  try {
    return execFileSync("git", ["rev-parse", "--short", "HEAD"], {
      cwd: root,
      encoding: "utf8",
    }).trim();
  } catch {
    return "none";
  }
}

const version =
  process.env.HAKAN_DESKTOP_VERSION ||
  JSON.parse(readFileSync(path.join(root, "package.json"), "utf8")).version;
const apiBase = (process.env.HAKAN_API_BASE_URL || "").trim();
if (apiBase && !/^https?:\/\/[^\s'"]+$/.test(apiBase)) {
  throw new Error(`HAKAN_API_BASE_URL geçerli bir http(s) adresi değil: ${apiBase}`);
}
const ldflags =
  `-s -w -X main.version=${version} -X main.commit=${gitCommit()}` +
  (apiBase ? ` -X main.defaultBaseURL=${apiBase}` : "");

function build(triple, output) {
  const target = goTargets[triple];
  if (!target) throw new Error(`desteklenmeyen hedef: ${triple}`);
  const [goos, goarch] = target;
  execFileSync(
    "go",
    ["build", "-trimpath", "-ldflags", ldflags, "-o", output, "."],
    {
      cwd: sidecarDir,
      stdio: "inherit",
      env: { ...process.env, GOOS: goos, GOARCH: goarch, CGO_ENABLED: "0" },
    },
  );
}

const triple =
  process.argv[2] || process.env.TAURI_ENV_TARGET_TRIPLE || hostTriple();
mkdirSync(outDir, { recursive: true });

if (triple === "universal-apple-darwin") {
  const parts = ["aarch64-apple-darwin", "x86_64-apple-darwin"].map((t) => {
    const out = path.join(outDir, `hakan-sidecar-${t}`);
    build(t, out);
    return out;
  });
  const out = path.join(outDir, `hakan-sidecar-${triple}`);
  execFileSync("lipo", ["-create", "-output", out, ...parts], {
    stdio: "inherit",
  });
  console.log(`sidecar: ${path.relative(root, out)}`);
} else {
  const ext = triple.includes("windows") ? ".exe" : "";
  const out = path.join(outDir, `hakan-sidecar-${triple}${ext}`);
  rmSync(out, { force: true });
  build(triple, out);
  console.log(`sidecar: ${path.relative(root, out)}`);
}
