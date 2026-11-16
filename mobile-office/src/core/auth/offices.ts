/**
 * Ofis (organization) üyelikleri: `Me.organizations`. Saf modül, node --test ile sınanır.
 *
 * Bu uygulamaya yalnızca bir mali müşavirlik ofisinde üyeliği olan kullanıcılar
 * (owner, admin, member ya da özel ofis rolü) girer. Yalnızca mükellef
 * üyeliği olan hesaplar açıklayıcı bir mesajla geri çevrilir.
 */
export type OfficeOption = {
  uuid: string;
  slug: string;
  name: string;
  logo_url?: string;
  status: 'active' | 'suspended';
  access_ends_at?: string | null;
  role: { key: string; name: string };
  permissions: string[];
};

type MeLike = {
  organizations?: readonly OfficeOption[] | null;
  taxpayers?: readonly unknown[] | null;
};

/**
 * `office`: en az bir ofis üyeliği var, uygulama açılır.
 * `taxpayerOnly`: yalnızca mükellef hesabı; mükellef uygulamasına yönlendirilir.
 * `none`: hiçbir üyelik yok (davet henüz kabul edilmemiş).
 */
export type OfficeAccess = 'office' | 'taxpayerOnly' | 'none';

export function officeAccess(me: MeLike): OfficeAccess {
  if ((me.organizations?.length ?? 0) > 0) return 'office';
  if ((me.taxpayers?.length ?? 0) > 0) return 'taxpayerOnly';
  return 'none';
}

/** Erişim penceresi açık ve askıda olmayan ofis seçilebilir. */
export function isSelectable(o: OfficeOption, now: number = Date.now()): boolean {
  if (o.status !== 'active') return false;
  if (o.access_ends_at && Date.parse(o.access_ends_at) <= now) return false;
  return true;
}

/**
 * Sormadan açılacak ofis: son kullanılan (üyelik ve erişim sürüyorsa), yoksa
 * seçilebilir tek ofis. Null → seçici gösterilir.
 */
export function pickOffice(options: readonly OfficeOption[], lastUsed: string | null, now: number = Date.now()): string | null {
  const open = options.filter((o) => isSelectable(o, now));
  if (lastUsed) {
    const hit = open.find((o) => o.slug === lastUsed || o.uuid === lastUsed);
    if (hit) return hit.slug;
  }
  if (open.length === 1) return open[0].slug;
  return null;
}

/** Seçici sırası: önce seçilebilenler, sonra Türkçe alfabetik. */
export function sortOffices(options: readonly OfficeOption[], now: number = Date.now()): OfficeOption[] {
  return [...options].sort((a, b) => {
    const sa = isSelectable(a, now) ? 0 : 1;
    const sb = isSelectable(b, now) ? 0 : 1;
    return sa - sb || a.name.localeCompare(b.name, 'tr');
  });
}

export function statusLabel(o: OfficeOption, now: number = Date.now()): string | null {
  if (o.status === 'suspended') return 'Askıda';
  if (o.access_ends_at && Date.parse(o.access_ends_at) <= now) return 'Erişim süresi doldu';
  return null;
}

/** Ofis rozetindeki baş harfler ("Yıldız Mali Müşavirlik" → "YM"). */
export function initials(name: string): string {
  const skip = new Set(['ltd', 'şti', 'a.ş', 'aş', 've', 'smmm', 'ymm']);
  const words = name
    .split(/\s+/)
    .map((w) => w.replace(/[^\p{L}\p{N}]/gu, ''))
    .filter((w) => w && !skip.has(w.toLocaleLowerCase('tr')));
  return words.slice(0, 2).map((w) => w[0].toLocaleUpperCase('tr')).join('') || '?';
}

export function hasPermission(permissions: readonly string[] | undefined, slug: string): boolean {
  return !!permissions?.includes(slug);
}
