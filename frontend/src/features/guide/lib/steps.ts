import { routes } from "@/config/routes";

/**
 * Rehber sayfasının adımları. Ekran adları menüden ve ekranların kendi
 * metinlerinden gelir; ana dalda henüz olmayan bir özellik `soon` ile
 * "Yakında" rozeti alır ve bağlantısı olmaz.
 */
export type GuideStep = {
  id: string;
  titleKey: string;
  bodyKey: string;
  /** "Bu ekrana git" hedefi; `soon` adımlarında yok. */
  href?: (slug: string) => string;
  soon?: boolean;
};

export type GuideSection = {
  id: string;
  titleKey: string;
  descriptionKey: string;
  /** Sıralı adımlar (Her ay) numaralı gösterilir. */
  ordered?: boolean;
  steps: GuideStep[];
};

export const GUIDE_SECTIONS: GuideSection[] = [
  {
    id: "setup",
    titleKey: "help.guide.setup.title",
    descriptionKey: "help.guide.setup.description",
    steps: [
      {
        id: "add-taxpayer",
        titleKey: "help.guide.setup.add_taxpayer.title",
        bodyKey: "help.guide.setup.add_taxpayer.body",
        href: routes.tenant.taxpayerRequests,
      },
      {
        id: "excel-import",
        titleKey: "help.guide.setup.excel_import.title",
        bodyKey: "help.guide.setup.excel_import.body",
        href: routes.tenant.setup,
      },
      {
        // Ofis panelinde mükellef paneli / mobil uygulama davet ekranı yok.
        id: "app-invite",
        titleKey: "help.guide.setup.app_invite.title",
        bodyKey: "help.guide.setup.app_invite.body",
        soon: true,
      },
      {
        id: "integrator",
        titleKey: "help.guide.setup.integrator.title",
        bodyKey: "help.guide.setup.integrator.body",
        href: routes.tenant.taxpayers,
      },
      {
        id: "bank-statement",
        titleKey: "help.guide.setup.bank_statement.title",
        bodyKey: "help.guide.setup.bank_statement.body",
        href: routes.tenant.taxpayers,
      },
      {
        id: "vat-threshold",
        titleKey: "help.guide.setup.vat_threshold.title",
        bodyKey: "help.guide.setup.vat_threshold.body",
        href: routes.tenant.settings,
      },
      {
        id: "desktop-agent",
        titleKey: "help.guide.setup.desktop_agent.title",
        bodyKey: "help.guide.setup.desktop_agent.body",
        href: routes.tenant.agents,
      },
      {
        id: "whatsapp",
        titleKey: "help.guide.setup.whatsapp.title",
        bodyKey: "help.guide.setup.whatsapp.body",
        href: routes.tenant.whatsapp,
      },
    ],
  },
  {
    id: "monthly",
    titleKey: "help.guide.monthly.title",
    descriptionKey: "help.guide.monthly.description",
    ordered: true,
    steps: [
      {
        id: "documents",
        titleKey: "help.guide.monthly.documents.title",
        bodyKey: "help.guide.monthly.documents.body",
        href: (slug) => routes.tenant.documentCollection(slug),
      },
      {
        id: "next-task",
        titleKey: "help.guide.monthly.next_task.title",
        bodyKey: "help.guide.monthly.next_task.body",
        href: routes.tenant.copilot,
      },
      {
        id: "taxpayer-check",
        titleKey: "help.guide.monthly.taxpayer_check.title",
        bodyKey: "help.guide.monthly.taxpayer_check.body",
        href: routes.tenant.taxpayers,
      },
      {
        id: "vat",
        titleKey: "help.guide.monthly.vat.title",
        bodyKey: "help.guide.monthly.vat.body",
        href: routes.tenant.reports,
      },
      {
        // Mükellef sayfasında "Luca'ya aktar" düğmesi henüz yok; ofis
        // genelindeki Luca / Zirve çıktısı ekranına gider.
        id: "luca-export",
        titleKey: "help.guide.monthly.luca_export.title",
        bodyKey: "help.guide.monthly.luca_export.body",
        href: routes.tenant.journals,
      },
      {
        id: "luca-import",
        titleKey: "help.guide.monthly.luca_import.title",
        bodyKey: "help.guide.monthly.luca_import.body",
        href: routes.tenant.taxpayers,
      },
      {
        id: "close",
        titleKey: "help.guide.monthly.close.title",
        bodyKey: "help.guide.monthly.close.body",
        href: routes.tenant.copilot,
      },
    ],
  },
  {
    // Kilitli ay seçenekleri ayrı bir PR'da; birleşene kadar "Yakında".
    id: "locked",
    titleKey: "help.guide.locked.title",
    descriptionKey: "help.guide.locked.description",
    steps: [
      {
        id: "reject",
        titleKey: "help.guide.locked.reject.title",
        bodyKey: "help.guide.locked.reject.body",
        soon: true,
      },
      {
        id: "correction",
        titleKey: "help.guide.locked.correction.title",
        bodyKey: "help.guide.locked.correction.body",
        soon: true,
      },
      {
        id: "apply",
        titleKey: "help.guide.locked.apply.title",
        bodyKey: "help.guide.locked.apply.body",
        soon: true,
      },
    ],
  },
  {
    id: "returns",
    titleKey: "help.guide.returns.title",
    descriptionKey: "help.guide.returns.description",
    steps: [
      {
        id: "tax-returns",
        titleKey: "help.guide.returns.track.title",
        bodyKey: "help.guide.returns.track.body",
        href: routes.tenant.taxReturns,
      },
    ],
  },
];

/**
 * Tur adımları: `target` kenar menüdeki öğenin `data-tour` değeri (menü
 * öğesinin id'si). Başlık menüdeki adın kendisidir; hedef görünmüyorsa
 * (telefon, yetki) kart ortada açılır.
 */
export type TourStep = {
  id: string;
  target?: string;
  titleKey: string;
  bodyKey: string;
};

export const TOUR_STEPS: TourStep[] = [
  {
    id: "welcome",
    titleKey: "help.guide.tour.welcome.title",
    bodyKey: "help.guide.tour.welcome.body",
  },
  {
    id: "taxpayer-requests",
    target: "taxpayer-requests",
    titleKey: "onboarding.nav_title",
    bodyKey: "help.guide.tour.taxpayer_requests",
  },
  {
    id: "document-collection",
    target: "document-collection",
    titleKey: "office.collection.nav",
    bodyKey: "help.guide.tour.document_collection",
  },
  {
    id: "copilot",
    target: "copilot",
    titleKey: "office.nav_copilot",
    bodyKey: "help.guide.tour.copilot",
  },
  {
    id: "review",
    target: "review",
    titleKey: "office.nav_review",
    bodyKey: "help.guide.tour.review",
  },
  {
    id: "taxpayers",
    target: "taxpayers",
    titleKey: "office.nav_taxpayers",
    bodyKey: "help.guide.tour.taxpayers",
  },
  {
    id: "journals",
    target: "journals",
    titleKey: "office.journals.nav",
    bodyKey: "help.guide.tour.journals",
  },
  {
    id: "tax-returns",
    target: "tax-returns",
    titleKey: "office.nav_tax_returns",
    bodyKey: "help.guide.tour.tax_returns",
  },
  {
    id: "agents",
    target: "agents",
    titleKey: "office.agents.nav",
    bodyKey: "help.guide.tour.agents",
  },
  {
    id: "guide",
    target: "guide",
    titleKey: "help.guide.nav",
    bodyKey: "help.guide.tour.guide",
  },
];
