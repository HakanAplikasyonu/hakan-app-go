import type { components } from "@/generated/api";

import { readLastOrg } from "./last-org";
import { safeNextPath } from "./next-path";

export type PublicUser = components["schemas"]["PublicUser"];
export type LoginRequest = components["schemas"]["LoginRequest"];
export type Me = components["schemas"]["Me"];
export type Tokens = components["schemas"]["Tokens"];
export type MeOrganization = components["schemas"]["MeOrganization"];
export type MeTaxpayer = components["schemas"]["MeTaxpayer"];

/** A mükellef (company) membership from `/v1/auth/me` (issue #4). */
export type AuthTaxpayer = {
  uuid: string;
  kind: "legal" | "individual";
  title: string;
  role: "owner" | "employee";
};

/** A tenant membership from `/v1/auth/me` (role + permissions in that org). */
export type AuthOrganization = {
  uuid: string;
  slug: string;
  name: string;
  logoUrl?: string;
  status: "active" | "suspended";
  accessEndsAt: string | null;
  role: { key: string; name: string };
  permissions: string[];
};

export type RoleSummary = {
  uuid: string;
  name: string;
  slug: string;
  description?: string | null;
  is_system?: boolean;
};

/** Client session identity — profile + RBAC. */
export type AuthUser = {
  uuid: string;
  email: string;
  name: string;
  surname: string;
  fullName: string;
  status: string;
  locale: string;
  /** SMMM oda sicil no (D16, unverified); null when not set. */
  smmmRegistryNo: string | null;
  isSuperAdmin: boolean;
  emailVerified: boolean;
  permissions: string[];
  roles: string[];
  /** Tenant memberships; empty when tenancy is off or the user has none. */
  organizations: AuthOrganization[];
  /** Mükellef (company) memberships; empty for office-only users. */
  taxpayers: AuthTaxpayer[];
  realtimeUserChannel?: string;
  realtimeEnabled?: boolean;
  impersonation?: {
    uuid: string;
    email: string;
    name: string;
    surname: string;
    fullName: string;
  };
};

export function mapMeToAuthUser(me: Me): AuthUser {
  const {
    user,
    roles,
    permissions,
    realtime,
    impersonation,
    organizations,
    taxpayers,
  } = me;
  const locale =
    typeof user.locale === "string" && user.locale ? user.locale : "tr";
  const mapped: AuthUser = {
    uuid: user.uuid,
    email: user.email,
    name: user.name,
    surname: user.surname,
    fullName: `${user.name} ${user.surname}`.trim(),
    status: user.status,
    locale,
    smmmRegistryNo: user.smmm_registry_no ?? null,
    isSuperAdmin: Boolean(user.is_super_admin),
    emailVerified: Boolean(user.email_verified),
    permissions: permissions ?? [],
    roles: roles ?? [],
    organizations: (organizations ?? []).map(mapMeOrganization),
    taxpayers: (taxpayers ?? []).map(mapMeTaxpayer),
    realtimeUserChannel: realtime?.user_channel,
    realtimeEnabled: realtime?.enabled,
  };
  if (impersonation?.user) {
    const actor = impersonation.user;
    mapped.impersonation = {
      uuid: actor.uuid,
      email: actor.email,
      name: actor.name,
      surname: actor.surname,
      fullName: `${actor.name} ${actor.surname}`.trim(),
    };
  }
  return mapped;
}

function mapMeOrganization(org: MeOrganization): AuthOrganization {
  return {
    uuid: org.uuid,
    slug: org.slug,
    name: org.name,
    logoUrl: org.logo_url || undefined,
    status: org.status,
    accessEndsAt: org.access_ends_at ?? null,
    role: { key: org.role.key, name: org.role.name },
    permissions: org.permissions ?? [],
  };
}

function mapMeTaxpayer(tp: MeTaxpayer): AuthTaxpayer {
  return { uuid: tp.uuid, kind: tp.kind, title: tp.title, role: tp.role };
}

/** The mükellef membership for `uuid`, if the user has one. */
export function findTaxpayer(
  user: AuthUser | null | undefined,
  uuid: string,
): AuthTaxpayer | undefined {
  if (!user) return undefined;
  const wanted = uuid.toLowerCase();
  return (user.taxpayers ?? []).find((t) => t.uuid.toLowerCase() === wanted);
}

