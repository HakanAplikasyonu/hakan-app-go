// Açılır pencerenin saf yardımcıları (kolay kurulum, #780): eşleştirme
// kodunun biçimi ve "Bağlantıyı test et" sonucunun metni. DOM ve chrome.*
// kullanmaz; tests/setup.test.ts ile denetlenir.

/** Sunucunun kod alfabesi (0/O, 1/I/L yok): backend whatsapp.NormalizeCode ile aynı. */
const CODE_ALPHABET = "ABCDEFGHJKMNPQRSTUVWXYZ23456789";
const CODE_LEN = 8;

/**
 * Kullanıcının yazdığı kodu XXXX-XXXX biçimine getirir. Boşluk ve tire
 * atılır, küçük harf büyütülür. Kod hatalı yazılmışsa (uzunluk ya da alfabe
 * dışı karakter) ok=false ve Türkçe açıklama döner; sunucuya gidilmez.
 */
export function normalizeCode(raw: string): { ok: true; code: string } | { ok: false; error: string } {
  const s = raw.replace(/[\s-]/g, "").toUpperCase();
  if (!s) return { ok: false, error: "Eşleştirme kodunu yazın. Kodu paneldeki WhatsApp sayfasından alın." };
  for (const ch of s) {
    if (!CODE_ALPHABET.includes(ch)) {
      const hint = "0O1IL".includes(ch) ? " Kodda 0, O, 1, I ve L bulunmaz; paneldeki kodu yeniden okuyun." : "";
      return { ok: false, error: `Kod hatalı yazılmış ("${ch}" kodda olamaz).${hint}` };
    }
  }
  if (s.length !== CODE_LEN) {
    return { ok: false, error: `Kod 8 karakterdir (XXXX-XXXX); ${s.length} karakter yazıldı.` };
  }
  return { ok: true, code: `${s.slice(0, 4)}-${s.slice(4)}` };
}

/**
 * Sunucu adresi: önce daha önce kaydedilen, yoksa pakete derlemede gömülen
 * (HAKAN_API_URL). İkisi de boşsa açılır pencere adres alanını gösterir.
 */
export function defaultBase(saved: string, built: string): string {
  return (saved || built || "").trim();
}

export interface TestOutcome {
  ok: boolean;
  message: string;
  /** WhatsApp Web açık değilse "WhatsApp Web'i aç" önerilir. */
  openWhatsApp: boolean;
}

/**
 * "Bağlantıyı test et": sunucu denemesinin hata metni (başarılıysa boş) ve
 * açık WhatsApp Web sekmesi sayısı → kullanıcıya gösterilecek sonuç.
 */
export function testOutcome(serverError: string, whatsappTabs: number): TestOutcome {
  if (serverError) return { ok: false, message: serverError, openWhatsApp: false };
  if (whatsappTabs === 0) {
    return {
      ok: false,
      message: "Sunucu bağlantısı çalışıyor ama açık bir WhatsApp Web sekmesi yok. \"WhatsApp Web'i aç\" ile açıp telefonunuzla giriş yapın.",
      openWhatsApp: true,
    };
  }
  return { ok: true, message: "Her şey hazır: sunucu bağlantısı çalışıyor ve WhatsApp Web açık.", openWhatsApp: false };
}
