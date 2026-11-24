/**
 * Mükellef iletişim bilgisi yardımcıları (#780): saf modül, node --test ile
 * sınanır. Telefonu, e-postayı ve yetkili kişinin adını sunucu denetler ve
 * düzeltir; burada yalnız ekran metni ve gönderilecek gövde üretilir.
 */

export type ContactMember = { name: string; surname: string; email: string; role: string };
export type ErrorDetailLike = { field?: string; code?: string };

/** Formun alanları (sunucunun alan adlarıyla aynı). */
export type ContactField = 'phone' | 'email' | 'person_name' | 'person_phone' | 'person_email';
export type ContactValues = Record<ContactField, string>;
export type ContactFieldErrors = Partial<Record<ContactField, string>>;

const FIELDS: ContactField[] = ['phone', 'email', 'person_name', 'person_phone', 'person_email'];

const MESSAGES: Record<string, string> = {
  telefon_gecersiz: 'Telefon numarası geçersiz. Örnek: 0532 123 45 67',
  eposta_gecersiz: 'E-posta adresi geçersiz. Örnek: muhasebe@firma.com.tr',
  ad_gecersiz: 'Ad en fazla 120 karakter olabilir.',
};

/** Hesap sahibi: mükellef hesabının sahipleri ("Ayşe Demir, Ali Kaya"); yoksa null. */
export function contactPerson(members: ContactMember[] | undefined): string | null {
  const owners = (members ?? []).filter((m) => m.role === 'owner').map((m) => `${m.name} ${m.surname}`.trim());
  return owners.length ? owners.join(', ') : null;
}

/**
 * Sunucunun alan hata kodları (`telefon_gecersiz`, `eposta_gecersiz`,
 * `ad_gecersiz`) → alan alan Türkçe ileti. Alan adı gelmezse telefon/e-posta
 * kodu şirket alanına yazılır.
 */
export function contactFieldErrors(details: ErrorDetailLike[] | undefined): ContactFieldErrors {
  const out: ContactFieldErrors = {};
  for (const d of details ?? []) {
    const msg = d.code ? MESSAGES[d.code] : undefined;
    if (!msg) continue;
    const field = FIELDS.find((f) => f === d.field) ?? (d.code === 'telefon_gecersiz' ? 'phone' : d.code === 'eposta_gecersiz' ? 'email' : undefined);
    if (field) out[field] = msg;
  }
  return out;
}

/** Gönderilecek gövde: kırpılmış; boş alan bilgiyi siler. */
export function contactBody(v: ContactValues): ContactValues {
  return {
    phone: v.phone.trim(),
    email: v.email.trim(),
    person_name: v.person_name.trim(),
    person_phone: v.person_phone.trim(),
    person_email: v.person_email.trim(),
  };
}
