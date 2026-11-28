"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { LogOut, UserRound } from "lucide-react";
import { toast } from "sonner";

import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { routes } from "@/config/routes";
import { isPlatformUser, loginPathFor, profilePathFor } from "@/lib/auth/types";
import { resetClickFix } from "@/lib/support/clickfix";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";

function initials(fullName?: string) {
  const parts = fullName?.trim().split(/\s+/).filter(Boolean) ?? [];
  const a = parts[0]?.[0] ?? "";
  const b = parts.length > 1 ? (parts[parts.length - 1]?.[0] ?? "") : "";
  return `${a}${b}`.toUpperCase() || "U";
}

export function UserMenu() {
  const { t } = useLocale();
  const { user, isAuthenticated, logout } = useAuth();
  const pathname = usePathname();

  if (!isAuthenticated) return null;

  // Profil kullanıcının bulunduğu kabukta açılır (#890 C): mükellef
  // kabuğunda /m/{uuid}/profil, diğerlerinde kendi kabuğu.
  const taxpayer = pathname?.match(/^\/m\/([^/]+)/)?.[1];
  const profileHref = isPlatformUser(user)
    ? routes.platform.profile.root
    : taxpayer
      ? routes.taxpayer.profile(taxpayer)
      : profilePathFor(user);

  const onLogout = async () => {
    // Destek widget'ı (#387) bu tarayıcıdaki ziyaretçiyi unutsun.
    resetClickFix();
    // Çıkıştan sonra kullanıcı bilgisi silinir: hedefi önce belirle (#890).
    const loginPath = loginPathFor(user);
    try {
      await logout();
    } finally {
      toast.success(t("auth.logout"));
      window.location.replace(loginPath);
    }
  };

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="icon" className="rounded-full">
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
      <DropdownMenuContent align="end" className="w-56">
        <DropdownMenuLabel className="font-normal">
          <div className="flex flex-col gap-0.5">
            <span className="truncate text-sm font-medium">
              {user?.fullName || t("auth.profile")}
            </span>
            {user?.email ? (
              <span className="text-muted-foreground truncate text-xs">
                {user.email}
              </span>
            ) : null}
          </div>
        </DropdownMenuLabel>
        <DropdownMenuSeparator />
        <DropdownMenuItem asChild>
          <Link href={profileHref}>
            <UserRound />
            {t("auth.profile")}
          </Link>
        </DropdownMenuItem>
        <DropdownMenuItem onClick={onLogout}>
          <LogOut />
          {t("auth.logout")}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
