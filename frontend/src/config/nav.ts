import {
  Activity,
  Bell,
  BookOpenCheck,
  BrainCircuit,
  ChartColumn,
  LifeBuoy,
  MessageCircle,
  MonitorSmartphone,
  ClipboardCheck,
  FileCheck2,
  FileText,
  TriangleAlert,
  Building2,
  UserPlus,
  UsersRound,
  Cpu,
  Download,
  HardDrive,
  KeyRound,
  LayoutDashboard,
  LogIn,
  Palette,
  ScrollText,
  Settings2,
  Share2,
  Shield,
  Sparkles,
  Upload,
  Waypoints,
} from "lucide-react";

import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { defineNav } from "@/features/nav-engine";
import type { NavCatalog } from "@/features/nav-engine";
import { usersNavItem } from "@/features/users/nav";

const integrationReadPermissions = [
  permissions.integrations.google.read,
  permissions.integrations.github.read,
  permissions.integrations.facebook.read,
  permissions.integrations.apple.read,
];

/**
 * System Settings sub-sidebar (`/platform/settings/*`). Rendered by
 * `features/settings` (not by `NavEngine`), but uses the same catalog shape
 * and `permission` / `anyPermission` gating.
 */
export const settingsNav = defineNav({
  id: "settings",
  groups: [
    {
      id: "system",
      labelKey: "settings.subnav.group_system",
      icon: Activity,
      defaultOpen: true,
      items: [
        {
          id: "system-overview",
          titleKey: "settings.subnav.system_overview",
          href: routes.platform.settings.root,
          icon: Activity,
          permission: permissions.settings.read,
        },
        {
          id: "system-environment",
          titleKey: "settings.subnav.system_environment",
          href: routes.platform.settings.environment,
          icon: Cpu,
          permission: permissions.settings.read,
        },
        {
          id: "logs",
          titleKey: "layout.nav_logs",
          href: routes.platform.settings.logs,
          icon: ScrollText,
          permission: permissions.logs.read,
        },
        {
          id: "activity",
          titleKey: "layout.nav_activity",
          href: routes.platform.settings.activity,
          icon: Activity,
          permission: permissions.activity.read,
        },
      ],
    },
    {
      id: "general",
      labelKey: "settings.subnav.group_general",
      icon: Settings2,
      defaultOpen: true,
      items: [
        {
          id: "general-branding",
          titleKey: "settings.subnav.general_branding",
          href: routes.platform.settings.general,
          icon: Palette,
          permission: permissions.settings.read,
        },
      ],
    },
    {
      id: "security",
      labelKey: "settings.subnav.group_security",
      icon: Shield,
      defaultOpen: true,
      items: [
        {
          id: "roles",
          titleKey: "layout.nav_roles",
          href: routes.platform.settings.roles,
          icon: Shield,
          permission: permissions.roles.read,
        },
        {
          id: "auth-settings",
          titleKey: "settings.subnav.auth_methods",
          href: routes.platform.settings.auth,
          icon: LogIn,
          permission: permissions.authSettings.read,
        },
        {
          id: "access-policy",
          titleKey: "settings.subnav.access_policy",
          href: routes.platform.settings.access,
          icon: KeyRound,
          permission: permissions.access.read,
        },
      ],
    },
    {
      id: "integrations",
      labelKey: "settings.subnav.group_integrations",
      icon: Share2,
      defaultOpen: true,
      items: [
        {
          id: "oauth-integrations",
          titleKey: "settings.subnav.oauth_providers",
          href: routes.platform.settings.integrations,
          icon: Share2,
          anyPermission: integrationReadPermissions,
        },
        {
          id: "storage",
          titleKey: "settings.subnav.storage",
          href: routes.platform.settings.storage,
          icon: HardDrive,
          permission: permissions.storage.read,
        },
      ],
    },
    {
      id: "io",
      labelKey: "settings.subnav.group_maintenance",
      icon: Upload,
      defaultOpen: true,
      items: [
        {
          id: "imports",
          titleKey: "layout.nav_imports",
          href: routes.platform.settings.imports,
          icon: Upload,
          permission: permissions.imports.read,
        },
        {
          id: "exports",
          titleKey: "layout.nav_exports",
          href: routes.platform.settings.exports,
          icon: Download,
          permission: permissions.exports.read,
        },
        {
          id: "purge-rules",
          titleKey: "settings.subnav.purge_rules",
          href: routes.platform.settings.maintenance,
          icon: Sparkles,
          permission: permissions.logs.read,
        },
      ],
    },
  ],
});

