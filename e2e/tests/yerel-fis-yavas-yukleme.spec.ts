import { request as httpRequest } from "node:http";
import { request as httpsRequest } from "node:https";

import { expect, test } from "@playwright/test";

import { addConsentedTaxpayerWithUser, API_URL, Api, DEMO, uniqueVkn } from "./lib";

/**
 * Yavaş mobil ağda fiş yükleme (#780): telefon fotoğrafı sunucunun genel
 * okuma süresinden (HTTP_READ_TIMEOUT, 10 sn) uzun sürede gönderince gövde
 * yarıda kesiliyor, uç 400 dönüyor, mobil kuyruk fişi "gönderilemedi"
 * bırakıyordu; fiş ofise hiç ulaşmıyordu. Fiş ucu artık kendi uzun okuma
 * süresini kullanıyor: gövdenin ortasında 12 sn bekleyen yükleme kabul
 * edilir ve ofisin fiş listesinde görünür.
 */
test.describe.configure({ mode: "serial" });

const taxpayer = { vkn: uniqueVkn(), title: "E2E Yavaş Yükleme Ltd." };
let engagement = "";
let taxpayerUuid = "";
let tpLogin = { email: "", password: "" };
let office: Api;

test.beforeAll(async ({ browser }) => {
  test.setTimeout(240_000);
  office = await Api.login(DEMO.owner);
  const added = await addConsentedTaxpayerWithUser(browser, office, taxpayer, "yavasfis");
  engagement = added.engagement;
  taxpayerUuid = added.taxpayerUuid;
  tpLogin = { email: added.email, password: added.password };
});

test.afterAll(async () => {
  await office.dispose();
});

/** Çok parçalı gövdeyi ortasında `pauseMs` bekleyerek gönderir. */
function slowPost(
  path: string,
  headers: Record<string, string>,
  body: Buffer,
  pauseMs: number,
): Promise<{ status: number; text: string }> {
  const url = new URL(path, API_URL);
  const send = url.protocol === "https:" ? httpsRequest : httpRequest;
  return new Promise((resolve, reject) => {
    const req = send(url, { method: "POST", headers: { ...headers, "Content-Length": String(body.length) } }, (res) => {
      let text = "";
      res.setEncoding("utf8");
      res.on("data", (c) => (text += c));
      res.on("end", () => resolve({ status: res.statusCode ?? 0, text }));
    });
    req.on("error", reject);
    const half = Math.floor(body.length / 2);
    req.write(body.subarray(0, half));
    setTimeout(() => req.end(body.subarray(half)), pauseMs);
  });
}

test("gövdesi 12 sn'de gelen fiş kabul edilir ve ofisin fiş listesinde görünür", async () => {
  test.setTimeout(120_000);
  const login = await fetch(new URL("/v1/auth/login", API_URL), {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify(tpLogin),
  });
  expect(login.status).toBe(200);
  const token = ((await login.json()) as { data: { access_token: string } }).data.access_token;

  const boundary = `e2eyavas${Date.now()}`;
  const png = Buffer.concat([
    Buffer.from("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR", "latin1"),
    Buffer.from(`yavas-yukleme-${Date.now()}-`.repeat(200)),
  ]);
  const body = Buffer.concat([
    Buffer.from(`--${boundary}\r\nContent-Disposition: form-data; name="kind"\r\n\r\nexpense_receipt\r\n`),
    Buffer.from(`--${boundary}\r\nContent-Disposition: form-data; name="image"; filename="fis.png"\r\nContent-Type: image/png\r\n\r\n`),
    png,
    Buffer.from(`\r\n--${boundary}--\r\n`),
  ]);
  const res = await slowPost(
    "/v1/taxpayer/receipts",
    {
      Authorization: `Bearer ${token}`,
      "X-Taxpayer": taxpayerUuid,
      "Idempotency-Key": `yavas-${Date.now()}`,
      "Content-Type": `multipart/form-data; boundary=${boundary}`,
    },
    body,
    12_000,
  );
  // Eskiden: 400 VALIDATION_ERROR "multipart form with an image is required".
  // #780: fotoğraf alınır, okuma arka planda (202).
  expect(res.status, res.text).toBe(202);
  const uuid = (JSON.parse(res.text) as { data: { uuid: string } }).data.uuid;

  const list = await office.json<{ items: { uuid: string }[] }>(
    await office.get(`/v1/tenant/engagements/${engagement}/receipts?limit=100`),
  );
  expect(list.items.map((i) => i.uuid)).toContain(uuid);
});
