import { describe, expect, it } from "vitest";

import { parseApiError } from "./errors";

/**
 * M15 uçlarının (#455) İngilizce sunucu mesajı kullanıcıya gösterilmez:
 * kodun Türkçe karşılığı kullanılır.
 */
describe("parseApiError: M15 hata kodları Türkçe", () => {
  const cases: [string, string, RegExp][] = [
    [
      "PAYROLL_RUN_LOCKED",
      "the payroll's journal entry is approved; reverse it first",
      /fişi onaylandı/,
    ],
    [
      "STATEMENT_BALANCE_MISMATCH",
      "statement: closing balance does not match",
      /bakiye/,
    ],
    [
      "RECONCILIATION_IN_REVIEW",
      "reconciliation: the item is waiting in the review queue",
      /inceleme kuyruğunda/,
    ],
    [
      "TAX_RETURN_NOT_DRAFT",
      "the return is prepared or filed; reopen it first",
      /taslağa geri alın/,
    ],
    [
      "EASYTRADE_AUTH_FAILED",
      "Easytrade refused the login or token",
      /Easytrade girişi reddetti/,
    ],
    [
      "PAIRING_CODE_INVALID",
      "This pairing code has expired or was already used",
      /yeni kod/,
    ],
  ];
  for (const [code, message, expected] of cases) {
    it(code, () => {
      const err = parseApiError(409, { error: { code, message } });
      expect(err.code).toBe(code);
      expect(err.message).toMatch(expected);
      expect(err.message).not.toBe(message);
    });
  }

  it("Türkçe sunucu mesajını olduğu gibi bırakır", () => {
    const err = parseApiError(422, {
      error: {
        code: "PAYROLL_RUN_LOCKED",
        message: "Bordro kilitli; önce fişi geri alın.",
      },
    });
    expect(err.message).toBe("Bordro kilitli; önce fişi geri alın.");
  });
});
