// Derlemede manifest'in ortama göre tamamlanması (#780 uzantıyı algılama).
// Saf fonksiyon; tests/manifest.test.ts denetler.
//
// - version: package.json'dan.
// - key: paketlenmemiş yüklemede uzantı kimliğini sabitler (genel anahtar,
//   gizli değil; özel anahtar hiç saklanmaz). Mağaza paketi kendi kimliğini
//   alır; mağaza için noKey ile çıkarılır.
// - host_permissions: pakete gömülen API adresinin kökeni eklenir; eşleştirmede
//   ayrıca izin penceresi çıkmaz.
// - externally_connectable: panelin uzantıya "kurulu mu?" diye sorabileceği
//   adresler. Yalnızca panel alan adları ve yerel geliştirme.

const LOCAL = ["http://localhost/*", "http://127.0.0.1/*"];

/** Adres → Chrome eşleme deseni (bağlantı noktası her zaman eşleşir). */
export function originPattern(raw) {
  let u;
  try {
    u = new URL(String(raw).trim());
  } catch {
    return null;
  }
  if (u.protocol !== "https:" && u.protocol !== "http:") return null;
  return `${u.protocol}//${u.hostname}/*`;
}

/** API adresinden panel adresi tahmini (src/summary.ts guessWebBase ile aynı kural). */
export function panelFromApi(apiUrl) {
  let u;
  try {
    u = new URL(String(apiUrl).trim());
  } catch {
    return "";
  }
  const host = u.hostname.startsWith("api.") ? "app." + u.hostname.slice(4) : u.hostname;
  return `${u.protocol}//${host}/`;
}

/**
 * @param {object} manifest static/manifest.json
 * @param {{version: string, apiUrl?: string, panelUrls?: string, key?: string, noKey?: boolean}} o
 */
export function finalizeManifest(manifest, o) {
  const m = structuredClone(manifest);
  m.version = o.version;
  if (o.key && !o.noKey) m.key = o.key.trim();
  else delete m.key;

  const api = o.apiUrl ? originPattern(o.apiUrl) : null;
  if (api) m.host_permissions = [...new Set([...(m.host_permissions ?? []), api])];

  const panels = (o.panelUrls ?? "")
    .split(",")
    .map((s) => s.trim())
    .filter(Boolean);
  if (panels.length === 0 && o.apiUrl) panels.push(panelFromApi(o.apiUrl));
  const matches = new Set(LOCAL);
  for (const p of panels) {
    const pat = originPattern(p);
    if (pat) matches.add(pat);
  }
  // API panelle aynı köktense (…/api) o da panel adresidir.
  if (api && o.apiUrl && /\/api\/?$/.test(new URL(o.apiUrl).pathname)) matches.add(api);
  m.externally_connectable = { matches: [...matches] };
  return m;
}
