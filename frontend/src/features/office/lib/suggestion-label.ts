/**
 * İnceleme önerilerinin insan okunur başlığı ve değeri (#375): ham alan adı
 * (`total_amount`) ya da ham JSON hiçbir yüzeyde gösterilmez. Saf modül;
 * çeviri işlevi dışarıdan gelir.
 */

type Translate = (
  key: string,
  vars?: Record<string, string | number>,
) => string;

export type SuggestionLike = {
  kind: string;
  target_type: string;
  target_field: string;
  proposed_value: unknown;
  provenance?: { inputs?: Record<string, unknown> | null } | null;
};

const DECIMAL_RE = /^-?\d+(\.\d+)?$/;
const DATE_RE = /^(\d{4})-(\d{2})-(\d{2})$/;
const trMoney = new Intl.NumberFormat("tr-TR", {
  minimumFractionDigits: 2,
  maximumFractionDigits: 2,
});

/** Çeviri yoksa `fallback`; çeviri işlevi bilinmeyen anahtarı aynen döndürür. */
function tr(
  t: Translate,
  key: string,
  fallback: string,
  vars?: Record<string, string | number>,
) {
  const v = t(key, vars);
  return v === key ? fallback : v;
}

/** "total_amount" → "Total amount": çevirisi olmayan alan için son çare. */
export function humanize(field: string) {
  const s = field.replace(/[_.]+/g, " ").trim();
  return s ? s[0].toLocaleUpperCase("tr-TR") + s.slice(1) : field;
}

/** Alanın Türkçe adı (`office.review_api.field_<alan>`). */
export function fieldTitle(field: string, t: Translate) {
  return tr(t, `office.review_api.field_${field}`, humanize(field));
}

/** Öneri türünün adı (süzgeç). */
export function kindTitle(kind: string, t: Translate) {
  return tr(t, `office.review_api.kind_${kind}`, humanize(kind));
}

/** Satır türü; `unknown` ya da boşsa gösterilmez (undefined). */
export function lineTypeLabel(lineType: string | undefined, t: Translate) {
  if (!lineType || lineType === "unknown") return undefined;
  return tr(t, `office.review_api.line_type_${lineType}`, humanize(lineType));
}

type MatchValue = {
  score?: number;
  invoices?: { number?: string; amount?: string }[];
};

function asMatch(v: unknown): MatchValue | null {
  if (!v || typeof v !== "object" || Array.isArray(v)) return null;
  const m = v as MatchValue;
  return Array.isArray(m.invoices) ? m : null;
}

/** Banka eşleşmesi başlığı: "Banka eşleşmesi: Fatura ATL…101" / "3 fatura". */
function matchTitle(m: MatchValue, t: Translate) {
  const nums = (m.invoices ?? [])
    .map((i) => i.number)
    .filter(Boolean) as string[];
  if (nums.length === 1)
    return tr(
      t,
      "office.review_api.match_one",
      `Banka eşleşmesi: Fatura ${nums[0]}`,
      {
        number: nums[0],
      },
    );
  return tr(
    t,
    "office.review_api.match_many",
    `Banka eşleşmesi: ${nums.length} fatura`,
    {
      count: nums.length,
    },
  );
}

/**
 * Değerin kısa gösterimi: hesap kodu, tutar (tr-TR), tarih (GG.AA.YYYY),
 * eşleşme puanı (%73). Nesne asla JSON olarak yazılmaz.
 */
export function valueText(value: unknown): string {
  if (value === null || value === undefined) return "";
  if (typeof value === "number") return trMoney.format(value);
  if (typeof value === "string") {
    const d = DATE_RE.exec(value);
    if (d) return `${d[3]}.${d[2]}.${d[1]}`;
    return DECIMAL_RE.test(value) && value.includes(".")
      ? trMoney.format(Number(value))
      : value;
  }
  if (typeof value === "boolean") return value ? "✓" : "—";
  if (typeof value === "object" && !Array.isArray(value)) {
    const v = value as Record<string, unknown>;
    if (typeof v.account_code === "string") return v.account_code;
    const m = asMatch(value);
    if (m) return typeof m.score === "number" ? `%${m.score}` : "";
  }
  return "—";
}

