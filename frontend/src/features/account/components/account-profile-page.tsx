"use client";

import type { ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  Fingerprint,
  KeyRound,
  Link2,
  Monitor,
  ShieldCheck,
  UserRound,
} from "lucide-react";
import { toast } from "sonner";

import { PageHeader } from "@/components/layout";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/common/card";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import { ChangePasswordForm } from "@/features/account/components/change-password-form";
import { DeleteAccountDialog } from "@/features/account/components/delete-account-dialog";
import { OAuthIdentityManager } from "@/features/account/components/github-identity-manager";
import { NotificationPreferencesForm } from "@/features/account/components/notification-preferences-form";
import { PasskeyManager } from "@/features/account/components/passkey-manager";
import { SessionManager } from "@/features/account/components/session-manager";
import { TotpManager } from "@/features/account/components/totp-manager";
import { ProfileEditForm } from "@/features/account/components/profile-edit-form";
import { ProfilePermissionsList } from "@/features/account/components/profile-permissions-list";
import { MyLegalAcceptancesCard } from "@/features/legal";
import { isApiError } from "@/lib/api";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";
import { authService } from "@/services/auth.service";
import { fetchAppPublicConfig } from "@/services/app-config.service";
import { cn } from "@/lib/utils";

type AccountShell = "platform" | "cms";

/**
 * Profilin açıldığı kapsam (#890 D): ofis kabuğunda ofis adı ve üyelik rolü,
 * mükellef kabuğunda şirket adı ve şirketteki rol. `permissions` verilirse
 * "Yetkilerim" bölümü bu kapsamın izinlerini gösterir (ofis üyeliği).
 */
export type AccountProfileScope = {
  kind: "office" | "company";
  name: string;
  role: string;
  permissions?: string[];
};

type SectionId = "kisisel" | "guvenlik" | "bildirimler" | "yetkiler" | "hesap";

function homeHref(shell: AccountShell) {
  return shell === "platform" ? routes.platform.home : routes.cms.home;
}

function initials(fullName?: string) {
  const parts = fullName?.trim().split(/\s+/).filter(Boolean) ?? [];
  const a = parts[0]?.[0] ?? "";
  const b = parts.length > 1 ? (parts[parts.length - 1]?.[0] ?? "") : "";
  return `${a}${b}`.toLocaleUpperCase("tr-TR") || "?";
}

/**
 * Hesap / profil ekranı (#890 D). Üstte ad, e-posta, rol ve ofis kartı;
 * altında bölümler: kişisel bilgiler, güvenlik, bildirim tercihleri,
 * yetkilerim ve hesap. Geniş ekranda bölüm menüsü solda, dar ekranda üstte
 * (ofis ayarları ekranıyla aynı düzen). `home` verilirse ekmek kırıntısındaki
 * ana sayfa oraya gider (ofis ya da mükellef kabuğunda açılan profil, #890 C).
 */