/** Every slug that unlocks at least one settings subpage (OR-gate for the shell). */
function catalogPermissions(catalog: NavCatalog): string[] {
  const slugs = new Set<string>();
  for (const group of catalog.groups) {
    for (const item of group.items) {
      if (typeof item.permission === "string") slugs.add(item.permission);
      else if (item.permission) item.permission.forEach((p) => slugs.add(p));
      item.anyPermission?.forEach((p) => slugs.add(p));
    }
  }
  return [...slugs].sort();
}

export const settingsNavPermissions = catalogPermissions(settingsNav);

export const platformNav = defineNav({
  id: "platform",
  groups: [
    {
      id: "general",
      labelKey: "layout.nav_platform",
      icon: LayoutDashboard,
      defaultOpen: true,
      items: [
        {
          id: "home",
          titleKey: "layout.home",
          href: routes.platform.home,
          icon: LayoutDashboard,
        },
      ],
    },
    {
      id: "manage",
      labelKey: "layout.section_manage",
      icon: Shield,
      defaultOpen: true,
      items: [
        usersNavItem,
        {
          id: "organizations",
          titleKey: "organizations.title",
          href: routes.platform.organizations.root,
          icon: Building2,
          permission: permissions.organizations.read,
        },
        {
          id: "notifications",
          titleKey: "layout.nav_notifications",
          href: routes.platform.notifications.root,
          icon: Bell,
          permission: permissions.notifications.platformRead,
        },
        {
          id: "settings",
          titleKey: "layout.nav_settings",
          href: routes.platform.settings.root,
          icon: Settings2,
          anyPermission: settingsNavPermissions,
        },
      ],
    },
  ],
});

export const cmsNav = defineNav({
  id: "cms",
  groups: [
    {
      id: "cms",
      labelKey: "layout.section_cms",
      icon: LayoutDashboard,
      defaultOpen: true,
      items: [
        {
          id: "home",
          titleKey: "layout.home",
          href: routes.cms.home,
          icon: LayoutDashboard,
        },
      ],
    },
  ],
});

/**
 * Organization shell sidebar (`/t/{slug}`). Items are gated by the
 * permissions of the membership in that organization (the tenant layout
 * swaps the permission context), never by global roles.
 */
