import { describe, expect, it } from "vitest";

import { exportIssueText, parseIssueField } from "./export-issue";

const catalog: Record<string, string> = {
  "office.journals.issue_ACCOUNT_NOT_IN_TARGET_PLAN":
    "{{account}} hesabı programın hesap planında yok.",
  "office.journals.issue_ACCOUNT_NOT_IN_TARGET_PLAN_group":
    "{{account}} bir grup hesabı; alt hesap açın.",
  "office.journals.issue_JOURNAL_UNBALANCED":
    "Borç {{debit}}, alacak {{credit}}; fark {{difference}}.",
  "office.journals.issue_ENTRY_TYPE_UNSUPPORTED":
    "{{entry_type}} fişi bu programa aktarılamıyor.",
  "office.journals.entry_type_acilis": "Açılış",
  "office.journals.issue_at_line": "Fiş {{entry}}, satır {{line}}",
  "office.journals.issue_at_entry": "Fiş {{entry}}",
  "office.journals.issue_unknown": "Fiş aktarıma uygun değil.",
};
const t = (key: string, vars?: Record<string, string | number>) => {
  const s = catalog[key];
  if (s === undefined) return key;
  return s.replace(/\{\{(\w+)\}\}/g, (_, k) => String(vars?.[k] ?? ""));
};

describe("export issue text (#455)", () => {
  it("parses the entry / line field", () => {
    expect(parseIssueField("entry:Y-12/line:2")).toEqual({
      entry: "Y-12",
      line: 2,
    });
    expect(parseIssueField("entry:Y-12")).toEqual({
      entry: "Y-12",
      line: undefined,
    });
    expect(parseIssueField("")).toEqual({});
  });

  it("never shows the raw code, field or JSON", () => {
    const text = exportIssueText(
      {
        code: "ACCOUNT_NOT_IN_TARGET_PLAN",
        field: "entry:Y-12/line:2",
        message: '{"account":"600.01"}',
      },
      t,
    );
    expect(text).toBe(
      "Fiş Y-12, satır 2: 600.01 hesabı programın hesap planında yok.",
    );
    expect(text).not.toMatch(/[{}]|entry:|ACCOUNT_/);
  });

  it("asks for a sub-account when the account is a group account", () => {
    expect(
      exportIssueText(
        {
          code: "ACCOUNT_NOT_IN_TARGET_PLAN",
          field: "entry:Y-6/line:4",
          message: '{"account":"391","reason":"group_account"}',
        },
        t,
      ),
    ).toBe("Fiş Y-6, satır 4: 391 bir grup hesabı; alt hesap açın.");
  });

  it("formats amounts tr-TR", () => {
    expect(
      exportIssueText(
        {
          code: "JOURNAL_UNBALANCED",
          field: "entry:Y-3",
          message:
            '{"debit":"1250.00","credit":"1000.00","difference":"250.00"}',
        },
        t,
      ),
    ).toBe("Fiş Y-3: Borç 1.250,00 TL, alacak 1.000,00 TL; fark 250,00 TL.");
  });

  it("names the entry type and falls back for unknown codes", () => {
    expect(
      exportIssueText(
        { code: "ENTRY_TYPE_UNSUPPORTED", message: '{"entry_type":"acilis"}' },
        t,
      ),
    ).toBe("Açılış fişi bu programa aktarılamıyor.");
    expect(
      exportIssueText({ code: "SOMETHING_NEW", message: "blocker" }, t),
    ).toBe("Fiş aktarıma uygun değil.");
  });
});
