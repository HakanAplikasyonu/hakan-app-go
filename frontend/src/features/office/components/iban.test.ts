import { describe, expect, it } from "vitest";

import { bankFromIban, compactIban, formatIban, isValidTrIban } from "./iban";

describe("TR IBAN", () => {
  it("accepts a valid IBAN with spaces and lower case", () => {
    expect(isValidTrIban("tr76 0001 0005 1978 6457 8413 26")).toBe(true);
    expect(isValidTrIban("TR660006200000000001002222")).toBe(true);
  });

  it("rejects a wrong check digit, length or country", () => {
    expect(isValidTrIban("TR760001000519786457841327")).toBe(false);
    expect(isValidTrIban("TR7600010005197864578413")).toBe(false);
    expect(isValidTrIban("DE89370400440532013000")).toBe(false);
    expect(isValidTrIban("")).toBe(false);
  });

  it("reads the bank from digits 5-9", () => {
    expect(bankFromIban("TR660006200000000001002222")).toBe("garanti");
    expect(bankFromIban("TR27 0020 5000 0000 0123 4567 89")).toBe(
      "kuveyt_turk",
    );
    expect(bankFromIban("TR12 0099 9")).toBeNull();
  });

  it("formats in groups of four", () => {
    expect(formatIban(compactIban("tr660006200000000001002222"))).toBe(
      "TR66 0006 2000 0000 0001 0022 22",
    );
  });
});