export function tenantNav(slug: string): NavCatalog {
  return defineNav({
    id: "tenant",
    // Arayüz v1 (#326, docs/tasarim/tuval/OfisNav.dc.html): Bugün / Kayıtlar /
    // Ofis. Sayaçlar ofis kabuğunda (features/office/shell) eklenir.
    groups: [
      {
        id: "today",
        labelKey: "office.shell.group_today",
        icon: LayoutDashboard,
        collapsible: false,
        items: [
          {
            id: "home",
            titleKey: "layout.home",
            href: routes.tenant.home(slug),
            icon: LayoutDashboard,
          },
          {
            // Co-pilot: sıradaki iş (#392).
            id: "copilot",
            titleKey: "office.nav_copilot",
            href: routes.tenant.copilot(slug),
            icon: Sparkles,
          },
          {
            id: "review",
            titleKey: "office.nav_review",
            href: routes.tenant.review(slug),
            icon: ClipboardCheck,
          },
          {
            id: "warnings",
            titleKey: "office.nav_warnings",
            href: routes.tenant.warnings(slug),
            icon: TriangleAlert,
          },
        ],
      },
      {
        id: "records",
        labelKey: "office.shell.group_records",
        icon: FileText,
        collapsible: false,
        items: [
          {
            id: "taxpayers",
            titleKey: "office.nav_taxpayers",
            href: routes.tenant.taxpayers(slug),
            icon: Building2,
          },
          {
            id: "tax-returns",
            titleKey: "office.nav_tax_returns",
            href: routes.tenant.taxReturns(slug),
            icon: FileCheck2,
          },
          {
            id: "invoices",
            titleKey: "office.nav_invoices",
            href: routes.tenant.invoices(slug),
            icon: FileText,
          },
          {
            id: "reports",
            titleKey: "office.nav_reports",
            href: routes.tenant.reports(slug),
            icon: ChartColumn,
          },
          {
            id: "account-plan",
            titleKey: "office.memory.nav",
            href: routes.tenant.accountPlan(slug),
            icon: BrainCircuit,
            permission: permissions.tenant.accountPlan.read,
          },
          {
            id: "journals",
            titleKey: "office.journals.nav",
            href: routes.tenant.journals(slug),
            icon: BookOpenCheck,
            permission: permissions.tenant.journals.read,
          },
          {
            id: "upload",
            titleKey: "office.nav_upload",
            href: routes.tenant.upload(slug),
            icon: Upload,
          },
        ],
      },
      {
        id: "office",
        labelKey: "office.shell.group_office",
        icon: Building2,
        collapsible: false,
        items: [
          {
            id: "command",
            titleKey: "office.nav_command",
            href: routes.tenant.command(slug),
            icon: Waypoints,
          },
          {
            id: "taxpayer-requests",
            titleKey: "onboarding.nav_title",
            href: routes.tenant.taxpayerRequests(slug),
            icon: UserPlus,
            permission: permissions.tenant.engagements.read,
          },
          {
            id: "agents",
            titleKey: "office.agents.nav",
            href: routes.tenant.agents(slug),
            icon: MonitorSmartphone,
            permission: permissions.tenant.agents.read,
          },
          {
            id: "whatsapp",
            titleKey: "office.whatsapp.nav",
            href: routes.tenant.whatsapp(slug),
            icon: MessageCircle,
            permission: permissions.tenant.receipts.read,
          },
          {
            id: "members",
            titleKey: "tenant.nav_members",
            href: routes.tenant.members(slug),
            icon: UsersRound,
            permission: permissions.tenant.members.read,
          },
          {
            id: "roles",
            titleKey: "tenant.nav_roles",
            href: routes.tenant.roles(slug),
            icon: Shield,
            permission: permissions.tenant.roles.read,
          },
          {
            id: "settings",
            titleKey: "tenant.nav_settings",
            href: routes.tenant.settings(slug),
            icon: Settings2,
            permission: permissions.tenant.settings.read,
          },
          {
            // Kullanım kılavuzları (#274): rolün kılavuzuna gider (#522).
            id: "help",
            titleKey: "help.nav",
            href: routes.help.mine(),
            icon: LifeBuoy,
          },
        ],
      },
    ],
  });
}

/** Sidebar catalog, home link and command-palette page index of one shell. */
export type ShellNav = {
  catalog: NavCatalog;
  homeHref: string;
  /** Catalogs whose pages the command palette indexes (sidebar + subpages). */
  paletteCatalogs: NavCatalog[];
};

export function shellNav(
  shell:
    | { variant: "platform" }
    | { variant: "cms" }
    | { variant: "tenant"; slug: string },
): ShellNav {
  switch (shell.variant) {
    case "cms":
      return {
        catalog: cmsNav,
        homeHref: routes.cms.home,
        paletteCatalogs: [cmsNav],
      };
    case "tenant": {
      const catalog = tenantNav(shell.slug);
      return {
        catalog,
        homeHref: routes.tenant.home(shell.slug),
        paletteCatalogs: [catalog],
      };
    }
    default:
      // Platform palette also indexes the System Settings sub-pages.
      return {
        catalog: platformNav,
        homeHref: routes.platform.home,
        paletteCatalogs: [platformNav, settingsNav],
      };
  }
}
