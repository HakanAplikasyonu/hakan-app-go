/**
 * UBL-TR fatura türü (`InvoiceTypeCode`) ve senaryo (`ProfileID`) kodlarının
 * Türkçe adı (#455): "SATIS · TICARIFATURA" yerine "Satış · Ticari e-fatura".
 * Çevirisi olmayan kod aynen gösterilir. Saf modül; çeviri dışarıdan.
 */

type Translate = (
  key: string,
  vars?: Record<string, string | number>,
) => string;

function label(t: Translate, prefix: string, code: string) {
  const key = `office.ubl.${prefix}_${code}`;
  const v = t(key);
  return v === key ? code : v;
}

export function invoiceTypeLabel(
  code: string | null | undefined,
  t: Translate,
) {
  return code ? label(t, "type", code) : "";
}

export function invoiceProfileLabel(
  code: string | null | undefined,
  t: Translate,
) {
  return code ? label(t, "profile", code) : "";
}

/** "Satış · Ticari e-fatura"; profil yoksa yalnızca tür. */
export function invoiceKindText(
  type: string | null | undefined,
  profile: string | null | undefined,
  t: Translate,
) {
  return [invoiceTypeLabel(type, t), invoiceProfileLabel(profile, t)]
    .filter(Boolean)
    .join(" · ");
}
