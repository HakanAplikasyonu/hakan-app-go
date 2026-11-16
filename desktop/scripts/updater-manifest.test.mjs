import assert from "node:assert/strict";
import { mkdirSync, mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { test } from "node:test";

import { buildManifest, channelFor, collectSigned, endpointFor } from "./updater-manifest.mjs";

test("channel from version: prerelease → beta", () => {
  assert.equal(channelFor("1.2.3"), "stable");
  assert.equal(channelFor("1.3.0-beta.1"), "beta");
  assert.throws(() => channelFor("v1.2"));
});

test("endpoint: GitHub by default, base URL per channel when set", () => {
  assert.match(endpointFor("stable"), /releases\/latest\/download\/latest\.json$/);
  assert.match(endpointFor("beta"), /releases\/download\/desktop-beta\/latest\.json$/);
  assert.equal(endpointFor("beta", "https://cdn.example.com/hakan/"), "https://cdn.example.com/hakan/beta/latest.json");
  assert.throws(() => endpointFor("nightly"));
});

test("manifest: universal mac bundle serves both arches, windows NSIS", () => {
  const m = buildManifest({
    version: "1.2.3",
    channel: "stable",
    tag: "desktop-v1.2.3",
    notes: "Düzeltmeler",
    pubDate: "2026-11-16T00:00:00Z",
    files: [
      { name: "Hakan.app.tar.gz", signature: "SIGMAC\n" },
      { name: "Hakan_1.2.3_x64-setup.exe", signature: "SIGWIN" },
    ],
  });
  assert.equal(m.version, "1.2.3");
  assert.equal(m.pub_date, "2026-11-16T00:00:00Z");
  assert.deepEqual(Object.keys(m.platforms).sort(), ["darwin-aarch64", "darwin-x86_64", "windows-x86_64"]);
  assert.equal(m.platforms["darwin-aarch64"].signature, "SIGMAC");
  assert.equal(
    m.platforms["windows-x86_64"].url,
    "https://github.com/HakanAplikasyonu/hakan-app-go/releases/download/desktop-v1.2.3/Hakan_1.2.3_x64-setup.exe",
  );
});

test("manifest: base URL puts files under the channel; channel must match version", () => {
  const m = buildManifest({
    version: "1.3.0-beta.1",
    channel: "beta",
    tag: "desktop-v1.3.0-beta.1",
    baseUrl: "https://cdn.example.com/hakan",
    files: [{ name: "Hakan.app.tar.gz", signature: "S" }],
  });
  assert.equal(m.platforms["darwin-x86_64"].url, "https://cdn.example.com/hakan/beta/Hakan.app.tar.gz");
  assert.throws(() =>
    buildManifest({ version: "1.3.0-beta.1", channel: "stable", tag: "t", files: [{ name: "a.app.tar.gz", signature: "S" }] }),
  );
  assert.throws(() => buildManifest({ version: "1.2.3", channel: "stable", tag: "t", files: [] }));
});

test("collectSigned finds bundles that have a .sig next to them", () => {
  const dir = mkdtempSync(path.join(tmpdir(), "hakan-manifest-"));
  mkdirSync(path.join(dir, "mac"));
  writeFileSync(path.join(dir, "mac", "Hakan.app.tar.gz"), "x");
  writeFileSync(path.join(dir, "mac", "Hakan.app.tar.gz.sig"), "SIG");
  writeFileSync(path.join(dir, "orphan.exe.sig"), "SIG");
  writeFileSync(path.join(dir, "Hakan.dmg"), "x");
  assert.deepEqual(collectSigned(dir), [{ name: "Hakan.app.tar.gz", signature: "SIG" }]);
});
