/**
 * Muhasebe programına aktarım engelinin (#391, EXPORT_BLOCKED) kullanıcı
 * dilinde metni (#455). Sunucu ayrıntıyı ham biçimde gönderir: alan
 * `entry:Y-12/line:2`, ileti parametrelerin JSON'u ya da önem derecesi.
 * Ekrana kod, alan adı ya da JSON yazılmaz. Saf modül; çeviri dışarıdan.
 */

type Translate = (
  key: string,
  vars?: Record<string, string | number>,
) => string;

export type ExportIssueDetail = {
  code?: string;
  field?: string;
  message?: string;
};

const trMoney = new Intl.NumberFormat("tr-TR", {
  minimumFractionDigits: 2,
  maximumFractionDigits: 2,
});

const MONEY_PARAMS = new Set(["debit", "credit", "difference"]);

/** "entry:Y-12/line:2" → { entry: "Y-12", line: 2 }. */
export function parseIssueField(field: string | undefined) {
  const m = /^entry:(.+?)(?:\/line:(\d+))?$/.exec(field ?? "");
  return m ? { entry: m[1], line: m[2] ? Number(m[2]) : undefined } : {};
}

function params(message: string | undefined): Record<string, string | number> {
  if (!message || !message.startsWith("{")) return {};
  try {
    const raw = JSON.parse(message) as Record<string, unknown>;
    const out: Record<string, string | number> = {};
    for (const [k, v] of Object.entries(raw)) {
      if (typeof v === "number") out[k] = v;
      else if (typeof v === "string")
        out[k] =
          MONEY_PARAMS.has(k) && /^-?\d+(\.\d+)?$/.test(v)
            ? `${trMoney.format(Number(v))} TL`
            : v;
    }
    return out;
  } catch {
    return {};
  }
}

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

/** "Fiş Y-12, satır 2: 600.01 hesabı programın hesap planında yok." */
export function exportIssueText(d: ExportIssueDetail, t: Translate) {
  const p = params(d.message);
  if (typeof p.entry_type === "string")
    p.entry_type = tr(
      t,
      `office.journals.entry_type_${p.entry_type}`,
      p.entry_type,
    );
  // Hesap planda var ama grup hesabı (altında alt hesap var): "planda yok"
  // değil, alt hesap açılması / seçilmesi istenir (Luca grup hesabına kayıt
  // almaz).
  const group =
    d.code === "ACCOUNT_NOT_IN_TARGET_PLAN" && p.reason === "group_account";
  const what = tr(
    t,
    `office.journals.issue_${d.code ?? ""}${group ? "_group" : ""}`,
    tr(t, "office.journals.issue_unknown", "Fiş aktarıma uygun değil."),
    p,
  );
  const { entry, line } = parseIssueField(d.field);
  if (!entry) return what;
  const where = line
    ? tr(t, "office.journals.issue_at_line", `Fiş ${entry}, satır ${line}`, {
        entry,
        line,
      })
    : tr(t, "office.journals.issue_at_entry", `Fiş ${entry}`, { entry });
  return `${where}: ${what}`;
}
