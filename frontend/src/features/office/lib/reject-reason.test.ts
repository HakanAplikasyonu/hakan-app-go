import { describe, expect, it } from "vitest";

import en from "@/locales/en/office.json";
import tr from "@/locales/tr/office.json";

import {
  MAX_REJECT_TEXT,
  REJECT_REASON_CODES,
  rejectionBody,
} from "./reject-reason";

describe("rejectionBody (#780)", () => {
  it("needs a code", () => {
    expect(rejectionBody("", "not")).toEqual({ ok: false, error: "code" });
  });
  it("needs a text for diger", () => {
    expect(rejectionBody("diger", "   ")).toEqual({
      ok: false,
      error: "text_required",
    });
    expect(rejectionBody("diger", " Ekim  ayına ait değil ")).toEqual({
      ok: true,
      body: { reason_code: "diger", reason_text: "Ekim ayına ait değil" },
    });
  });
  it("sends a fixed reason without a text", () => {
    expect(rejectionBody("okunaksiz", "")).toEqual({
      ok: true,
      body: { reason_code: "okunaksiz" },
    });
  });
  it("limits the text to 500 characters", () => {
    expect(rejectionBody("mukerrer", "ş".repeat(MAX_REJECT_TEXT)).ok).toBe(
      true,
    );
    expect(rejectionBody("mukerrer", "ş".repeat(MAX_REJECT_TEXT + 1))).toEqual({
      ok: false,
      error: "text_long",
    });
  });
  it("has a label for every reason in both languages", () => {
    for (const code of REJECT_REASON_CODES) {
      const key = `receipts.reject_reason_${code}`;
      expect((tr as Record<string, string>)[key]).toBeTruthy();
      expect((en as Record<string, string>)[key]).toBeTruthy();
    }
  });
});