/**
 * Listede ve ayrıntıda başlık. Hesap kodu önerisinde kalem adı / cari;
 * fiş alanında alanın Türkçe adı; banka eşleşmesinde fatura numaraları.
 */
export function suggestionTitle(s: SuggestionLike, t: Translate) {
  const inputs = s.provenance?.inputs ?? {};
  const str = (k: string) => {
    const v = inputs[k];
    return typeof v === "string" && v ? v : undefined;
  };
  const m = s.kind === "bank_match" ? asMatch(s.proposed_value) : null;
  if (m) return matchTitle(m, t);
  // Mutabakat farkı (#389): hedef alan `status`; ham alan adı yerine hesap.
  if (s.kind === "reconciliation") {
    const code = str("main_code");
    return code
      ? tr(
          t,
          "office.review_api.reconciliation_title",
          `Mutabakat farkı: ${code}`,
          {
            code,
          },
        )
      : kindTitle(s.kind, t);
  }
  if (s.kind === "receipt_field") {
    const where = tr(t, `office.review_api.target_${s.target_type}`, "");
    return where
      ? `${fieldTitle(s.target_field, t)} · ${where}`
      : fieldTitle(s.target_field, t);
  }
  const item = str("item_name");
  if (item) return item;
  const cp = str("counterparty_name");
  if (cp)
    return tr(t, "office.review_api.counterparty_account", cp, { name: cp });
  // Banka hareketinin karşı hesabı (#383): karşı tarafsız harekette açıklama.
  const desc = s.kind === "bank_account" ? str("description") : undefined;
  if (desc) return desc;
  return fieldTitle(s.target_field, t);
}

/** Banka eşleşmesinin alt satırı: karşı taraf · tutar · tarih. Mutabakat
 * farkında: mizan · kaynaklar · fark (tr-TR). */
export function matchSummary(s: SuggestionLike, t?: Translate) {
  const i = s.provenance?.inputs ?? {};
  if (s.kind === "reconciliation") {
    const money = (k: string) =>
      typeof i[k] === "string" && DECIMAL_RE.test(i[k] as string)
        ? `${trMoney.format(Number(i[k]))} TL`
        : undefined;
    const part = (k: string, key: string, fallback: string) => {
      const v = money(k);
      if (!v) return undefined;
      return t
        ? tr(t, key, `${fallback} ${v}`, { amount: v })
        : `${fallback} ${v}`;
    };
    return (
      [
        part("ledger_net", "office.review_api.reconciliation_ledger", "Mizan"),
        part(
          "source_net",
          "office.review_api.reconciliation_sources",
          "Kaynaklar",
        ),
        part(
          "difference",
          "office.review_api.reconciliation_difference",
          "Fark",
        ),
      ]
        .filter(Boolean)
        .join(" · ") || undefined
    );
  }
  if (s.kind !== "bank_match") return undefined;
  const amount =
    typeof i.amount === "string"
      ? `${trMoney.format(Number(i.amount))} ${typeof i.currency === "string" ? i.currency : ""}`.trim()
      : undefined;
  const date = typeof i.date === "string" ? valueText(i.date) : undefined;
  const cp =
    typeof i.counterparty === "string" && i.counterparty
      ? i.counterparty
      : undefined;
  return [cp, amount, date].filter(Boolean).join(" · ") || undefined;
}

const OCR_RATIONALE_RE = /^OCR alan güveni\s+(\d+(?:\.\d+)?)/;

/**
 * Önerinin gerekçesi insan diliyle (#576): eski kayıtlardaki
 * "OCR alan güveni 0.41000000" yüzdeye çevrilir; diğer gerekçeler aynen.
 */
export function humanRationale(
  rationale: string | null | undefined,
  t: Translate,
): string {
  if (!rationale) return "";
  const m = OCR_RATIONALE_RE.exec(rationale);
  if (!m) return rationale;
  const pct = Math.round(Number(m[1]) * 100);
  return t("office.review_api.ocr_rationale", { pct });
}
