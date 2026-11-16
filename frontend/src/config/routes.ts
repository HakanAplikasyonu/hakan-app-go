export const routes = {
  public: {
    root: "/",
    health: "/health",
    share: {
      slug: (slug: string) => `/share/${slug}`,
      signed: (token: string) => `/share/s/${token}`,
    },
  },
  guest: {
    login: "/platform/login",
    register: "/platform/register",
    forgotPassword: "/platform/forgot-password",
    resetPassword: "/platform/reset-password",
    verifyEmail: "/platform/verify-email",
  },
  cms: {
    root: "/",
    home: "/",
    profile: {
      root: "/profile",
      password: "/profile/password",
      preferences: "/profile/preferences",
    },
  },
  platform: {
    root: "/platform",
    home: "/platform",
    /**
     * System Settings shell (`/platform/settings/*`). Platform admin pages
     * live here behind one grouped sub-sidebar (`settingsNav`). The legacy
     * top-level URLs redirect permanently via `next.config.ts`.
     */
    settings: {
      root: "/platform/settings",
      system: "/platform/settings/system",
      environment: "/platform/settings/environment",
      general: "/platform/settings/general",
      maintenance: "/platform/settings/maintenance",
      integrations: "/platform/settings/integrations",
      roles: "/platform/settings/roles",
      activity: "/platform/settings/activity",
      logs: "/platform/settings/logs",
      storage: "/platform/settings/storage",
      access: "/platform/settings/access",
      auth: "/platform/settings/auth",
      imports: "/platform/settings/imports",
      exports: "/platform/settings/exports",
    },
    roles: {
      root: "/platform/settings/roles",
      create: "/platform/settings/roles/create",
      detail: (uuid: string) => `/platform/settings/roles/${uuid}`,
      edit: (uuid: string) => `/platform/settings/roles/${uuid}/edit`,
    },
    users: {
      root: "/platform/users",
      create: "/platform/users/create",
      detail: (uuid: string) => `/platform/users/${uuid}`,
      edit: (uuid: string) => `/platform/users/${uuid}/edit`,
    },
    organizations: {
      root: "/platform/organizations",
      create: "/platform/organizations/create",
      detail: (uuid: string) => `/platform/organizations/${uuid}`,
    },
    notifications: {
      root: "/platform/notifications",
      detail: (uuid: string) => `/platform/notifications/${uuid}`,
    },
    imports: {
      root: "/platform/settings/imports",
      detail: (uuid: string) => `/platform/settings/imports/${uuid}`,
    },
    exports: {
      root: "/platform/settings/exports",
      detail: (uuid: string) => `/platform/settings/exports/${uuid}`,
    },
    access: {
      root: "/platform/settings/access",
    },
    authSettings: {
      root: "/platform/settings/auth",
    },
    integrations: {
      root: "/platform/settings/integrations",
      github: "/platform/settings/integrations?tab=github",
      google: "/platform/settings/integrations?tab=google",
      facebook: "/platform/settings/integrations?tab=facebook",
      apple: "/platform/settings/integrations?tab=apple",
    },
    activity: {
      root: "/platform/settings/activity",
    },
    logs: {
      root: "/platform/settings/logs",
    },
    storage: {
      root: "/platform/settings/storage",
    },
    profile: {
      root: "/platform/profile",
      password: "/platform/profile/password",
      preferences: "/platform/profile/preferences",
    },
  },
  /** Organization (tenant) shell — mounted only when tenancy is enabled. */
  tenant: {
    /** Organization picker for users with several memberships. */
    picker: "/t",
    home: (slug: string) => `/t/${encodeURIComponent(slug)}`,
    members: (slug: string) => `/t/${encodeURIComponent(slug)}/members`,
    roles: (slug: string) => `/t/${encodeURIComponent(slug)}/roles`,
    settings: (slug: string) => `/t/${encodeURIComponent(slug)}/settings`,
    /** Office setup wizard (issue #273). */
    setup: (slug: string) => `/t/${encodeURIComponent(slug)}/setup`,
    taxpayers: (slug: string) => `/t/${encodeURIComponent(slug)}/taxpayers`,
    /** "Mükellef ekle": consent requests the office sent (issue #7). */
    taxpayerRequests: (slug: string) =>
      `/t/${encodeURIComponent(slug)}/taxpayer-requests`,
    invoices: (slug: string) => `/t/${encodeURIComponent(slug)}/invoices`,
    /** Office invoice detail; `engagement` selects whose invoice (#142). */
    invoice: (slug: string, id: string, engagement?: string) =>
      `/t/${encodeURIComponent(slug)}/invoices/${encodeURIComponent(id)}${engagement ? `?taxpayer=${encodeURIComponent(engagement)}` : ""}`,
    warnings: (slug: string) => `/t/${encodeURIComponent(slug)}/warnings`,
    command: (slug: string) => `/t/${encodeURIComponent(slug)}/command`,
    review: (slug: string) => `/t/${encodeURIComponent(slug)}/review`,
    reviewStudio: (slug: string) =>
      `/t/${encodeURIComponent(slug)}/review/studio`,
    upload: (slug: string) => `/t/${encodeURIComponent(slug)}/upload`,
    /** Journal drafts and Luca / Zirve export (issue #143). */
    journals: (slug: string) => `/t/${encodeURIComponent(slug)}/journals`,
    /** One engagement; opens on its documents. */
    engagement: (slug: string, engagementUuid: string) =>
      `/t/${encodeURIComponent(slug)}/engagements/${encodeURIComponent(engagementUuid)}`,
    /** Documents + history consents of one engagement (issue #8). */
    engagementDocuments: (slug: string, engagementUuid: string) =>
      `/t/${encodeURIComponent(slug)}/engagements/${encodeURIComponent(engagementUuid)}/documents`,
    /** Template-based petitions of one engagement (issue #56). */
    engagementPetitions: (slug: string, engagementUuid: string) =>
      `/t/${encodeURIComponent(slug)}/engagements/${encodeURIComponent(engagementUuid)}/petitions`,
  },
  /** Mükellef (taxpayer) shell — company users invited by an office. */
  taxpayer: {
    /** Company picker for users with several companies. */
    picker: "/m",
    home: (uuid: string) => `/m/${encodeURIComponent(uuid)}`,
    invoices: (uuid: string) => `/m/${encodeURIComponent(uuid)}/invoices`,
    invoice: (uuid: string, invoice: string) =>
      `/m/${encodeURIComponent(uuid)}/invoices/${encodeURIComponent(invoice)}`,
    receipts: (uuid: string) => `/m/${encodeURIComponent(uuid)}/receipts`,
    uploads: (uuid: string) => `/m/${encodeURIComponent(uuid)}/uploads`,
  },
  /** Organization invitation link (public; the token is the credential). */
  invite: (token: string) => `/invite/${encodeURIComponent(token)}`,
  /** Mükellef invitation link sent by an office (public). */
  inviteTaxpayer: (token: string) => `/invite/m/${encodeURIComponent(token)}`,
  /** Consent link an office's add-mükellef request emails (public). */
  consent: (token: string) => `/consent/${encodeURIComponent(token)}`,
  /**
   * Public KVKK text (issue #57): current version, or `version` (what
   * someone accepted) in `lang`.
   */
  legal: (kind: string, opts: { version?: number; lang?: string } = {}) => {
    const q = new URLSearchParams();
    if (opts.version) q.set("version", String(opts.version));
    if (opts.lang) q.set("lang", opts.lang);
    const qs = q.toString();
    return `/legal/${encodeURIComponent(kind)}${qs ? `?${qs}` : ""}`;
  },
  /** Public usage guides (#274, docs/kullanim/). */
  help: {
    index: () => "/yardim",
    guide: (slug: string) => `/yardim/${encodeURIComponent(slug)}`,
  },
  errors: {
    unauthorized: "/unauthorized",
    forbidden: "/forbidden",
  },
} as const;

type NestedRoutes<T> = T extends string
  ? T
  : T extends (...args: never[]) => infer R
    ? R extends string
      ? R
      : never
    : T extends Record<string, unknown>
      ? NestedRoutes<T[keyof T]>
      : never;

export type AppRoute = NestedRoutes<typeof routes>;
