import { describe, expect, it } from "vitest";

import {
  dayMonth,
  lateDocumentKind,
  lateSentence,
  lateStatusTone,
} from "./late-document-text";

const t = (key: string, vars?: Record<string, string | number>) =>
  key === "office.late_docs.sentence"
    ? `Bu belge dönem kapandıktan sonra geldi (${vars?.month} kapanışı: ${vars?.closed}; geliş: ${vars?.arrived})`
    : key === "office.late_docs.kind_expense"
      ? "Masraf fişi"
      : key === "office.late_docs.kind_other"
        ? "Belge"
        : key;

describe("geç gelen belge cümlesi (#780 C)", () => {
  it("gün ve ay İstanbul saatine göre", () => {
    expect(dayMonth("2026-11-03T07:00:00Z", "tr")).toBe("3 Kasım");
    // 17 Kasım 22:30 UTC İstanbul'da 18 Kasım'dır.
    expect(dayMonth("2026-11-17T22:30:00Z", "tr")).toBe("18 Kasım");
  });
  it("kapanış ve geliş ayrı yazılır", () => {
    expect(
      lateSentence(
        {
          period: "2026-10",
          closed_at: "2026-11-03T07:00:00Z",
          arrived_at: "2026-11-18T06:00:00Z",
        },
        t,
        "tr",
      ),
    ).toBe(
      "Bu belge dönem kapandıktan sonra geldi (Ekim 2026 kapanışı: 3 Kasım; geliş: 18 Kasım)",
    );
  });
  it("durum tonu ve tür adı", () => {
    expect(lateStatusTone("pending")).toBe("warn");
    expect(lateStatusTone("current_period")).toBe("ok");
    expect(lateStatusTone("ignored")).toBe("neutral");
    expect(lateDocumentKind("expense", t)).toBe("Masraf fişi");
    expect(lateDocumentKind("payroll", t)).toBe("Belge");
  });
});
