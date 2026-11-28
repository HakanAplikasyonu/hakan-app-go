import { describe, expect, it } from "vitest";

import {
  clampStep,
  DEFAULT_EXTENSION_IDS,
  detectExtension,
  extensionIds,
  type ExtensionRuntime,
  extensionStatus,
  storeUrl,
  parsePackageInfo,
  stepStates,
} from "./whatsapp-setup";

describe("clampStep", () => {
  it("keeps a valid stored step and repairs broken values", () => {
    expect(clampStep(2)).toBe(2);
    expect(clampStep("3")).toBe(3);
    expect(clampStep(9)).toBe(3);
    expect(clampStep(-1)).toBe(0);
    expect(clampStep("x")).toBe(0);
    expect(clampStep(null)).toBe(0);
    expect(clampStep(1.5)).toBe(0);
  });
});

describe("stepStates", () => {
  it("marks earlier steps done and the selected one current", () => {
    expect(stepStates(2, false)).toEqual([
      "done",
      "done",
      "current",
      "pending",
    ]);
  });
  it("marks every step done once the extension is connected", () => {
    expect(stepStates(0, true)).toEqual(["done", "done", "done", "done"]);
  });
});

describe("parsePackageInfo", () => {
  it("reads version and size", () => {
    expect(
      parsePackageInfo({
        version: "0.2.0",
        size: 41234,
        file: "/indir/easytrade-whatsapp.zip",
      }),
    ).toEqual({
      version: "0.2.0",
      size: 41234,
    });
  });
  it("rejects missing or odd content", () => {
    expect(parsePackageInfo(null)).toBeNull();
    expect(parsePackageInfo("<html>")).toBeNull();
    expect(parsePackageInfo({ version: "", size: 1 })).toBeNull();
    expect(parsePackageInfo({ version: "0.2.0", size: 0 })).toBeNull();
  });
});

describe("extensionStatus", () => {
  it("is waiting without an extension session", () => {
    expect(
      extensionStatus([
        {
          user_agent: "Mozilla/5.0 Chrome",
          created_at: "2026-11-28T10:00:00Z",
        },
        { user_agent: null, created_at: "2026-11-28T10:00:00Z" },
      ]),
    ).toEqual({ connected: false });
  });
  it("is connected with the newest extension session as last seen", () => {
    expect(
      extensionStatus([
        {
          user_agent: "HAKAN WhatsApp Web uzantısı · Ofis",
          created_at: "2026-11-28T09:00:00Z",
        },
        {
          user_agent: "Mozilla/5.0 Chrome",
          created_at: "2026-11-28T12:00:00Z",
        },
        {
          user_agent: "HAKAN WhatsApp Web uzantısı",
          created_at: "2026-11-28T11:30:00Z",
        },
      ]),
    ).toEqual({
      connected: true,
      lastSeen: "2026-11-28T11:30:00Z",
      devices: 2,
    });
  });
});

describe("extension detection", () => {
  const fake = (installed: Record<string, unknown>) => {
    const rt: ExtensionRuntime = {
      lastError: undefined,
      sendMessage(id, _msg, cb) {
        if (id in installed) {
          rt.lastError = undefined;
          cb(installed[id]);
        } else {
          rt.lastError = { message: "Could not establish connection." };
          cb(undefined);
        }
      },
    };
    return rt;
  };

  it("merges store ids with the built-in unpacked id", () => {
    expect(extensionIds("  abcdefghijklmnopabcdefghijklmnop, bad-id ")).toEqual(
      ["abcdefghijklmnopabcdefghijklmnop", ...DEFAULT_EXTENSION_IDS],
    );
    expect(extensionIds(undefined)).toEqual(DEFAULT_EXTENSION_IDS);
  });

  it("accepts only an https store link", () => {
    expect(storeUrl(" https://chromewebstore.google.com/detail/x ")).toBe(
      "https://chromewebstore.google.com/detail/x",
    );
    expect(storeUrl("javascript:alert(1)")).toBe("");
    expect(storeUrl("")).toBe("");
  });

  it("finds the installed extension and its pairing state", async () => {
    const id = DEFAULT_EXTENSION_IDS[0]!;
    const rt = fake({ [id]: { ok: true, version: "0.3.0", paired: false } });
    await expect(
      detectExtension(rt, ["abcdefghijklmnopabcdefghijklmnop", id]),
    ).resolves.toEqual({ id, version: "0.3.0", paired: false });
  });

  it("is null outside Chrome or when nothing answers", async () => {
    await expect(
      detectExtension(undefined, DEFAULT_EXTENSION_IDS),
    ).resolves.toBeNull();
    await expect(
      detectExtension(fake({}), DEFAULT_EXTENSION_IDS),
    ).resolves.toBeNull();
    const silent: ExtensionRuntime = { sendMessage() {} };
    await expect(
      detectExtension(silent, DEFAULT_EXTENSION_IDS, 10),
    ).resolves.toBeNull();
  });

  it("marks download and install done once installed", () => {
    expect(stepStates(0, false, true)).toEqual([
      "done",
      "done",
      "pending",
      "pending",
    ]);
  });
});
