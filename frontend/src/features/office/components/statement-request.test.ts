import { describe, expect, it } from "vitest";

import { latestRequest } from "./statement-request";

const task = (
  title: string,
  status: string,
  created_at: string,
  kind = "document_request",
) => ({ kind, title, status, created_at });

describe("latestRequest", () => {
  const title = "Ekim 2026 Ziraat TL banka ekstresi";

  it("aynı ekstrenin en son, iptal edilmemiş isteğini bulur", () => {
    const items = [
      task(title, "open", "2026-11-01T09:00:00Z"),
      task(title, "submitted", "2026-11-05T09:00:00Z"),
      task(title, "cancelled", "2026-11-09T09:00:00Z"),
      task("Başka belge", "open", "2026-11-10T09:00:00Z"),
      task(title, "open", "2026-11-11T09:00:00Z", "undocumented_payment"),
    ];
    expect(latestRequest(items, title)?.created_at).toBe(
      "2026-11-05T09:00:00Z",
    );
  });

  it("istek yoksa boş döner", () => {
    expect(
      latestRequest([task(title, "cancelled", "2026-11-01")], title),
    ).toBeUndefined();
    expect(latestRequest([], title)).toBeUndefined();
  });
});
