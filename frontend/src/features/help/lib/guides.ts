import { HELP_GUIDES, type HelpGuide } from "../content.generated";

export type { HelpGuide };
export { HELP_GUIDES };

/** Adresle açılır; iç belgeler de (listede görünmez, bağlantısı olan okur). */
export function findGuide(slug: string): HelpGuide | undefined {
  return HELP_GUIDES.find((g) => g.slug === slug);
}

/** Son kullanıcıya gösterilen kılavuzlar: iç belgeler (demo senaryosu) hariç. */
export const PUBLIC_GUIDES: readonly HelpGuide[] = HELP_GUIDES.filter(
  (g) => !g.internal,
);

type RoleSource = {
  isSuperAdmin?: boolean;
  /** Genel (platform) rolleri; ofis ve mükellef kullanıcılarında boş. */
  roles?: readonly string[];
  organizations?: readonly { role: { key: string } }[];
  taxpayers?: readonly unknown[];
};

/**
 * Kullanıcının yardım rolleri (#522): ofis üyeliklerindeki rol anahtarları
 * (`owner`, `admin`, `accountant`, `intern`, `member`), mükellef üyeliği
 * varsa `taxpayer`, platform kullanıcısıysa `platform`. Giriş yapmamış
 * ziyaretçide boş.
 */
export function helpRoles(user: RoleSource | null | undefined): string[] {
  if (!user) return [];
  const out = new Set<string>();
  for (const o of user.organizations ?? []) out.add(o.role.key);
  if ((user.taxpayers ?? []).length > 0) out.add("taxpayer");
  if (user.isSuperAdmin || (user.roles ?? []).length > 0) out.add("platform");
  return [...out];
}

/**
 * Role göre sıralama: `mine` kullanıcının rolüne yazılmış kılavuzlar (önce
 * ve öne çıkmış gösterilir), `others` kalanlar ("Tüm kılavuzlar"). Rolü
 * olmayan ziyaretçide hepsi `others`.
 */
export function guidesFor(roles: readonly string[]): {
  mine: HelpGuide[];
  others: HelpGuide[];
} {
  const mine: HelpGuide[] = [];
  const others: HelpGuide[] = [];
  for (const g of PUBLIC_GUIDES)
    (g.roles.some((r) => roles.includes(r)) ? mine : others).push(g);
  return { mine, others };
}

/** Rolün ilk kılavuzu: uygulama içi "Yardım" bağlantısının hedefi. */
export function primaryGuide(roles: readonly string[]): HelpGuide | undefined {
  return guidesFor(roles).mine[0];
}
