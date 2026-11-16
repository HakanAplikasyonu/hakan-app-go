/**
 * Mobil bildirim listesi (tasarım: MusavirMobil · 5, MukellefMobil · 4).
 * İki uygulama ortak kullanır. Saf modül: React Native içe aktarmaz,
 * node --test ile sınanır.
 *
 * Uçlar #333 (#339): ofiste `/v1/tenant/notifications` (X-Organization),
 * mükellefte `/v1/taxpayer/notifications` (X-Taxpayer); yanıt `ScopedInbox`
 * (`items`, `total`, `unread`). Satırda tür alanı yok; tür `template_code`
 * (yoksa `source_event`) olay adından çıkarılır.
 */

export type InboxAudience = 'office' | 'taxpayer';

const uclar = (base: string) => ({ list: base, read: (uuid: string) => `${base}/${uuid}/read`, readAll: `${base}/read-all` });

/** Liste ve okundu işaretleme uçları (#333); null olursa ekran istek atmaz, boş durum gösterir. */
export const INBOX_ENDPOINTS: Record<InboxAudience, { list: string; read: (uuid: string) => string; readAll: string } | null> = {
  office: uclar('/v1/tenant/notifications'),
  taxpayer: uclar('/v1/taxpayer/notifications'),
};

export function inboxEndpoint(audience: InboxAudience) {
  return INBOX_ENDPOINTS[audience];
}

/** Bildirim türü: ikon ve taç durumu buna göre seçilir. */
export type InboxKind = 'blocker' | 'upload' | 'task' | 'agent' | 'export' | 'reminder' | 'other';

/** Taç rozeti durumu (bilesenler.md, `TacRozeti`). */
export type CrownState = 'bekleniyor' | 'yuklendi' | 'onaylandi' | 'aktarildi' | 'sorun';

export type InboxItem = {
  uuid: string;
  kind: InboxKind;
  title: string;
  body: string;
  createdAt: string;
  read: boolean;
  /** Dokununca açılacak web yolu; uygulamanın bağlantı eşlemesinden geçer. */
  url: string | null;
};

const KINDS: readonly InboxKind[] = ['blocker', 'upload', 'task', 'agent', 'export', 'reminder', 'other'];

/** Olay adı (backend/internal/platform/events/catalog.go) → tür. */
const OLAYLAR: Record<string, InboxKind> = {
  'warnings.blocker': 'blocker',
  'doc_tasks.submitted': 'upload',
  'doc_tasks.created': 'task',
  'doc_tasks.requested': 'task',
  'doc_tasks.reminder': 'reminder',
  'agents.offline': 'agent',
  'exports.ready': 'export',
  'reports.export.completed': 'export',
};

/** Satırın türü: açık `kind` alanı, yoksa şablon kodu ya da olay adı. */
export function kindOf(o: { kind?: unknown; template_code?: unknown; source_event?: unknown }): InboxKind {
  const k = str(o.kind);
  if ((KINDS as readonly string[]).includes(k)) return k as InboxKind;
  return OLAYLAR[str(o.template_code)] ?? OLAYLAR[str(o.source_event)] ?? 'other';
}

const str = (v: unknown) => (typeof v === 'string' ? v : '');

/**
 * Uçtan gelen satırı ekran modeline çevirir. Bilinmeyen tür `other` olur;
 * uuid ya da başlığı olmayan satır atılır.
 */
export function normalizeInbox(raw: unknown): InboxItem[] {
  const rows = Array.isArray(raw) ? raw : raw && typeof raw === 'object' && Array.isArray((raw as { items?: unknown }).items) ? (raw as { items: unknown[] }).items : [];
  const out: InboxItem[] = [];
  for (const r of rows) {
    if (!r || typeof r !== 'object') continue;
    const o = r as Record<string, unknown>;
    const uuid = str(o.uuid);
    const title = str(o.title);
    if (!uuid || !title) continue;
    out.push({
      uuid,
      kind: kindOf(o),
      title,
      body: str(o.body),
      createdAt: str(o.created_at),
      read: Boolean(o.read_at) || o.read === true,
      url: str(o.action_url) || str(o.url) || null,
    });
  }
  return out;
}

export function crownFor(kind: InboxKind): CrownState {
  switch (kind) {
    case 'blocker':
    case 'agent':
      return 'sorun';
    case 'upload':
      return 'yuklendi';
    case 'export':
      return 'aktarildi';
    default:
      return 'bekleniyor';
  }
}

export const kindLabel: Record<InboxKind, string> = {
  blocker: 'Engelleyici',
  upload: 'Yükleme',
  task: 'Belge isteği',
  agent: 'Ajan',
  export: 'Luca çıktısı',
  reminder: 'Hatırlatma',
  other: 'Bildirim',
};

function istanbulDay(d: Date): number {
  const p = new Intl.DateTimeFormat('en-CA', { timeZone: 'Europe/Istanbul', year: 'numeric', month: '2-digit', day: '2-digit' }).format(d);
  return Math.round(Date.parse(`${p}T00:00:00Z`) / 86_400_000);
}

/** Bugün "08:31", dün "Dün", 2–6 gün "3 gün", daha eski "12 Eki" (Türkiye saati). */
export function inboxTime(iso: string, now: Date = new Date()): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '';
  const diff = istanbulDay(now) - istanbulDay(d);
  if (diff <= 0) return d.toLocaleTimeString('tr-TR', { hour: '2-digit', minute: '2-digit', timeZone: 'Europe/Istanbul' });
  if (diff === 1) return 'Dün';
  if (diff < 7) return `${diff} gün`;
  return d.toLocaleDateString('tr-TR', { day: 'numeric', month: 'short', timeZone: 'Europe/Istanbul' });
}

export const unreadCount = (items: readonly InboxItem[]) => items.reduce((n, i) => n + (i.read ? 0 : 1), 0);

/** Ekran okuyucu etiketi: okunmamış bilgisi renkle sınırlı kalmasın. */
export function inboxA11yLabel(i: InboxItem, now?: Date): string {
  return [i.read ? null : 'Okunmadı', kindLabel[i.kind], i.title, i.body, inboxTime(i.createdAt, now)].filter(Boolean).join(', ');
}
