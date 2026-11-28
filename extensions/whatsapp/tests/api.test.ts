import { describe, expect, it } from "vitest";

import { Api, ApiError, describeError, normalizeBase, type KV } from "../src/api";

function memory(): KV & { data: Map<string, unknown> } {
  const data = new Map<string, unknown>();
  return {
    data,
    get: async (k) => data.get(k),
    set: async (k, v) => void data.set(k, v),
    remove: async (k) => void data.delete(k),
  };
}

const ok = (data: unknown, status = 200) => new Response(JSON.stringify({ success: true, data, meta: {} }), { status });
const fail = (status: number, code: string) =>
  new Response(JSON.stringify({ success: false, error: { code, message: code }, meta: {} }), { status });

interface Call {
  url: string;
  init: RequestInit;
}

function fakeFetch(handler: (c: Call) => Response) {
  const calls: Call[] = [];
  const fn = (async (url: string, init: RequestInit = {}) => {
    const c = { url: String(url), init };
    calls.push(c);
    return handler(c);
  }) as unknown as typeof fetch;
  return { fn, calls };
}

const paired = {
  access_token: "a1",
  refresh_token: "r1",
  expires_in: 900,
  user: { uuid: "u", email: "sahip@ofis.test", name: "Mert" },
  organization: { uuid: "o", slug: "demo-smmm", name: "Demir" },
};

describe("normalizeBase", () => {
  it("keeps https and strips /v1 and slashes", () => {
    expect(normalizeBase(" https://api.ofis.test/v1/ ")).toBe("https://api.ofis.test");
    expect(normalizeBase("http://localhost:8080")).toBe("http://localhost:8080");
  });
  it("refuses plain http to a remote server", () => {
    expect(() => normalizeBase("http://api.ofis.test")).toThrow(ApiError);
    expect(() => normalizeBase("ofis")).toThrow(ApiError);
  });
});

