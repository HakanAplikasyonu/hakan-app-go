"use client";

import { useEffect, type ReactNode } from "react";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";

import { KristalYukleyici, YetkiYok } from "@/components/hakan";
import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import {
  defaultHomeForUser,
  hasScopedHome,
  isCmsUser,
  isPlatformUser,
  postLoginTarget,
  profilePathFor,
} from "@/lib/auth/types";
import { useAuthStore } from "@/lib/auth/session-store";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";

type RouteGuardProps = {
  children: ReactNode;
  /**
   * `tenant` only requires a session: organization access is decided per
   * membership by the tenant shell, so platform users who are members may
   * enter too.
   */
  mode: "guest" | "platform" | "cms" | "tenant";
};

/** Tam sayfa bekleme: 400 ms sonra kristal yükleyici (#332). */
function PageLoader({ label }: { label: string }) {
  return (
    <div className="flex min-h-svh items-center justify-center">
      <KristalYukleyici etiket={label} />
    </div>
  );
}

function RoleMismatch({ expected, home }: { expected: string; home: string }) {
  const { t } = useLocale();
  return (
    <div className="flex min-h-svh items-center justify-center p-6">
      <YetkiYok
        className="w-full max-w-md"
        aciklama={`${t("errors.role_mismatch")} ${t("errors.area_requires", { area: expected })}`}
        eylem={
          <Button asChild>
            <Link href={home}>{t("errors.continue")}</Link>
          </Button>
        }
      />
    </div>
  );
}

const AUTHENTICATED_MODES = new Set<RouteGuardProps["mode"]>([
  "platform",
  "cms",
  "tenant",
]);

export function RouteGuard({ children, mode }: RouteGuardProps) {
  const router = useRouter();
  const pathname = usePathname();
  const { bootstrapped, isAuthenticated, user } = useAuth();
  const { t } = useLocale();
  const cachedUser = useAuthStore((s) => s.user);

  useEffect(() => {
    if (!bootstrapped) return;

    if (mode === "guest" && isAuthenticated && user) {
      // Same target as the login form: an explicit ?next= wins, otherwise
      // the user's default shell. (Without this the guard raced the form and
      // dropped deep links such as /t/{slug}.)
      const next = new URLSearchParams(window.location.search).get("next");
      router.replace(postLoginTarget(user, next));
      return;
    }

    if (AUTHENTICATED_MODES.has(mode) && !isAuthenticated) {
      // Platform alanı platform girişine, diğerleri sistemin girişine (#890).
      const login =
        mode === "platform" ? routes.platform.login : routes.guest.login;
      // Tenant links are shareable: come back to the same org after login.
      router.replace(
        mode === "tenant" && pathname
          ? `${login}?next=${encodeURIComponent(pathname)}`
          : login,
      );
      return;
    }

    // CMS kabuğu (`/`, `/profile`) yalnızca kapsamı olmayan kullanıcıya;
    // ofis / mükellef kullanıcısı kendi kabuğuna gider (#890 B, C).
    if (mode === "cms" && user && hasScopedHome(user)) {
      router.replace(
        pathname?.startsWith(routes.cms.profile.root)
          ? profilePathFor(user)
          : defaultHomeForUser(user),
      );
      return;
    }

    // Süper admin eski /profile bağlantısında kendi profiline (#890 C).
    if (
      mode === "cms" &&
      user &&
      isPlatformUser(user) &&
      pathname?.startsWith(routes.cms.profile.root)
    ) {
      router.replace(routes.platform.profile.root);
    }
  }, [bootstrapped, mode, isAuthenticated, user, router, pathname]);

  // Show the loader only when there is no cached user to render optimistically.
  // With a cached user, render immediately and let session verification update
  // state in the background.
  if (!bootstrapped && !cachedUser) {
    return <PageLoader label={t("common.loading")} />;
  }

  if (AUTHENTICATED_MODES.has(mode) && !isAuthenticated) {
    return null;
  }

  // Yönlendirme sürerken CMS ekranı bir an bile görünmesin.
  if (
    mode === "cms" &&
    user &&
    (hasScopedHome(user) ||
      (isPlatformUser(user) && pathname?.startsWith(routes.cms.profile.root)))
  ) {
    return <PageLoader label={t("common.loading")} />;
  }

  if (mode === "platform" && user && !isPlatformUser(user)) {
    return (
      <RoleMismatch
        expected={t("layout.nav_platform")}
        home={isCmsUser(user) ? routes.cms.home : routes.errors.forbidden}
      />
    );
  }

  if (mode === "cms" && user && !isCmsUser(user)) {
    return (
      <RoleMismatch
        expected="CMS"
        home={
          isPlatformUser(user) ? routes.platform.home : routes.errors.forbidden
        }
      />
    );
  }

  return <>{children}</>;
}
