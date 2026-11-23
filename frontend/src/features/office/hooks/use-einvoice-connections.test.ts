import { describe, expect, it } from "vitest";

import {
  cleanValues,
  isLoadingHistory,
  isWaiting,
} from "./use-einvoice-connections";

describe("cleanValues", () => {
  it("trims values and drops empty ones", () => {
    expect(
      cleanValues({
        username: " ofis ",
        password: "s3cret",
        sender_alias: "  ",
      }),
    ).toEqual({ username: "ofis", password: "s3cret" });
  });
});

describe("isWaiting", () => {
  it("is true only while the first sync runs", () => {
    expect(isWaiting({ state: "waiting" })).toBe(true);
    expect(isWaiting({ state: "active" })).toBe(false);
    expect(isWaiting({ state: "draft" })).toBe(false);
  });
});

describe("isLoadingHistory", () => {
  const p = {
    recent_ready: false,
    this_month_documents: 0,
    this_month_invoices: 0,
    history_months_done: 0,
    history_months_total: 21,
  };
  it("is true until the first load is done", () => {
    expect(isLoadingHistory({ progress: { ...p, phase: "queued" } })).toBe(
      true,
    );
    expect(isLoadingHistory({ progress: { ...p, phase: "history" } })).toBe(
      true,
    );
    expect(isLoadingHistory({ progress: { ...p, phase: "done" } })).toBe(false);
    expect(isLoadingHistory({})).toBe(false);
  });
});
