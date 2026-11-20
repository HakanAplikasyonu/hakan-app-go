import { describe, expect, it } from "vitest";

import { ApiError } from "@/lib/api";

import { queryOrNull } from "./or-null";

const apiError = (status: number) =>
  new ApiError({ status, code: "X", message: "x" });

describe("queryOrNull (#639)", () => {
  it("başarılı yanıtı olduğu gibi döner", async () => {
    await expect(queryOrNull(async () => ({ run: 1 }))).resolves.toEqual({
      run: 1,
    });
  });

  it("404'ü null veri sayar", async () => {
    await expect(
      queryOrNull(async () => {
        throw apiError(404);
      }),
    ).resolves.toBeNull();
  });

  it("diğer API hatalarını ve ağ hatalarını fırlatır", async () => {
    for (const status of [400, 403, 409, 500, 503]) {
      await expect(
        queryOrNull(async () => {
          throw apiError(status);
        }),
      ).rejects.toMatchObject({ status });
    }
    await expect(
      queryOrNull(async () => {
        throw new TypeError("Failed to fetch");
      }),
    ).rejects.toBeInstanceOf(TypeError);
  });
});
