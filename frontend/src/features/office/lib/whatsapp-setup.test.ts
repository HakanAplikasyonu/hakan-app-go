import { describe, expect, it } from "vitest";

import {
  clampStep,
  extensionStatus,
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
