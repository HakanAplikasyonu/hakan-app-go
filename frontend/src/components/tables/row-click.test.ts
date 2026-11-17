import { describe, expect, it } from "vitest";

import { isRowKeyActivation } from "./row-click";

describe("isRowKeyActivation", () => {
  const row = {};
  it("satır odaktayken Enter ve Boşluk satırı açar", () => {
    expect(
      isRowKeyActivation({
        key: "Enter",
        target: row as EventTarget,
        currentTarget: row as EventTarget,
      }),
    ).toBe(true);
    expect(
      isRowKeyActivation({
        key: " ",
        target: row as EventTarget,
        currentTarget: row as EventTarget,
      }),
    ).toBe(true);
  });

  it("içteki denetimden gelen tuş satırı açmaz", () => {
    const button = {};
    expect(
      isRowKeyActivation({
        key: "Enter",
        target: button as EventTarget,
        currentTarget: row as EventTarget,
      }),
    ).toBe(false);
  });

  it("diğer tuşlar ve engellenmiş olay", () => {
    expect(
      isRowKeyActivation({
        key: "a",
        target: row as EventTarget,
        currentTarget: row as EventTarget,
      }),
    ).toBe(false);
    expect(
      isRowKeyActivation({
        key: "Enter",
        target: row as EventTarget,
        currentTarget: row as EventTarget,
        defaultPrevented: true,
      }),
    ).toBe(false);
  });
});
