"use client";

import {
  Building2,
  Check,
  CircleHelp,
  LogOut,
  Moon,
  Sun,
  UserRound,
} from "lucide-react";
import Link from "next/link";
import { useTheme } from "next-themes";
import { toast } from "sonner";

import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { resetClickFix } from "@/lib/support/clickfix";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";

function initials(fullName?: string) {
  const parts = fullName?.trim().split(/\s+/).filter(Boolean) ?? [];
  const a = parts[0]?.[0] ?? "";
  const b = parts.length > 1 ? (parts[parts.length - 1]?.[0] ?? "") : "";
  return `${a}${b}`.toUpperCase() || "U";
}

/**
 * Ofis kullanıcı menüsü (#521): ad, rol ve ofis; birden çok ofis üyeliği
 * varsa ofis değiştirme (düz gezinme: oturum değişmez); profil, yardım, tema
 * ve çıkış.
 */
export function OfficeUserMenu() {
  const { t } = useLocale();
  const { user, isAuthenticated, logout } = useAuth();
  const tenant = useTenant();
  const { resolvedTheme, setTheme } = useTheme();
  if (!isAuthenticated) return null;

  const orgs = user?.organizations ?? [];
  const dark = resolvedTheme === "dark";
  const onLogout = async () => {
    // Destek widget'ı (#387) bu tarayıcıdaki ziyaretçiyi unutsun.
    resetClickFix();
    try {
      await logout();
    } finally {
      toast.success(t("auth.logout"));
      window.location.replace(routes.guest.login);
    }
  };

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          size="icon"
          className="shrink-0 rounded-full pointer-coarse:size-11"
        >
          {/* Ad = ekran okuyucu metni + görünen baş harfler (WCAG 2.5.3). */}
          <span className="sr-only">
            {user?.fullName
              ? `${t("auth.profile")}: ${user.fullName}`
              : t("auth.profile")}
          </span>
          <Avatar className="size-8">
            <AvatarFallback className="text-xs">
              {initials(user?.fullName)}
            </AvatarFallback>
          </Avatar>
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-64">
        <DropdownMenuLabel className="font-normal">
          <div className="flex flex-col gap-0.5">
            <span className="truncate text-sm font-medium">
              {user?.fullName || t("auth.profile")}
            </span>
            <span className="text-muted-foreground truncate text-xs">
              {tenant.role.name} · {tenant.organization.name}
            </span>
          </div>
        </DropdownMenuLabel>
        {orgs.length > 1 ? (
          <>
            <DropdownMenuSeparator />
            <DropdownMenuGroup>
              <DropdownMenuLabel className="text-muted-foreground text-xs font-normal">
                {t("office.shell.switch_office")}
              </DropdownMenuLabel>
              {orgs.map((o) => (
                <DropdownMenuItem key={o.slug} asChild>
                  <Link href={routes.tenant.home(o.slug)}>
                    <Building2 />
                    <span className="min-w-0 flex-1 truncate">{o.name}</span>
                    {o.slug === tenant.organization.slug ? (
                      <Check aria-hidden className="size-4" />
                    ) : null}
                  </Link>
                </DropdownMenuItem>
              ))}
            </DropdownMenuGroup>
          </>
        ) : null}
        <DropdownMenuSeparator />
        <DropdownMenuItem asChild>
          <Link href={routes.cms.profile.root}>
            <UserRound />
            {t("auth.profile")}
          </Link>
        </DropdownMenuItem>
        <DropdownMenuItem asChild>
          <Link href={routes.help.index()}>
            <CircleHelp />
            {t("office.shell.help")}
          </Link>
        </DropdownMenuItem>
        <DropdownMenuItem
          onSelect={(e) => {
            // Menü açık kalsın: tema değişimi hemen görünür.
            e.preventDefault();
            setTheme(dark ? "light" : "dark");
          }}
        >
          {dark ? <Sun /> : <Moon />}
          {dark ? t("office.shell.theme_light") : t("office.shell.theme_dark")}
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem onClick={onLogout}>
          <LogOut />
          {t("auth.logout")}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
