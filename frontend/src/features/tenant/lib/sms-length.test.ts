import { describe, expect, it } from "vitest";

import { renderVariables, SAMPLE_VARS, smsLength } from "./sms-length";

describe("smsLength (NetGSM TR)", () => {
  it("counts Turkish letters twice and adds the operator suffix", () => {
    expect(smsLength("çğış")).toEqual({ chars: 13, parts: 1 });
    expect(smsLength("abc")).toEqual({ chars: 8, parts: 1 });
  });

  it("splits at 155 for one SMS and 148 per part", () => {
    expect(smsLength("a".repeat(150))).toEqual({ chars: 155, parts: 1 });
    expect(smsLength("a".repeat(151))).toEqual({ chars: 156, parts: 2 });
    expect(smsLength("a".repeat(291))).toEqual({ chars: 296, parts: 2 });
    expect(smsLength("a".repeat(292))).toEqual({ chars: 297, parts: 3 });
  });

  it("counts the sample taxpayer like the server", () => {
    expect(smsLength(SAMPLE_VARS.mukellef).chars).toBe(35);
  });
});

describe("renderVariables", () => {
  it("places variables and folds spaces", () => {
    expect(
      renderVariables("Sayın :mukellef, :donem son gün :son_gun.", SAMPLE_VARS),
    ).toBe(
      "Sayın Örnek Gıda Ticaret Ltd. Şti., Ekim 2026 son gün 28 Kasım 2026.",
    );
    expect(renderVariables("Banka :banka ekstresi saat 10:00", {})).toBe(
      "Banka ekstresi saat 10:00",
    );
  });
});