/** The membership for `slug` (case-insensitive), if the user has one. */
export function findOrganization(
  user: AuthUser | null | undefined,
  slug: string,
): AuthOrganization | undefined {
  if (!user) return undefined;
  const wanted = slug.toLowerCase();
  return (user.organizations ?? []).find(
    (o) => o.slug.toLowerCase() === wanted,
  );
}

export function hasPlatformPermission(
  user: AuthUser | null | undefined,
): boolean {
  if (!user) return false;
  if (user.isSuperAdmin || user.roles.includes("super_admin")) return true;
  return user.permissions.some((p) => p.startsWith("platform."));
}

export function isPlatformUser(user: AuthUser | null | undefined): boolean {
  return hasPlatformPermission(user);
}

export function isCmsUser(user: AuthUser | null | undefined): boolean {
  if (!user) return false;
  return !isPlatformUser(user);
}

/**
 * Shell after sign-in (an explicit `next` always wins, see
 * `postLoginTarget`): platform users → /platform; one organization →
 * /t/{slug}; several → the last office opened in this browser if still a
 * member (#890), else the /t picker; no office but companies → /m/{uuid} or
 * the /m picker; none → the CMS home (single-scope apps keep working).
 */
export function defaultHomeForUser(
  user: AuthUser,
  lastOrg: string | null = readLastOrg(),
): string {
  if (isPlatformUser(user)) return "/platform";
  const orgs = user.organizations ?? [];
  if (orgs.length === 1) return `/t/${orgs[0]!.slug}`;
  if (orgs.length > 1) {
    const last = lastOrg ? findOrganization(user, lastOrg) : undefined;
    return last ? `/t/${last.slug}` : "/t";
  }
  const tps = user.taxpayers ?? [];
  if (tps.length === 1) return `/m/${tps[0]!.uuid}`;
  if (tps.length > 1) return "/m";
  return "/";
}

/** Ofisi ya da mükellefi olan (platform dışı) kullanıcı: CMS'e düşmemeli. */
export function hasScopedHome(user: AuthUser | null | undefined): boolean {
  if (!user || isPlatformUser(user)) return false;
  return (
    (user.organizations ?? []).length > 0 || (user.taxpayers ?? []).length > 0
  );
}

/** CMS kabuğunun adresleri: `/` ve `/profile/*`. */
export function isCmsPath(path: string): boolean {
  const p = path.split(/[?#]/)[0]!.replace(/\/+$/, "") || "/";
  return p === "/" || p === "/profile" || p.startsWith("/profile/");
}

/**
 * Girişten sonra gidilecek yer (#890 B): güvenli `?next=` önceliklidir; ama
 * ofis / mükellef kullanıcısı için `/` ya da CMS adresi yok sayılır.
 */
export function postLoginTarget(
  user: AuthUser,
  next: string | null | undefined,
): string {
  const home = defaultHomeForUser(user);
  const safe = safeNextPath(next, "");
  if (!safe) return home;
  if (hasScopedHome(user) && isCmsPath(safe)) return home;
  return safe;
}

/** Çıkıştan sonra giriş sayfası: süper admin platform girişine, diğerleri /login. */
export function loginPathFor(user: AuthUser | null | undefined): string {
  return isPlatformUser(user) ? "/platform/login" : "/login";
}

/**
 * Kullanıcının kendi kabuğundaki profil (#890 C): ofis → /t/{slug}/profil,
 * mükellef → /m/{uuid}/profil, süper admin → /platform/profile. Birden çok
 * ofiste son ofis yoksa seçici profil sayfasına götürür; birden çok
 * mükellefte mükellef seçici açılır.
 */
export function profilePathFor(user: AuthUser | null | undefined): string {
  if (!user) return "/profile";
  if (isPlatformUser(user)) return "/platform/profile";
  const home = defaultHomeForUser(user);
  if (home === "/t") return "/t?to=profil";
  if (home === "/m") return "/m";
  if (home.startsWith("/t/") || home.startsWith("/m/")) return `${home}/profil`;
  return "/profile";
}
