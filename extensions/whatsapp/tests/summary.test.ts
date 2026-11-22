import { describe, expect, it } from "vitest";

import { Api, type KV } from "../src/api";
import {
  badgeText,
  badgeTitle,
  guessWebBase,
  inboxBreakdown,
  links,
  taskItems,
  vatItems,
  type InboxCounts,
} from "../src/summary";

const counts: InboxCounts = {
  invoice: 3,
  bank_transaction: 4,
  expense: 0,
  retail_sale: 1,
  foreign_invoice: 0,
  late_document: 2,
  total: 10,
};

const l = links("https://app.ofis.test/", "demo smmm");

describe("badgeText", () => {
  it("hides zero, caps at 99+", () => {
    expect(badgeText(0)).toBe("");
    expect(badgeText(null)).toBe("");
    expect(badgeText(undefined)).toBe("");
    expect(badgeText(-2)).toBe("");
    expect(badgeText(7)).toBe("7");
    expect(badgeText(99)).toBe("99");
    expect(badgeText(100)).toBe("99+");
  });
});

describe("badgeTitle and inboxBreakdown", () => {
  it("lists only non-zero types", () => {
    expect(badgeTitle("HAKAN", counts)).toBe("HAKAN\nGelen kutusu: 10 (Fatura 3, Banka 4, Z raporu 1, Geç gelen belge 2)");
    expect(inboxBreakdown(counts)).toBe("Fatura 3 · Banka 4 · Z raporu 1 · Geç gelen belge 2");
  });
  it("handles empty and unknown inbox", () => {
    expect(badgeTitle("HAKAN", null)).toBe("HAKAN");
    expect(badgeTitle("HAKAN", { ...counts, invoice: 0, bank_transaction: 0, retail_sale: 0, late_document: 0, total: 0 })).toBe(
      "HAKAN\nGelen kutusu boş",
    );
  });
});

describe("guessWebBase", () => {
  it("maps api. to app. and strips /api and /v1", () => {
    expect(guessWebBase("https://api.ofis.test")).toBe("https://app.ofis.test");
    expect(guessWebBase("https://ofis.test/api")).toBe("https://ofis.test");
    expect(guessWebBase("https://ofis.test/api/v1/")).toBe("https://ofis.test");
    expect(guessWebBase("http://localhost:8080")).toBe("http://localhost:8080");
    expect(guessWebBase("bozuk")).toBe("");
  });
});

describe("links", () => {
  it("builds web app screens under /t/{slug}", () => {
    expect(l.inbox).toBe("https://app.ofis.test/t/demo%20smmm/review?view=inbox");
    expect(l.reports).toBe("https://app.ofis.test/t/demo%20smmm/reports");
    expect(l.documentCollection).toBe("https://app.ofis.test/t/demo%20smmm/document-collection");
    expect(l.engagementTasks("e1")).toBe("https://app.ofis.test/t/demo%20smmm/engagements/e1#belge-istekleri");
  });
});

describe("vatItems", () => {
  it("maps events to Turkish labels, max 5, linking to reports", () => {
    const ev = (i: number, event: string) => ({
      uuid: `v${i}`,
      period: "2026-11",
      event,
      engagement_uuid: "e",
      taxpayer_title: `Firma ${i}`,
      created_at: "2026-11-20T09:00:00Z",
    });
    const items = vatItems(
      [ev(1, "vat.threshold_exceeded"), ev(2, "vat.report_ready"), ev(3, "vat.status_changed"), ev(4, "x"), ev(5, "x"), ev(6, "x")],
      l,
    );
    expect(items).toHaveLength(5);
    expect(items[0]!.title).toBe("Firma 1");
    expect(items[0]!.detail.startsWith("KDV eşiği aşıldı · 2026-11 · ")).toBe(true);
    expect(items[1]!.detail.startsWith("KDV raporu hazır")).toBe(true);
    expect(items[2]!.detail.startsWith("KDV durumu değişti")).toBe(true);
    expect(items[3]!.detail.startsWith("KDV uyarısı")).toBe(true);
    expect(items[0]!.url).toBe(l.reports);
  });
});

describe("taskItems", () => {
  it("keeps clients with open tasks and links to their document requests", () => {
    const items = taskItems(
      [
        { engagement_uuid: "e1", title: "Ayşe Yılmaz", open_tasks: 2 },
        { engagement_uuid: "e2", title: "Boş Ltd", open_tasks: 0 },
      ],
      l,
    );
    expect(items).toEqual([{ title: "Ayşe Yılmaz", detail: "2 açık belge isteği", url: l.engagementTasks("e1") }]);
  });
});

function memory(init: Record<string, unknown> = {}): KV {
  const data = new Map<string, unknown>(Object.entries(init));
  return {
    get: async (k) => data.get(k),
    set: async (k, v) => void data.set(k, v),
    remove: async (k) => void data.delete(k),
  };
}

const ok = (data: unknown) => new Response(JSON.stringify({ success: true, data, meta: {} }), { status: 200 });
const fail = (status: number) => new Response(JSON.stringify({ success: false, error: { code: "X", message: "X" }, meta: {} }), { status });

describe("Api.summary", () => {
  it("reads the three office endpoints and nulls the ones it may not read", async () => {
    const local = memory({
      session: {
        apiBase: "https://api.ofis.test",
        refreshToken: "r",
        organization: { uuid: "o", slug: "demo", name: "Demo" },
        user: { uuid: "u", email: "a@ofis.test", name: "A" },
      },
    });
    const session = memory({ access: { token: "t", expiresAt: 10_000 } });
    const urls: string[] = [];
    const fetchFn = (async (url: string) => {
      urls.push(String(url));
      if (String(url).includes("/v1/tenant/inbox")) return ok({ counts, items: [] });
      if (String(url).includes("/vat-forecast/watch-events")) return fail(403);
      return ok({ items: [{ engagement_uuid: "e1", title: "Ayşe", open_tasks: 1 }] });
    }) as unknown as typeof fetch;
    const api = new Api(local, session, fetchFn, () => 1_000);
    const s = await api.summary();
    expect(urls.sort()).toEqual([
      "https://api.ofis.test/v1/tenant/clients?filter=tasks&limit=5",
      "https://api.ofis.test/v1/tenant/inbox?limit=1",
      "https://api.ofis.test/v1/tenant/vat-forecast/watch-events?limit=5",
    ]);
    expect(s.webBase).toBe("https://app.ofis.test");
    expect(s.inbox).toEqual({ counts, url: "https://app.ofis.test/t/demo/review?view=inbox" });
    expect(s.vat).toBeNull();
    expect(s.tasks?.items).toEqual([{ title: "Ayşe", detail: "1 açık belge isteği", url: "https://app.ofis.test/t/demo/engagements/e1#belge-istekleri" }]);
    expect(s.fetchedAt).toBe(1_000);

    expect(await api.setWebBase("https://panel.ofis.test/")).toBe("https://panel.ofis.test");
    expect(await api.setWebBase("")).toBe("https://app.ofis.test");
  });
});
