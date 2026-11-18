// WhatsApp Web DOM seçicileri — TEK YER (issue #388).
//
// WhatsApp Web'in DOM'u habersiz değişir. Uzantının sayfaya dokunduğu her
// seçici burada durur; başka modül sayfa yapısı varsaymaz. WhatsApp bir
// güncellemeyle yapıyı değiştirirse yalnızca bu dosya ve
// tests/fixtures/*.html güncellenir (docs/kullanim/whatsapp-uzantisi.md,
// "Seçiciler kırılırsa").
//
// Gizlilik: buradaki seçicilerin hiçbiri mesaj metnini hedeflemez. Okunan
// şeyler yalnızca mesaj satırının data-id özniteliği (eski sürümlerde sohbet
// kimliği için), sohbet başlığı, görselin blob adresi ve belgenin dosya adıdır.

export const SELECTORS = {
  /** Açık sohbet paneli. */
  main: "#main",
  /** Sohbet başlığındaki ad (kişi / grup adı); title özniteliği tercih edilir. */
  chatTitle: [
    '#main header [data-testid="conversation-info-header-chat-title"]',
    '#main header [data-testid="conversation-info-header-chat-title-name"]',
    "#main header span[title]",
    '#main header span[dir="auto"]',
  ],
  /**
   * Bir mesaj satırı. Güncel sürüm: data-id = "<mesaj id>",
   * data-testid = "conv-msg-<mesaj id>". Eski sürüm: data-id =
   * "<true|false>_<sohbet jid>_<mesaj id>[_<katılımcı>]".
   */
  messageRow: ["#main [data-id]", '#main [data-testid^="conv-msg-"]'],
  /** Eski sürümde sohbet kimliğini taşıyan satırlar. */
  legacyMessageRow: "#main [data-id]",
  /** Mesaj satırı sayılmayacak data-id önekleri (gruplanmış çıkartmalar). */
  messageRowSkipPrefix: ["grouped-sticker"],
  /** Mesaj içindeki görsel: indirilmiş görseller blob: adresiyle gelir. */
  messageImage: 'img[src^="blob:"]',
  /** Henüz indirilmemiş görselin bulanık önizlemesi. */
  messageImagePreview: 'img[src^="data:image"]',
  /** Görsel sayılmayacak kapsayıcılar (çıkartma, profil fotoğrafı, bağlantı önizlemesi). */
  imageExcluded: [
    '[data-testid*="sticker"]',
    '[aria-label*="Çıkartma"]',
    '[aria-label*="Sticker"]',
    '[data-testid="link-preview"]',
  ],
  /** Belge (PDF vb.) baloncuğu. */
  documentBubble: [
    '[data-testid="document-thumb"]',
    '[data-icon^="document-"]',
    '[data-icon="document"]',
  ],
  /** Belgenin dosya adı (title özniteliği, mesaj metni değil). */
  documentName: ['[data-testid="document-name"][title]', "span[title]"],
  /** Belgenin indirme düğmesi (uzantı bunu tıklatıp dosyayı yakalar). */
  documentDownload: [
    '[data-testid="document-download"]',
    '[data-testid="media-state-download"]',
    '[data-icon="audio-download"]',
    '[data-icon*="download"]',
    'button[aria-label*="İndir"]',
    'button[aria-label*="Download"]',
  ],
  /** Görsel açıldığında tam ekran görüntüleyici. */
  mediaViewer: ['[data-testid="media-viewer"]', '[data-animate-media-viewer="true"]'],
  /** Görüntüleyicideki büyük görsel. */
  mediaViewerImage: 'img[src^="blob:"]',
  /** Görüntüleyicinin araç çubuğu (düğmeler buraya eklenir). */
  mediaViewerToolbar: ['[data-testid="media-viewer-toolbar"]', '[data-testid="media-viewer"] header', "header"],
} as const;

/** Uzantının eklediği öğelerin sınıf öneki; WhatsApp'ın sınıflarıyla çakışmaz. */
export const OWN_PREFIX = "hakan-wa";

/**
 * Eski sürümlerde data-id'den sohbet kimliği (güncel sürümde kimlik
 * sayfanın kendi dünyasından gelir, bkz. chat.ts). Örnekler:
 *   false_905551112233@c.us_3EB0A1B2C3        → 905551112233@c.us
 *   true_120363041234567890@g.us_ABCD_9055…@c.us → 120363041234567890@g.us
 *   false_98765432101234@lid_3EB0…            → 98765432101234@lid
 */
export const CHAT_ID_RE = /^(?:true|false)_([0-9A-Za-z-]+@(?:c\.us|g\.us|lid|s\.whatsapp\.net))_/;

/** PDF olarak gönderilebilen belge uzantıları (diğerleri düğme almaz). */
export const DOCUMENT_EXTENSIONS = [".pdf", ".jpg", ".jpeg", ".png", ".webp", ".heic"] as const;

export function queryFirst<T extends Element = Element>(root: ParentNode, selectors: readonly string[] | string): T | null {
  for (const s of typeof selectors === "string" ? [selectors] : selectors) {
    const el = root.querySelector<T & Element>(s);
    if (el) return el as T;
  }
  return null;
}

export function closestAny(el: Element, selectors: readonly string[]): Element | null {
  for (const s of selectors) {
    const hit = el.closest(s);
    if (hit) return hit;
  }
  return null;
}