export function AccountProfilePage({
  shell = "cms",
  home,
  scope,
}: {
  shell?: AccountShell;
  home?: string;
  scope?: AccountProfileScope;
}) {
  const { t, locale } = useLocale();
  const { user, hydrateProfile } = useAuth();
  const { data: appConfig } = useQuery({
    queryKey: ["app", "config", locale],
    queryFn: () => fetchAppPublicConfig(locale),
  });

  /** Çevirisi yoksa ham anahtarı değil, verilen yedeği gösterir. */
  const label = (key: string, fallback: string) => {
    const msg = t(key);
    return msg === key ? fallback : msg;
  };

  const globalRoles = user?.roles?.length
    ? user.roles
    : user?.isSuperAdmin
      ? ["super_admin"]
      : [];
  const roleLabels = scope
    ? [scope.role]
    : globalRoles.map((role) => label(`auth.profile.role.${role}`, role));
  const granted = scope?.permissions ?? user?.permissions;

  const requestVerify = async () => {
    try {
      await authService.requestEmailVerify(user?.email);
      toast.success(t("auth.verify.request_success"));
    } catch (error) {
      if (isApiError(error)) {
        toast.error(error.message || t("common.error_generic"));
      } else {
        toast.error(t("common.error_generic"));
      }
    } finally {
      await hydrateProfile();
    }
  };

  const sections: { id: SectionId; label: string }[] = [
    { id: "kisisel", label: t("auth.profile.section_personal") },
    { id: "guvenlik", label: t("auth.profile.section_security") },
    { id: "bildirimler", label: t("auth.profile.section_notifications") },
    { id: "yetkiler", label: t("auth.profile.section_permissions") },
    { id: "hesap", label: t("auth.profile.section_account") },
  ];

  return (
    <div className="min-w-0 space-y-6">
      <PageHeader
        title={t("auth.profile.title")}
        description={t("auth.profile.page_description")}
        icon={<UserRound className="size-6" />}
        breadcrumbs={[
          { label: t("layout.breadcrumb_home"), href: home ?? homeHref(shell) },
          { label: t("auth.profile") },
        ]}
      />

      <Card className="shadow-none">
        <section
          aria-label={t("auth.profile.summary_label")}
          className="flex flex-wrap items-center gap-4 p-4"
        >
          <Avatar className="size-14 shrink-0">
            <AvatarFallback className="text-lg font-semibold">
              {initials(user?.fullName)}
            </AvatarFallback>
          </Avatar>
          <div className="min-w-0 flex-1 basis-48 space-y-1">
            <p className="font-display truncate text-lg font-semibold">
              {user?.fullName || "—"}
            </p>
            <p className="text-text-2 truncate text-sm" title={user?.email}>
              {user?.email || "—"}
            </p>
            <div className="flex flex-wrap items-center gap-2 pt-1">
              {user?.emailVerified ? (
                <Badge variant="success">{t("auth.profile.verified")}</Badge>
              ) : (
                <>
                  <Badge variant="warning">
                    {t("auth.profile.unverified")}
                  </Badge>
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    onClick={requestVerify}
                  >
                    {t("auth.verify.request")}
                  </Button>
                </>
              )}
              {user?.status && user.status !== "active" ? (
                <Badge variant="secondary">
                  {label(`auth.profile.status.${user.status}`, user.status)}
                </Badge>
              ) : null}
            </div>
          </div>
          <dl className="grid w-full min-w-0 gap-3 sm:w-auto sm:grid-cols-2 sm:gap-6">
            <Fact
              label={t("auth.profile.role")}
              value={
                roleLabels.length ? (
                  <div className="flex flex-wrap gap-1.5">
                    {roleLabels.map((role) => (
                      <Badge key={role} variant="secondary">
                        {role}
                      </Badge>
                    ))}
                  </div>
                ) : (
                  "—"
                )
              }
            />
            {scope ? (
              <Fact
                label={
                  scope.kind === "office"
                    ? t("auth.profile.office")
                    : t("auth.profile.company")
                }
                value={
                  <span className="block truncate" title={scope.name}>
                    {scope.name}
                  </span>
                }
              />
            ) : null}
          </dl>
        </section>
      </Card>

      <div className="grid grid-cols-[minmax(0,1fr)] gap-6 lg:grid-cols-[220px_minmax(0,1fr)]">
        <ProfileNav sections={sections} />
        <div className="flex min-w-0 flex-col gap-10">
          <Section
            id="kisisel"
            title={t("auth.profile.section_personal")}
            description={t("auth.profile.section_personal_description")}
          >
            <Card className="shadow-none">
              <CardHeader>
                <CardTitle>{t("auth.profile.edit_title")}</CardTitle>
                <CardHint>{t("auth.profile.edit_hint")}</CardHint>
              </CardHeader>
              <CardContent className="space-y-5">
                <dl className="grid gap-4 sm:grid-cols-2">
                  <Fact
                    label={t("auth.profile.email")}
                    value={
                      <span className="break-all">{user?.email || "—"}</span>
                    }
                  />
                  <Fact
                    label={t("auth.profile.email_verified")}
                    value={
                      user?.emailVerified ? (
                        <Badge variant="success">
                          {t("auth.profile.verified")}
                        </Badge>
                      ) : (
                        <Badge variant="warning">
                          {t("auth.profile.unverified")}
                        </Badge>
                      )
                    }
                  />
                </dl>
                <div className="border-border border-t pt-5">
                  <ProfileEditForm />
                </div>
              </CardContent>
            </Card>
          </Section>

          <Section
            id="guvenlik"
            title={t("auth.profile.section_security")}
            description={t("auth.profile.section_security_description")}
          >
            <div className="grid gap-4 xl:grid-cols-2 xl:items-start">
              <Card className="shadow-none">
                <CardHeader>
                  <CardTitle className="flex items-center gap-2">
                    <KeyRound
                      className="text-muted-foreground size-5 shrink-0"
                      aria-hidden
                    />
                    {t("auth.password.title")}
                  </CardTitle>
                  <CardHint>{t("auth.profile.change_password_body")}</CardHint>
                </CardHeader>
                <CardContent>
                  <ChangePasswordForm />
                </CardContent>
              </Card>

              <Card className="shadow-none">
                <CardHeader>
                  <CardTitle className="flex items-center gap-2">
                    <ShieldCheck
                      className="text-muted-foreground size-5 shrink-0"
                      aria-hidden
                    />
                    {t("auth.totp.title")}
                  </CardTitle>
                  <CardHint>{t("auth.totp.description")}</CardHint>
                </CardHeader>
                <CardContent>
                  <TotpManager />
                </CardContent>
              </Card>

              <Card className="shadow-none">
                <CardHeader>
                  <CardTitle className="flex items-center gap-2">
                    <Fingerprint
                      className="text-muted-foreground size-5 shrink-0"
                      aria-hidden
                    />
                    {t("auth.passkey.title")}
                  </CardTitle>
                  <CardHint>{t("auth.passkey.description")}</CardHint>
                </CardHeader>
                <CardContent>
                  <PasskeyManager />
                </CardContent>
              </Card>

              <Card className="shadow-none">
                <CardHeader>
                  <CardTitle className="flex items-center gap-2">
                    <Link2
                      className="text-muted-foreground size-5 shrink-0"
                      aria-hidden
                    />
                    {t("auth.oauth.title")}
                  </CardTitle>
                  <CardHint>{t("auth.oauth.profile_hint")}</CardHint>
                </CardHeader>
                <CardContent>
                  <OAuthIdentityManager
                    providers={[
                      {
                        id: "github",
                        enabled: Boolean(appConfig?.auth_methods.github.login),
                      },
                      {
                        id: "google",
                        enabled: Boolean(appConfig?.auth_methods.google.login),
                      },
                      {
                        id: "facebook",
                        enabled: Boolean(
                          appConfig?.auth_methods.facebook.login,
                        ),
                      },
                      {
                        id: "apple",
                        enabled: Boolean(appConfig?.auth_methods.apple.login),
                      },
                    ]}
                  />
                </CardContent>
              </Card>
            </div>

            <Card className="shadow-none">
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <Monitor
                    className="text-muted-foreground size-5 shrink-0"
                    aria-hidden
                  />
                  {t("auth.sessions.title")}
                </CardTitle>
                <CardHint>{t("auth.sessions.description")}</CardHint>
              </CardHeader>
              <CardContent>
                <SessionManager />
              </CardContent>
            </Card>
          </Section>

          <Section
            id="bildirimler"
            title={t("auth.profile.section_notifications")}
            description={t("auth.profile.section_notifications_description")}
          >
            <Card className="shadow-none">
              <CardContent>
                <NotificationPreferencesForm />
              </CardContent>
            </Card>
          </Section>

          <Section
            id="yetkiler"
            title={t("auth.profile.section_permissions")}
            description={t("auth.profile.section_permissions_description")}
          >
            <Card className="shadow-none">
              <CardHeader>
                <CardTitle className="flex flex-wrap items-center gap-2">
                  <span>{t("auth.profile.role")}</span>
                  {roleLabels.map((role) => (
                    <Badge key={role} variant="secondary">
                      {role}
                    </Badge>
                  ))}
                </CardTitle>
                <CardHint>
                  {scope
                    ? t("auth.profile.permissions_scope_hint")
                    : t("auth.profile.permissions_hint")}
                </CardHint>
              </CardHeader>
              <CardContent>
                <ProfilePermissionsList granted={granted} />
              </CardContent>
            </Card>
          </Section>

          <Section
            id="hesap"
            title={t("auth.profile.section_account")}
            description={t("auth.profile.section_account_description")}
          >
            <MyLegalAcceptancesCard />
            <Card className="border-destructive/40 shadow-none">
              <CardHeader>
                <CardTitle>{t("auth.delete_account.title")}</CardTitle>
                <CardHint>{t("auth.delete_account.description")}</CardHint>
              </CardHeader>
              <CardContent>
                <DeleteAccountDialog />
              </CardContent>
            </Card>
          </Section>
        </div>
      </div>
    </div>
  );
}

