import { describe, expect, it } from "vitest";

import { DOUBLE_BOOKING_CODES, doubleBookingText } from "./double-booking";

const t = (key: string, vars?: Record<string, string | number>) =>
  `${key}${vars ? JSON.stringify(vars) : ""}`;

describe("double booking issue text (#466)", () => {
  it("names the earlier file and entry", () => {
    const text = doubleBookingText(t, "tr", {
      code: "DOCUMENT_ALREADY_EXPORTED",
      severity: "warning",
      params: {
        previous_file: "luca_fis_2026-10.xlsx",
        previous_entry: "2026-10/3",
        previous_exported_at: "2026-10-20T09:30:00Z",
      },
    });
    expect(text).toContain("office.journals.dup.already");
    expect(text).toContain("luca_fis_2026-10.xlsx");
    expect(text).toContain("2026-10/3");
  });
  it("points at the entry of the same package", () => {
    expect(
      doubleBookingText(t, "tr", {
        code: "DOCUMENT_ALREADY_EXPORTED",
        severity: "warning",
        params: { same_export_entry: "4" },
      }),
    ).toBe('office.journals.dup.same_export{"entry":"4"}');
  });
  it("names the integrator Luca fetches from", () => {
    expect(
      doubleBookingText(t, "tr", {
        code: "PROGRAM_PULLS_DOCUMENT",
        severity: "warning",
        params: { integrator: "isnet" },
      }),
    ).toBe(
      'office.journals.dup.pulls{"integrator":"office.journals.integrator_isnet"}',
    );
  });
  it("knows both guard codes", () => {
    expect([...DOUBLE_BOOKING_CODES].sort()).toEqual([
      "DOCUMENT_ALREADY_EXPORTED",
      "PROGRAM_PULLS_DOCUMENT",
    ]);
  });
});
