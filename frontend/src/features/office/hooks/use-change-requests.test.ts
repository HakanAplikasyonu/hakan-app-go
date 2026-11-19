import { describe, expect, it } from "vitest";

import { readDiff } from "./use-change-requests";

describe("readDiff", () => {
  it("reads source and changed fields", () => {
    expect(
      readDiff({
        kind: "update",
        source: { system: "parasut", capture_method: "api" },
        fields: {
          total: { old: "100.00", new: "120.00" },
          note: { old: null, new: "x" },
        },
      }),
    ).toEqual({
      source: "parasut",
      method: "api",
      fields: [
        { field: "note", old: "—", new: "x" },
        { field: "total", old: "100.00", new: "120.00" },
      ],
    });
  });

  it("survives an unknown shape", () => {
    expect(readDiff(undefined)).toEqual({ source: "", method: "", fields: [] });
  });
});
