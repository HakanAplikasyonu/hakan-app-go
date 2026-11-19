import { describe, expect, it } from "vitest";

import {
  duplicateFields,
  excelBusy,
  excelStep,
  missingRequired,
} from "./excel-import";

describe("excelStep", () => {
  it("maps server statuses to wizard steps", () => {
    expect(excelStep("analyzing")).toBe("working");
    expect(excelStep("mapping_review")).toBe("mapping");
    expect(excelStep("has_errors")).toBe("rows");
    expect(excelStep("preview")).toBe("ready");
    expect(excelStep("committed")).toBe("done");
    expect(excelStep("failed")).toBe("done");
    expect(excelStep("rolled_back")).toBe("done");
  });

  it("polls only while the server works", () => {
    expect(excelBusy("validating")).toBe(true);
    expect(excelBusy("preview")).toBe(false);
  });
});

describe("missingRequired", () => {
  const required = [["date"], ["total", "net"]];

  it("accepts one mapped field per group", () => {
    expect(
      missingRequired(required, [
        { column: "A", field: "date" },
        { column: "B", field: "net" },
        { column: "C", field: "" },
      ]),
    ).toEqual([]);
  });

  it("reports the groups without a mapped field", () => {
    expect(missingRequired(required, [{ column: "A", field: "date" }])).toEqual(
      [["total", "net"]],
    );
  });
});

describe("duplicateFields", () => {
  it("finds fields mapped to more than one column", () => {
    expect(
      duplicateFields([
        { column: "A", field: "date" },
        { column: "B", field: "date" },
        { column: "C", field: "" },
        { column: "D", field: "" },
        { column: "E", field: "total" },
      ]),
    ).toEqual(["date"]);
  });
});
