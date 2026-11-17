import { execFileSync } from "node:child_process";
import { mkdtempSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { inflateRawSync } from "node:zlib";
import { describe, expect, it } from "vitest";

// @ts-expect-error: düz JS betiği (scripts/zip.mjs), tip bildirimi yok.
import { crc32, zip } from "../scripts/zip.mjs";

describe("zip", () => {
  it("writes a valid archive", () => {
    const manifest = Buffer.from('{"manifest_version":3,"name":"HAKAN"}');
    const icon = Buffer.from([0x89, 0x50, 0x4e, 0x47]);
    const out: Buffer = zip([
      { name: "manifest.json", data: manifest },
      { name: "icons/16.png", data: icon },
    ]);
    expect(out.readUInt32LE(0)).toBe(0x04034b50);
    expect(out.readUInt32LE(out.length - 22)).toBe(0x06054b50);
    expect(out.readUInt16LE(out.length - 22 + 10)).toBe(2);
    // First entry: name, then its (deflated or stored) data.
    const method = out.readUInt16LE(8);
    const size = out.readUInt32LE(18);
    const nameLen = out.readUInt16LE(26);
    expect(out.subarray(30, 30 + nameLen).toString()).toBe("manifest.json");
    const body = out.subarray(30 + nameLen, 30 + nameLen + size);
    expect((method === 8 ? inflateRawSync(body) : body).equals(manifest)).toBe(true);
    expect(out.readUInt32LE(14)).toBe(crc32(manifest));
  });

  it("matches the system unzip when available", () => {
    const dir = mkdtempSync(join(tmpdir(), "hakan-zip-"));
    const file = join(dir, "t.zip");
    writeFileSync(file, zip([{ name: "a/b.txt", data: Buffer.from("merhaba ".repeat(50)) }]));
    let listing = "";
    try {
      listing = execFileSync("unzip", ["-t", file], { encoding: "utf8" });
    } catch (e) {
      if ((e as NodeJS.ErrnoException).code === "ENOENT") return; // unzip yok
      throw e;
    }
    expect(listing).toMatch(/No errors detected/);
    expect(readFileSync(file).length).toBeGreaterThan(0);
  });
});
