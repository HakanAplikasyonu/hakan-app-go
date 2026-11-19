import type { HelpGuide } from "./guides";

/** Kılavuzun bir bölümü: `## Başlık` ve altındaki metin. */
export type GuideSection = {
  /** Sayfa içi bağlantı kimliği (`#ubl-yükleme`), GitHub ile aynı kural. */
  id: string;
  title: string;
  /** Başlık hariç Markdown gövdesi. */
  source: string;
};

export type GuideOutline = {
  title: string;
  summary: string;
  /** İlk bölümden önceki giriş metni (başlık ve özet hariç). */
  intro: string;
  sections: GuideSection[];
};

/**
 * Başlıktan bağlantı kimliği: GitHub'ın kuralı (küçük harf, noktalama
 * silinir, boşluk tire olur; Türkçe harfler kalır). Aynı kimlik ikinci kez
 * gelirse `-1`, `-2` eklenir. Böylece `docs/kullanim` içindeki
 * `ofis-calisani.md#inceleme` bağlantıları uygulamada da çalışır.
 */
export function headingId(title: string, used?: Map<string, number>): string {
  const base =
    title
      .trim()
      .toLocaleLowerCase("tr")
      .replace(/[`*_~]/g, "")
      .replace(/[^\p{L}\p{N}\s-]/gu, "")
      .replace(/\s/g, "-") || "bolum";
  if (!used) return base;
  const n = used.get(base) ?? 0;
  used.set(base, n + 1);
  return n === 0 ? base : `${base}-${n}`;
}

/** Kılavuzu bölümlerine ayırır; kod bloklarının içindeki `##` sayılmaz. */
export function guideOutline(body: string): GuideOutline {
  const lines = body.replace(/\r\n?/g, "\n").split("\n");
  const out: GuideOutline = { title: "", summary: "", intro: "", sections: [] };
  const used = new Map<string, number>();
  let current: { title: string; lines: string[] } | null = null;
  const intro: string[] = [];
  let fenced = false;
  const close = () => {
    if (!current) return;
    out.sections.push({
      id: headingId(current.title, used),
      title: current.title,
      source: current.lines.join("\n").trim(),
    });
  };
  for (const line of lines) {
    if (line.trimStart().startsWith("```")) fenced = !fenced;
    const h2 = fenced ? null : /^##\s+(.+?)\s*$/.exec(line);
    if (h2) {
      close();
      current = { title: h2[1]!, lines: [] };
      continue;
    }
    if (current) {
      current.lines.push(line);
      continue;
    }
    const h1 = /^#\s+(.+?)\s*$/.exec(line);
    if (h1 && !out.title) {
      out.title = h1[1]!;
      continue;
    }
    const quote = /^>\s*(.+?)\s*$/.exec(line);
    if (quote && !out.summary && intro.every((l) => !l.trim())) {
      out.summary = quote[1]!;
      continue;
    }
    intro.push(line);
  }
  close();
  out.intro = intro.join("\n").trim();
  return out;
}

/** Aramada görünen düz metin: Markdown işaretleri ve görseller ayıklanır. */
export function plainText(markdown: string): string {
  return markdown
    .replace(/```[\s\S]*?```/g, " ")
    .replace(/!\[[^\]]*\]\([^)]*\)/g, " ")
    .replace(/\[([^\]]+)\]\([^)]*\)/g, "$1")
    .replace(/^\s*(?:[-*>]|\d+\.|#{1,6}|\|)\s*/gm, "")
    .replace(/[*_`|]/g, "")
    .replace(/\s+/g, " ")
    .trim();
}

export type HelpHit = {
  guide: HelpGuide;
  /** Eşleşen bölüm; kılavuzun girişinde eşleştiyse null. */
  section: { id: string; title: string } | null;
  /** Eşleşmenin çevresindeki kısa alıntı. */
  snippet: string;
  /** Başlıkta eşleşme önce gelir. */
  inTitle: boolean;
};

const fold = (s: string) => s.toLocaleLowerCase("tr");

/**
 * Kılavuzlarda tam metin arama (#522): bütün sözcükler aynı bölümde geçmeli
 * (büyük / küçük harf Türkçe kuralıyla eşitlenir). Başlık eşleşmeleri önce,
 * sonra kılavuz sırası; en çok `limit` sonuç.
 */
export function searchGuides(
  query: string,
  guides: readonly HelpGuide[],
  limit = 12,
): HelpHit[] {
  const words = fold(query)
    .split(/\s+/)
    .filter((w) => w.length >= 2);
  if (words.length === 0) return [];
  const hits: HelpHit[] = [];
  for (const guide of guides) {
    const o = guideOutline(guide.body);
    const parts = [
      { section: null, title: guide.title, text: plainText(o.intro) },
      ...o.sections.map((s) => ({
        section: { id: s.id, title: s.title },
        title: s.title,
        text: plainText(s.source),
      })),
    ];
    for (const p of parts) {
      const title = fold(p.title);
      const text = fold(p.text);
      if (!words.every((w) => title.includes(w) || text.includes(w))) continue;
      const at = Math.max(
        0,
        text.indexOf(words.find((w) => text.includes(w)) ?? ""),
      );
      const start = Math.max(0, at - 40);
      const snippet =
        (start > 0 ? "…" : "") +
        p.text.slice(start, start + 140).trim() +
        (start + 140 < p.text.length ? "…" : "");
      hits.push({
        guide,
        section: p.section,
        snippet,
        inTitle: words.every((w) => title.includes(w)),
      });
    }
  }
  return hits
    .sort((a, b) => Number(b.inTitle) - Number(a.inTitle))
    .slice(0, limit);
}
