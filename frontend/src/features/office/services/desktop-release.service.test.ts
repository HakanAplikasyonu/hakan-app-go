import { describe, expect, it } from "vitest";

import { pickDesktopRelease } from "./desktop-release.service";

const asset = (name: string) => ({
  name,
  browser_download_url: `https://github.com/x/releases/download/${name}`,
});

describe("pickDesktopRelease", () => {
  it("returns null without desktop releases", () => {
    expect(pickDesktopRelease([])).toBeNull();
    expect(
      pickDesktopRelease([
        { tag_name: "v1.0.0", assets: [asset("a.dmg")] },
        { tag_name: "desktop-v0.1.0", draft: true, assets: [asset("b.dmg")] },
        { tag_name: "desktop-v0.2.0", assets: [asset("latest.json")] },
      ]),
    ).toBeNull();
  });

  it("picks the newest stable release with dmg and msi", () => {
    const r = pickDesktopRelease([
      {
        tag_name: "desktop-v0.3.0-beta.1",
        prerelease: true,
        published_at: "2026-11-20T00:00:00Z",
        assets: [asset("Hakan_0.3.0-beta.1_universal.dmg")],
      },
      {
        tag_name: "desktop-v0.2.0",
        published_at: "2026-11-10T00:00:00Z",
        assets: [
          asset("Hakan_0.2.0_universal.dmg"),
          asset("Hakan_0.2.0_x64-setup.exe"),
          asset("Hakan_0.2.0_x64_en-US.msi"),
          asset("Hakan_0.2.0_x64_en-US.msi.sig"),
        ],
      },
      {
        tag_name: "desktop-v0.1.0",
        published_at: "2026-10-01T00:00:00Z",
        assets: [asset("Hakan_0.1.0_universal.dmg")],
      },
    ]);
    expect(r?.version).toBe("0.2.0");
    expect(r?.mac?.name).toBe("Hakan_0.2.0_universal.dmg");
    expect(r?.windows?.name).toBe("Hakan_0.2.0_x64_en-US.msi");
  });

  it("falls back to exe and to a beta when no stable release exists", () => {
    const r = pickDesktopRelease([
      {
        tag_name: "desktop-v0.1.0-beta.2",
        prerelease: true,
        published_at: "2026-11-20T00:00:00Z",
        assets: [asset("Hakan_0.1.0_x64-setup.exe.sig"), asset("Hakan_0.1.0_x64-setup.exe")],
      },
    ]);
    expect(r?.prerelease).toBe(true);
    expect(r?.mac).toBeUndefined();
    expect(r?.windows?.name).toBe("Hakan_0.1.0_x64-setup.exe");
  });
});
