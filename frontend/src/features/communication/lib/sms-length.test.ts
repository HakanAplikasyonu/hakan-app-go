import { describe, expect, it } from "vitest";

import { normalizeSmsText, smsLength } from "./sms-length";

// Sunucudaki TestSMSLength ile aynı örnekler (#780).
describe("smsLength", () => {
  it("counts the operator suffix: 150 plain chars are one SMS", () => {
    expect(smsLength("a".repeat(150))).toEqual({ chars: 155, parts: 1 });
  });
  it("counts Turkish letters twice in TR encoding", () => {
    expect(smsLength("a".repeat(149) + "ş")).toEqual({ chars: 156, parts: 2 });
  });
  it("splits long texts into 148-char parts", () => {
    expect(smsLength("a".repeat(300)).parts).toBe(3);
    expect(smsLength("a".repeat(439)).parts).toBe(3);
    expect(smsLength("a".repeat(440)).parts).toBe(4);
  });
  it("uses 70 / 67 in UNICODE", () => {
    expect(smsLength("ş".repeat(70), "UNICODE")).toEqual({
      chars: 70,
      parts: 1,
    });
    expect(smsLength("ş".repeat(71), "UNICODE").parts).toBe(2);
  });
  it("normalizes currency signs and outer spaces like the server", () => {
    expect(normalizeSmsText("  1.000 ₺ \r\n")).toBe("1.000 TL");
  });
});
