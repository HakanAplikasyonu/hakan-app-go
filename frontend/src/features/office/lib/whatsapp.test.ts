import { describe, expect, it } from "vitest";

import { formatChatKey, receiptStatusKey } from "./whatsapp";

describe("formatChatKey", () => {
  it("formats Turkish mobile numbers", () => {
    expect(formatChatKey("905321234567")).toBe("+90 532 123 45 67");
  });
  it("keeps other numbers as digits", () => {
    expect(formatChatKey("4915112345678")).toBe("+4915112345678");
  });
  it("names groups and hidden numbers", () => {
    expect(formatChatKey("g:120363041234567890")).toBe("grup");
    expect(formatChatKey("l:98765432101234")).toBe("gizli numara");
  });
});

describe("receiptStatusKey", () => {
  it("maps known statuses and falls back", () => {
    expect(receiptStatusKey("draft")).toBe("office.whatsapp.receipt_draft");
    expect(receiptStatusKey("x")).toBe("office.whatsapp.result_none");
  });
});
