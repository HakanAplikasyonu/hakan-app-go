// Chrome Web Store paketi için manifest dönüşümü ve denetimi (saf işlevler;
// testleri tests/store.test.ts). scripts/store.mjs kullanır.
//
// Mağaza paketinde sunucu adresi pakete gömülüdür; bu yüzden isteğe bağlı
// konak izni "her https sitesi" yerine yalnızca o sunucuya daraltılır. Kalan
// izinler değişmez: storage ve web.whatsapp.com.

/** Mağaza paketinde izin verilen izinler (fazlası paketi düşürür). */
export const ALLOWED_PERMISSIONS = ["storage"];
export const WHATSAPP_HOST = "https://web.whatsapp.com/*";

/** Sunucu adresi → "https://api.ornek.com/*". Uzak, https adres ister. */
export function storeOrigin(raw) {
  let u;
  try {
    u = new URL(String(raw ?? "").trim());
  } catch {
    throw new Error("WHATSAPP_API_URL geçerli bir adres değil (https://api.ofisiniz.com gibi olmalı).");
  }
  if (u.protocol !== "https:") throw new Error("Mağaza paketi için sunucu adresi https olmalı.");
  if (["localhost", "127.0.0.1", "[::1]"].includes(u.hostname)) {
    throw new Error("Mağaza paketine yerel adres gömülemez.");
  }
  return `${u.origin}/*`;
}

/** Derlenmiş manifest → mağaza manifesti (girdiyi değiştirmez). */
export function storeManifest(manifest, apiUrl) {
  const m = structuredClone(manifest);
  m.optional_host_permissions = [storeOrigin(apiUrl)];
  delete m.key;
  delete m.update_url;
  return m;
}

/** Mağazanın reddedeceği ya da sorgulayacağı durumlar; boş dizi = sorun yok. */
export function storeProblems(m) {
  const out = [];
  if (m.manifest_version !== 3) out.push("manifest_version 3 olmalı");
  if (!m.name || m.name.length > 75) out.push("name boş ya da 75 karakterden uzun");
  if (!m.description || m.description.length > 132) out.push("description boş ya da 132 karakterden uzun");
  if (!/^\d+(\.\d+){0,3}$/.test(String(m.version ?? "")) || m.version === "0.0.0") out.push("version geçersiz");
  if (!m.icons?.["128"]) out.push("128 piksellik simge yok");
  for (const p of m.permissions ?? []) {
    if (!ALLOWED_PERMISSIONS.includes(p)) out.push(`gereksiz izin: ${p}`);
  }
  const hosts = m.host_permissions ?? [];
  if (hosts.length !== 1 || hosts[0] !== WHATSAPP_HOST) out.push(`host_permissions yalnızca ${WHATSAPP_HOST} olmalı`);
  for (const h of m.optional_host_permissions ?? []) {
    if (/^\*:|^https?:\/\/\*[./]|<all_urls>|localhost|127\.0\.0\.1/.test(h)) out.push(`geniş ya da yerel isteğe bağlı izin: ${h}`);
  }
  if ("key" in m || "update_url" in m) out.push("key / update_url mağaza paketinde olmamalı");
  return out;
}
