import { describe, expect, it } from "vitest";

import { cellTarget } from "./command";

const cell = (
  open_blockers: number,
  open_warnings: number,
  open_change_requests: number,
) => ({ open_blockers, open_warnings, open_change_requests });

describe("cellTarget (#567)", () => {
  it("opens the blockers when the period has any", () => {
    expect(cellTarget(cell(2, 7, 1))).toBe("blockers");
  });
  it("opens the overview for an open change request", () => {
    expect(cellTarget(cell(0, 3, 1))).toBe("overview");
  });
  it("opens the warning list when only non-blocking warnings are open", () => {
    expect(cellTarget(cell(0, 3, 0))).toBe("warnings");
  });
  it("opens the overview when nothing is open", () => {
    expect(cellTarget(cell(0, 0, 0))).toBe("overview");
  });
});
