import { describe, expect, it } from "vitest";

import { lastDays } from "../components/engagement-connections-page";
import { bankStep, splitIBANs } from "./use-bank-connections";

describe("bankStep", () => {
  it("follows the setup states", () => {
    expect(bankStep({ setup_state: "draft" })).toBe("instruction");
    expect(bankStep({ setup_state: "form_ready" })).toBe("submit");
    expect(bankStep({ setup_state: "submitted_to_bank" })).toBe("credentials");
    expect(bankStep({ setup_state: "verifying" })).toBe("credentials");
    expect(bankStep({ setup_state: "active" })).toBe("active");
  });
  it("asks for the service user again after a failure", () => {
    expect(bankStep({ setup_state: "failed" })).toBe("credentials");
  });
});

describe("splitIBANs", () => {
  it("drops spaces, upper-cases and dedupes", () => {
    expect(
      splitIBANs(
        "tr33 0006 1005 1978 6457 8413 26\nTR330006100519786457841326, ;\n",
      ),
    ).toEqual(["TR330006100519786457841326"]);
  });
  it("is empty for blank input", () => {
    expect(splitIBANs("  \n ")).toEqual([]);
  });
});

describe("lastDays", () => {
  it("spans seven days ending today", () => {
    expect(lastDays(7, new Date(2026, 10, 3))).toEqual({
      from: "2026-10-28",
      to: "2026-11-03",
    });
  });
});
