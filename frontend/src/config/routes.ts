/** Mükellef özetinde açık belge istekleri bölümünün çapası (#563). */
export const DOC_TASKS_ANCHOR = "belge-istekleri";
/** Fişler ekranında taslak fiş kontrolü ve aktarım bölümleri (#566). */
export const DRAFTS_ANCHOR = "taslak-fisler";
export const EXPORT_ANCHOR = "aktarim";
/** Fişler ekranında programa aktarım geçmişi (#574). */
export const TRANSFERS_ANCHOR = "programa-aktarim";

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
    /** Paraşüt OAuth dönüşü; PARASUT_REDIRECT_URL bu yolu gösterir (#518). */
    parasutCallback: "/parasut/callback",
    home: (slug: string) => `/t/${encodeURIComponent(slug)}`,
    members: (slug: string) => `/t/${encodeURIComponent(slug)}/members`,
    roles: (slug: string) => `/t/${encodeURIComponent(slug)}/roles`,
    settings: (slug: string) => `/t/${encodeURIComponent(slug)}/settings`,
    /** Office setup wizard (issue #273). */
    setup: (slug: string) => `/t/${encodeURIComponent(slug)}/setup`,
    taxpayers: (slug: string) => `/t/${encodeURIComponent(slug)}/taxpayers`,
    /** Ofisin beyan dönemi: mükellef bazında KDV1 / MUHSGK durumu (#575). */
    taxReturns: (slug: string) => `/t/${encodeURIComponent(slug)}/tax-returns`,
    /** "Mükellef ekle": consent requests the office sent (issue #7). */
    taxpayerRequests: (slug: string) =>
      `/t/${encodeURIComponent(slug)}/taxpayer-requests`,
    invoices: (slug: string) => `/t/${encodeURIComponent(slug)}/invoices`,
    /** Faturalar, bir mükellefe süzülmüş (engagement seçici, #142). */
    invoicesFor: (slug: string, engagementUuid: string) =>
      `/t/${encodeURIComponent(slug)}/invoices?taxpayer=${encodeURIComponent(engagementUuid)}`,
    /** Office invoice detail; `engagement` selects whose invoice (#142). */
    invoice: (slug: string, id: string, engagement?: string) =>
      `/t/${encodeURIComponent(slug)}/invoices/${encodeURIComponent(id)}${engagement ? `?taxpayer=${encodeURIComponent(engagement)}` : ""}`,
    warnings: (slug: string) => `/t/${encodeURIComponent(slug)}/warnings`,
    /** Mükellef raporları: toplu e-posta ve gönderim geçmişi (#386). */
    reports: (slug: string) => `/t/${encodeURIComponent(slug)}/reports`,
    command: (slug: string) => `/t/${encodeURIComponent(slug)}/command`,
    /** Co-pilot: sıradaki iş akışı ve komut çubuğu (#392). */
    copilot: (slug: string) => `/t/${encodeURIComponent(slug)}/copilot`,
    review: (slug: string) => `/t/${encodeURIComponent(slug)}/review`,
    upload: (slug: string) => `/t/${encodeURIComponent(slug)}/upload`,
    /** Journal drafts and Luca / Zirve export (issue #143). */
    journals: (slug: string) => `/t/${encodeURIComponent(slug)}/journals`,
    /** Ofis geneli bekleyen ve son programa aktarımlar (#574). */
    transfers: (slug: string) => `/t/${encodeURIComponent(slug)}/transfers`,
    /** One engagement; opens on its documents. */
    engagement: (slug: string, engagementUuid: string) =>
      `/t/${encodeURIComponent(slug)}/engagements/${encodeURIComponent(engagementUuid)}`,
    /** Mükellef özetindeki açık belge istekleri listesi (#563). */
    engagementDocTasks: (slug: string, engagementUuid: string) =>
      `/t/${encodeURIComponent(slug)}/engagements/${encodeURIComponent(engagementUuid)}#${DOC_TASKS_ANCHOR}`,
    /** Mizan hafızası: account plan, rule candidates, rules (#310). */
    accountPlan: (slug: string) =>
      `/t/${encodeURIComponent(slug)}/account-plan`,
    /** Desktop agents of the office (#285). */
    agents: (slug: string) => `/t/${encodeURIComponent(slug)}/agents`,
    /** WhatsApp Web extension: pairing, saved files, chat links (#388). */
    whatsapp: (slug: string) => `/t/${encodeURIComponent(slug)}/whatsapp`,
    /** Documents + history consents of one engagement (issue #8). */
    engagementDocuments: (slug: string, engagementUuid: string) =>
      `/t/${encodeURIComponent(slug)}/engagements/${encodeURIComponent(engagementUuid)}/documents`,
    /** Belge arşivi: faaliyet belgesi, ticaret sicil, İSG … (#669 I). */
    engagementArchive: (slug: string, engagementUuid: string) =>
      `/t/${encodeURIComponent(slug)}/engagements/${encodeURIComponent(engagementUuid)}/archive`,
    /** Faturalar sekmesi: mükellef detayı içinde fatura listesi (#468). */
    engagementInvoices: (slug: string, engagementUuid: string) =>
      `/t/${encodeURIComponent(slug)}/engagements/${encodeURIComponent(engagementUuid)}/invoices`,
    /** Mükellef detayı içinde fatura ayrıntısı; sekme çubuğu korunur (#468). */
    engagementInvoice: (slug: string, engagementUuid: string, id: string) =>
      `/t/${encodeURIComponent(slug)}/engagements/${encodeURIComponent(engagementUuid)}/invoices/${encodeURIComponent(id)}`,
    /** Uyarılar sekmesi: mükellefin uyarıları (#468). */
    engagementWarnings: (slug: string, engagementUuid: string) =>
      `/t/${encodeURIComponent(slug)}/engagements/${encodeURIComponent(engagementUuid)}/warnings`,
    /** Mükellefin açık engelleyici uyarıları (#562, #567). */
    engagementBlockers: (slug: string, engagementUuid: string) =>
      `/t/${encodeURIComponent(slug)}/engagements/${encodeURIComponent(engagementUuid)}/warnings?severity=blocker`,
    /** Fişler sekmesi: mükellefin yevmiye taslakları ve çıktısı (#468). */
    engagementJournals: (slug: string, engagementUuid: string) =>
      `/t/${encodeURIComponent(slug)}/engagements/${encodeURIComponent(engagementUuid)}/journals`,
    /** İnceleme sekmesi: mükellefin öneri kuyruğu (#468). */
    engagementReview: (slug: string, engagementUuid: string) =>
      `/t/${encodeURIComponent(slug)}/engagements/${encodeURIComponent(engagementUuid)}/review`,
    /** Yüklemeler sekmesi: mükellefin UBL yüklemeleri (#468). */
    engagementUpload: (slug: string, engagementUuid: string) =>
      `/t/${encodeURIComponent(slug)}/engagements/${encodeURIComponent(engagementUuid)}/upload`,
    /** Bank transactions of one engagement (#329). */
    engagementBank: (slug: string, engagementUuid: string) =>
      `/t/${encodeURIComponent(slug)}/engagements/${encodeURIComponent(engagementUuid)}/bank`,
    /** Personel, bordro ve SGK of one engagement (#382). */
    engagementPayroll: (slug: string, engagementUuid: string) =>
      `/t/${encodeURIComponent(slug)}/engagements/${encodeURIComponent(engagementUuid)}/payroll`,
    /** KDV1, MUHSGK ve Ba/Bs beyan önizlemesi (#390). */
    engagementTaxReturns: (slug: string, engagementUuid: string) =>
      `/t/${encodeURIComponent(slug)}/engagements/${encodeURIComponent(engagementUuid)}/tax-returns`,
    /** Easytrade sync, financial transactions and Z reports (#385). */
    engagementEasytrade: (slug: string, engagementUuid: string) =>
      `/t/${encodeURIComponent(slug)}/engagements/${encodeURIComponent(engagementUuid)}/easytrade`,
    /** Mükellefe özgü dış bağlantılar: banka web servisi, Easytrade … (#518). */
    engagementConnections: (slug: string, engagementUuid: string) =>
      `/t/${encodeURIComponent(slug)}/engagements/${encodeURIComponent(engagementUuid)}/connections`,
    /** Genel Excel / CSV içe aktarma sihirbazı (#395, #518). */
    engagementExcelImports: (slug: string, engagementUuid: string) =>
      `/t/${encodeURIComponent(slug)}/engagements/${encodeURIComponent(engagementUuid)}/excel-imports`,
    /** KVKK ilgili kişi talepleri (#193, #518). */
    engagementPrivacy: (slug: string, engagementUuid: string) =>
      `/t/${encodeURIComponent(slug)}/engagements/${encodeURIComponent(engagementUuid)}/privacy`,
    /** Expenses (masraf) of one engagement (#384). */
    engagementExpenses: (slug: string, engagementUuid: string) =>
      `/t/${encodeURIComponent(slug)}/engagements/${encodeURIComponent(engagementUuid)}/expenses`,
    /** Mutabakat panosu: mizan – kaynak ve cari mutabakat (#389). */
    engagementReconciliation: (slug: string, engagementUuid: string) =>
      `/t/${encodeURIComponent(slug)}/engagements/${encodeURIComponent(engagementUuid)}/reconciliation`,
    /** İşletme hesabı: işletme defteri / serbest meslek gelir-gider (#780). */
    engagementOperatingAccount: (slug: string, engagementUuid: string) =>
      `/t/${encodeURIComponent(slug)}/engagements/${encodeURIComponent(engagementUuid)}/operating-account`,
    /** Mükellef mizanı: yüklenen Luca mizanı, Excel / PDF (#669 E). */
    engagementTrialBalance: (slug: string, engagementUuid: string) =>
      `/t/${encodeURIComponent(slug)}/engagements/${encodeURIComponent(engagementUuid)}/trial-balance`,
    /** Z reports (ÖKC gün sonu) of one engagement (#384). */
    engagementZReports: (slug: string, engagementUuid: string) =>
      `/t/${encodeURIComponent(slug)}/engagements/${encodeURIComponent(engagementUuid)}/z-reports`,
    /** Template-based petitions of one engagement (issue #56). */
    engagementPetitions: (slug: string, engagementUuid: string) =>
      `/t/${encodeURIComponent(slug)}/engagements/${encodeURIComponent(engagementUuid)}/petitions`,
    /** Mükellef raporu: dörtlü grafik, PDF, e-posta (#386). */
    engagementReport: (slug: string, engagementUuid: string) =>
      `/t/${encodeURIComponent(slug)}/engagements/${encodeURIComponent(engagementUuid)}/report`,
  },
  /** Mükellef (taxpayer) shell — company users invited by an office. */
  taxpayer: {
    /** Company picker for users with several companies. */
    picker: "/m",
    home: (uuid: string) => `/m/${encodeURIComponent(uuid)}`,
    invoices: (uuid: string) => `/m/${encodeURIComponent(uuid)}/invoices`,
    invoice: (uuid: string, invoice: string) =>
      `/m/${encodeURIComponent(uuid)}/invoices/${encodeURIComponent(invoice)}`,
    requests: (uuid: string) => `/m/${encodeURIComponent(uuid)}/requests`,
    receipts: (uuid: string) => `/m/${encodeURIComponent(uuid)}/receipts`,
    uploads: (uuid: string) => `/m/${encodeURIComponent(uuid)}/uploads`,
    /** Aylık / yıllık rapor (#386). */
    report: (uuid: string) => `/m/${encodeURIComponent(uuid)}/report`,
    /** Bordro PDF'leri, salt okuma (#382). */
    payroll: (uuid: string) => `/m/${encodeURIComponent(uuid)}/payroll`,
    /** Müşavirin paylaştığı arşiv belgeleri, salt okuma (#682). */
    archive: (uuid: string) => `/m/${encodeURIComponent(uuid)}/archive`,
  },
  /** Organization invitation link (public; the token is the credential). */
  invite: (token: string) => `/invite/${encodeURIComponent(token)}`,
  /** Mükellef invitation link sent by an office (public). */
  inviteTaxpayer: (token: string) => `/invite/m/${encodeURIComponent(token)}`,
  /** Cari mutabakat mektubu yanıt bağlantısı (#389, public). */
  reconciliationResponse: (token: string) =>
    `/mutabakat/${encodeURIComponent(token)}`,
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
    /** Kullanıcıyı rolünün kılavuzuna yönlendirir (#522). */
    mine: () => "/yardim/benim",
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