/** Bölüm menüsü: geniş ekranda solda yapışık, dar ekranda üstte kayar. */
function ProfileNav({
  sections,
}: {
  sections: { id: SectionId; label: string }[];
}) {
  const { t } = useLocale();
  return (
    <nav
      aria-label={t("auth.profile.nav_label")}
      className="min-w-0 lg:sticky lg:top-20 lg:self-start"
    >
      <ul className="flex gap-1 overflow-x-auto pb-1 lg:flex-col lg:overflow-visible">
        {sections.map((s) => (
          <li key={s.id} className="shrink-0">
            <a
              href={`#${s.id}`}
              className="hover:bg-surface-2 text-text-2 hover:text-foreground flex min-h-11 items-center rounded-md px-3 text-sm font-medium whitespace-nowrap"
            >
              {s.label}
            </a>
          </li>
        ))}
      </ul>
    </nav>
  );
}

function Section({
  id,
  title,
  description,
  children,
}: {
  id: SectionId;
  title: string;
  description?: string;
  children: ReactNode;
}) {
  return (
    <section
      id={id}
      aria-labelledby={`${id}-heading`}
      className="min-w-0 scroll-mt-20 space-y-4"
    >
      <div>
        <h2
          id={`${id}-heading`}
          className="font-display text-xl font-semibold tracking-tight"
        >
          {title}
        </h2>
        {description ? (
          <p className="text-text-2 text-sm">{description}</p>
        ) : null}
      </div>
      {children}
    </section>
  );
}

function CardHint({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) {
  return (
    <p className={cn("text-muted-foreground text-sm", className)}>{children}</p>
  );
}

function Fact({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="min-w-0 space-y-1">
      <dt className="text-muted-foreground text-xs font-medium tracking-wide uppercase">
        {label}
      </dt>
      <dd className="min-w-0 text-sm font-medium">{value}</dd>
    </div>
  );
}
