import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";

/**
 * Ortak axe-core denetimi (#194, #455): WCAG 2.1 A/AA. Kritik ve ciddi
 * (critical / serious) ihlal testi kırar; orta ve küçük ihlaller test ekine
 * (annotation) yazılır. Başka spec dosyaları da (ör. #452 akışları) bunu
 * kullanır; ikinci bir axe yardımcısı yazmayın.
 *
 * `include`: yalnızca bir bölgeyi (ör. açık diyalog) denetler.
 */
const BLOCKING = new Set(["critical", "serious"]);

export async function scan(page: Page, name: string, opts: { include?: string } = {}) {
  // İçerik yüklensin: yükleniyor iskeletleri ve geçici durumlar denetlenmez.
  await page.waitForLoadState("networkidle");
  let builder = new AxeBuilder({ page })
    .withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa"])
    // Realtime hata ayıklama paneli yalnızca `next dev`'de çizilir (üretimde yok).
    .exclude('[aria-label^="Realtime debug"]')
    .exclude('[title^="Realtime ·"]');
  if (opts.include) builder = builder.include(opts.include);
  const result = await builder.analyze();
  const blocking = result.violations.filter((v) => BLOCKING.has(v.impact ?? ""));
  const minor = result.violations.filter((v) => !BLOCKING.has(v.impact ?? ""));
  for (const v of minor) {
    test.info().annotations.push({ type: `a11y:${v.impact}`, description: `${name}: ${v.id} (${v.nodes.length})` });
  }
  const report = blocking
    .map(
      (v) =>
        `${v.impact} ${v.id}: ${v.help}\n` +
        v.nodes
          .slice(0, 5)
          .map((n) => `    ${n.target.join(" ")} — ${n.failureSummary?.split("\n").slice(1, 2).join("").trim()}`)
          .join("\n"),
    )
    .join("\n");
  expect(blocking, `${name}\n${report}`).toEqual([]);
}
