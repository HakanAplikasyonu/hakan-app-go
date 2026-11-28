import type {
  BankStatementAnalysis,
  BankStatementReport,
} from "../services/bank-posting.service";

// Ekstre yükleme ve önizlemenin saf yardımcıları: sunucudan beklenmedik
// biçimde (boş liste yerine null, geçersiz tarih, eksik hücre) gelen yanıt
// ekranı düşürmesin.

const PERIOD = /^\d{4}-(0[1-9]|1[0-2])$/;

/** Raporun hesapları: sunucu boş listeyi `null` yollasa da dizi. */
export function accountsOf(
  rep: BankStatementReport | null | undefined,
): BankStatementReport["accounts"] {
  return Array.isArray(rep?.accounts) ? rep.accounts : [];
}

/**
 * Raporun yeni hareket eklenen dönemleri (YYYY-MM; en çok 12). Tarihi
 * okunamayan hesap atlanır (geçersiz tarih ekranı düşürmez).
 */
export function periodsOf(
  rep: BankStatementReport | null | undefined,
): string[] {
  const out = new Set<string>();
  for (const a of accountsOf(rep)) {
    if (!a || !(Number(a.inserted) > 0) || !a.from || !a.to) continue;
    const start = String(a.from).slice(0, 7);
    const end = String(a.to).slice(0, 7);
    if (!PERIOD.test(start) || !PERIOD.test(end)) continue;
    let [y, m] = start.split("-").map(Number);
    for (let i = 0; i < 12; i++) {
      const p = `${y}-${String(m).padStart(2, "0")}`;
      out.add(p);
      if (p >= end) break;
      m++;
      if (m > 12) [y, m] = [y + 1, 1];
    }
  }
  return [...out].sort();
}

/**
 * Sunucunun analizini güvenli biçime getirir: boş listeler `null` gelse,
 * hücre dizisi eksik olsa ya da aynı satır numarası iki kez gelse de
 * önizleme çizilir (ekran düşmez).
 */
export function safeAnalysis(a: BankStatementAnalysis): BankStatementAnalysis {
  const columns = (Array.isArray(a.columns) ? a.columns : []).filter(Boolean);
  const seen = new Set<number>();
  const rows = (Array.isArray(a.rows) ? a.rows : []).filter((r) => {
    if (!r || !Number.isFinite(r.row) || seen.has(r.row)) return false;
    seen.add(r.row);
    return true;
  });
  return {
    ...a,
    columns: columns.map((c) => ({ ...c, header: String(c.header ?? "") })),
    rows: rows.map((r) => ({
      ...r,
      cells: (Array.isArray(r.cells) ? r.cells : []).map((c) =>
        c === null || c === undefined ? "" : String(c),
      ),
    })),
    warnings: (Array.isArray(a.warnings) ? a.warnings : []).filter(
      (w) => typeof w === "string" && w !== "",
    ),
  };
}
