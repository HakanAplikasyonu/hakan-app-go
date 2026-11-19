import { beforeEach, describe, expect, it, vi } from "vitest";

const tenantRequest = vi.fn();
vi.mock("@/lib/api/tenant-request", () => ({
  tenantRequest: (...args: unknown[]) => tenantRequest(...args),
}));

const { bankMatchingKeys, bankMatchingService } =
  await import("./bank-matching.service");

const base = "/v1/tenant/engagements/e1/bank-transactions/t1";

describe("bankMatchingService (#528)", () => {
  beforeEach(() => tenantRequest.mockReset());

  it("reads the movement with its candidates", () => {
    void bankMatchingService.get("demo", "e1", "t1");
    expect(tenantRequest).toHaveBeenCalledWith(
      "demo",
      "GET",
      `${base}/matching`,
    );
  });
  it("matches the chosen invoices and undoes", () => {
    void bankMatchingService.match("demo", "e1", "t1", ["i1", "i2"]);
    expect(tenantRequest).toHaveBeenCalledWith(
      "demo",
      "PUT",
      `${base}/matches`,
      {
        body: { invoice_uuids: ["i1", "i2"] },
      },
    );
    void bankMatchingService.unmatch("demo", "e1", "t1");
    expect(tenantRequest).toHaveBeenLastCalledWith(
      "demo",
      "DELETE",
      `${base}/matches`,
    );
  });
  it("keys per office, engagement and movement", () => {
    expect(bankMatchingKeys.one("Demo", "e1", "t1")).toEqual([
      "tenant",
      "demo",
      "bank-matching",
      "e1",
      "t1",
    ]);
  });
});