describe("Api", () => {
  it("asks capture statuses by file hash and saves the chat number as phone", async () => {
    const local = memory();
    const session = memory();
    const f = fakeFetch((c) => (c.url.endsWith("/v1/auth/whatsapp/pair") ? ok(paired) : c.url.includes("/captures/status") ? ok({ items: [{ sha256: "a" }] }) : ok({})));
    const api = new Api(local, session, f.fn, () => 1_000);
    await api.pair("https://api.ofis.test/", "k7qm-3xpa", "Ofis");
    expect(await api.statuses([])).toEqual([]);
    expect(await api.statuses(["a".repeat(64), "b".repeat(64)])).toEqual([{ sha256: "a" }]);
    expect(f.calls.at(-1)!.url).toBe(`https://api.ofis.test/v1/tenant/whatsapp/captures/status?sha256=${"a".repeat(64)}%2C${"b".repeat(64)}`);
    await api.savePhone("eng-1", "+905321234567");
    const put = f.calls.at(-1)!;
    expect(put.url).toBe("https://api.ofis.test/v1/tenant/engagements/eng-1/contacts");
    expect(put.init.method).toBe("PUT");
    expect(JSON.parse(String(put.init.body))).toEqual({ phone: "+905321234567" });
  });

  it("pairs, then calls tenant endpoints with the token and office", async () => {
    const local = memory();
    const session = memory();
    const f = fakeFetch((c) => (c.url.endsWith("/v1/auth/whatsapp/pair") ? ok(paired) : ok({ chat_key: "905", link: null, suggestions: [] })));
    const api = new Api(local, session, f.fn, () => 1_000);
    const st = await api.pair("https://api.ofis.test/", "k7qm-3xpa", "Ofis");
    expect(st).toEqual({ paired: true, apiBase: "https://api.ofis.test", organization: { slug: "demo-smmm", name: "Demir" }, user: { email: "sahip@ofis.test", name: "Mert" } });
    expect(JSON.parse(String(f.calls[0]!.init.body))).toEqual({ code: "k7qm-3xpa", device_name: "Ofis" });

    await api.lookup("905321234567@c.us");
    const c = f.calls[1]!;
    expect(c.url).toBe("https://api.ofis.test/v1/tenant/whatsapp/chat-lookup?chat_key=905321234567%40c.us");
    expect(c.init.headers).toMatchObject({ Authorization: "Bearer a1", "X-Organization": "demo-smmm" });
  });

  it("refreshes once on 401 and keeps the rotated refresh token", async () => {
    const local = memory();
    const session = memory();
    let first = true;
    const f = fakeFetch((c) => {
      if (c.url.endsWith("/pair")) return ok(paired);
      if (c.url.endsWith("/v1/auth/refresh")) return ok({ access_token: "a2", refresh_token: "r2", expires_in: 900 });
      if (first) {
        first = false;
        return fail(401, "UNAUTHENTICATED");
      }
      return ok({ items: [] });
    });
    const api = new Api(local, session, f.fn, () => 1_000);
    await api.pair("https://api.ofis.test", "K7QM3XPA", "");
    await api.openTasks("eng-1");
    const urls = f.calls.map((c) => c.url.replace("https://api.ofis.test", ""));
    expect(urls).toEqual([
      "/v1/auth/whatsapp/pair",
      "/v1/tenant/engagements/eng-1/document-tasks?status=open&limit=20",
      "/v1/auth/refresh",
      "/v1/tenant/engagements/eng-1/document-tasks?status=open&limit=20",
    ]);
    expect(JSON.parse(String(f.calls[2]!.init.body))).toEqual({ refresh_token: "r1" });
    expect((local.data.get("session") as { refreshToken: string }).refreshToken).toBe("r2");
    expect(f.calls[3]!.init.headers).toMatchObject({ Authorization: "Bearer a2" });
  });

  it("signs out when the refresh token is refused", async () => {
    const local = memory();
    const f = fakeFetch((c) => (c.url.endsWith("/pair") ? ok({ ...paired, expires_in: 0 }) : fail(401, "UNAUTHENTICATED")));
    const api = new Api(local, memory(), f.fn, () => 1_000);
    await api.pair("https://api.ofis.test", "K7QM3XPA", "");
    const err = await api.search("atlas").catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect((err as ApiError).code).toBe("SIGNED_OUT");
    expect(local.data.has("session")).toBe(false);
    expect((await api.status()).paired).toBe(false);
  });

  it("sends the clicked file as multipart", async () => {
    const f = fakeFetch((c) => (c.url.endsWith("/pair") ? ok(paired) : ok({ uuid: "c1" }, 201)));
    const api = new Api(memory(), memory(), f.fn, () => 1_000);
    await api.pair("https://api.ofis.test", "K7QM3XPA", "");
    await api.capture("eng-1", "document", "905321234567@c.us", new Blob(["%PDF-1.4"], { type: "application/pdf" }), "kira.pdf", "task-1");
    const form = f.calls[1]!.init.body as FormData;
    expect(f.calls[1]!.url).toBe("https://api.ofis.test/v1/tenant/engagements/eng-1/whatsapp/captures");
    expect(form.get("target")).toBe("document");
    expect(form.get("task_uuid")).toBe("task-1");
    expect((form.get("file") as File).name).toBe("kira.pdf");
    expect(Array.from(form.keys()).sort()).toEqual(["chat_key", "file", "target", "task_uuid"]);
  });
});

describe("describeError", () => {
  it("maps API codes to Turkish", () => {
    expect(describeError(new ApiError("x", 415, "IMAGE_REQUIRED")).message).toMatch(/PDF'i belge olarak/);
    expect(describeError(new ApiError("x", 403)).message).toBe("Bu işlem için yetkiniz yok.");
    expect(describeError(new TypeError("fetch failed")).message).toMatch(/Sunucuya ulaşılamadı/);
  });
});
