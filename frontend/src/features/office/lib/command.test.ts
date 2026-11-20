import { describe, expect, it } from "vitest";

import { cellHref, cellTarget } from "./command";

const cell = (
  open_blockers: number,
  open_warnings: number,
  open_change_requests: number,
) => ({ open_blockers, open_warnings, open_change_requests });

describe("cellTarget (#567)", () => {
  it("opens the blockers when the period has any", () => {
    expect(cellTarget(cell(2, 7, 1))).toBe("blockers");
  });
  it("opens the change requests for an open change request (#710)", () => {
    expect(cellTarget(cell(0, 3, 1))).toBe("change_requests");
  });
  it("opens the warning list when only non-blocking warnings are open", () => {
    expect(cellTarget(cell(0, 3, 0))).toBe("warnings");
  });
  it("opens the overview when nothing is open", () => {
    expect(cellTarget(cell(0, 0, 0))).toBe("overview");
  });
});

describe("cellHref (#710)", () => {
  it("goes to the screen of the work", () => {
    expect(cellHref("ofis", "e1", "blockers")).toBe(
      "/t/ofis/engagements/e1/warnings?severity=blocker",
    );
    expect(cellHref("ofis", "e1", "warnings")).toBe(
      "/t/ofis/engagements/e1/warnings",
    );
    expect(cellHref("ofis", "e1", "change_requests")).toBe(
      "/t/ofis/engagements/e1?talepler=ac",
    );
    expect(cellHref("ofis", "e1", "overview")).toBe("/t/ofis/engagements/e1");
  });
});
