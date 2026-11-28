import { describe, expect, it } from "vitest";

import {
  cleanValues,
  connectionErrorKey,
  isLoadingHistory,
  isWaiting,
  testOutcomeKey,
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

describe("connectionErrorKey", () => {
  it("says wrong password only for a real login refusal", () => {
    const conn = (state: string, kind?: string, ip_filtered = false) => ({
      state,
      ip_filtered,
      error: kind ? { kind } : null,
    });
    expect(connectionErrorKey(conn("auth_failed", "auth"))).toBe(
      "auth_failed",
    );
    // EDM: the user has no web-service right.
    expect(connectionErrorKey(conn("auth_failed", "permission"))).toBe(
      "test_permission",
    );
    // İşNet has no user: an unanswered call means the IP is not registered.
    expect(connectionErrorKey(conn("auth_failed", "permission", true))).toBe(
      "ip_not_allowed",
    );
    expect(connectionErrorKey(conn("auth_failed", undefined, true))).toBe(
      "ip_not_allowed",
    );
    expect(connectionErrorKey(conn("active", "transport"))).toBe(
      "test_unavailable",
    );
    expect(connectionErrorKey(conn("active", "validation"))).toBe(
      "last_error",
    );
  });
});

describe("testOutcomeKey", () => {
  it("speaks of IP and VKN for an IP-filtered integrator", () => {
    expect(testOutcomeKey({ ok: true }, false)).toBe("test_ok");
    expect(testOutcomeKey({ ok: true }, true)).toBe("test_ok_ip");
    expect(testOutcomeKey({ ok: false, kind: "permission" }, true)).toBe(
      "ip_not_allowed",
    );
    expect(testOutcomeKey({ ok: false, kind: "permission" }, false)).toBe(
      "test_permission",
    );
    expect(testOutcomeKey({ ok: false, kind: "auth" }, false)).toBe(
      "test_auth",
    );
  });
});
