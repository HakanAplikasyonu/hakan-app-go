import { describe, expect, it } from "vitest";

import { ApiError, describeError } from "../src/api";
import { defaultBase, externalReply, normalizeCode, testOutcome } from "../src/setup";

describe("normalizeCode", () => {
  it("accepts any spacing / case and returns XXXX-XXXX", () => {
    expect(normalizeCode(" k7qm 3xpa ")).toEqual({ ok: true, code: "K7QM-3XPA" });
    expect(normalizeCode("K7QM-3XPA")).toEqual({ ok: true, code: "K7QM-3XPA" });
  });
  it("explains a mistyped code in Turkish", () => {
    const empty = normalizeCode("  ");
    expect(empty.ok).toBe(false);
    const short = normalizeCode("K7QM-3XP");
    expect(short).toEqual({ ok: false, error: expect.stringMatching(/8 karakterdir/) });
    const zero = normalizeCode("K7QM-30PA");
    expect(zero).toEqual({ ok: false, error: expect.stringMatching(/"0" kodda olamaz.*0, O, 1, I ve L/) });
  });
});

describe("defaultBase", () => {
  it("prefers the saved address, then the one built into the package", () => {
    expect(defaultBase("https://a.test", "https://b.test")).toBe("https://a.test");
    expect(defaultBase("", " https://b.test ")).toBe("https://b.test");
    expect(defaultBase("", "")).toBe("");
  });
});

describe("testOutcome", () => {
  it("reports server errors first", () => {
    expect(testOutcome("Sunucuya ulaşılamadı.", 1)).toEqual({ ok: false, message: "Sunucuya ulaşılamadı.", openWhatsApp: false });
  });
  it("asks to open WhatsApp Web when no tab is open", () => {
    const r = testOutcome("", 0);
    expect(r.ok).toBe(false);
    expect(r.openWhatsApp).toBe(true);
    expect(r.message).toMatch(/WhatsApp Web sekmesi yok/);
  });
  it("is green when both work", () => {
    expect(testOutcome("", 2).ok).toBe(true);
  });
});

describe("pairing error messages", () => {
  it("covers wrong / expired code, unreachable server and a wrong address", () => {
    expect(describeError(new ApiError("x", 404, "PAIRING_CODE_INVALID")).message).toMatch(/süresi dolmuş.*yeni kod/);
    expect(describeError(new TypeError("Failed to fetch"))).toEqual({ message: expect.stringMatching(/Sunucuya ulaşılamadı/), code: "UNREACHABLE" });
    expect(describeError(new ApiError("HTTP 404", 404, "NOT_A_SERVER")).message).toMatch(/Easytrade sunucusu yanıt vermedi/);
  });
});

describe("externalReply", () => {
  it("answers only ping, without personal data", () => {
    expect(externalReply({ type: "ping" }, "0.3.0", true)).toEqual({ ok: true, version: "0.3.0", paired: true });
    expect(externalReply({ type: "pair" }, "0.3.0", true)).toBeNull();
    expect(externalReply("ping", "0.3.0", true)).toBeNull();
    expect(externalReply(null, "0.3.0", true)).toBeNull();
  });
});
