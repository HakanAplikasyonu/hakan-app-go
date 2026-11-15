"use client";

import { useQuery } from "@tanstack/react-query";
import Link from "next/link";
import { useRouter } from "next/navigation";

import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { routes } from "@/config/routes";
import { taxpayerInvitationService } from "@/features/taxpayer/services/taxpayer.service";
import { ApiError } from "@/lib/api/errors";
import { useAppMutation } from "@/lib/query/mutation";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

const STATUS_KEYS: Record<string, string> = {
  expired: "taxpayer.accept_status_expired",
  accepted: "taxpayer.accept_status_accepted",
  revoked: "taxpayer.accept_status_revoked",
};

function Shell({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex min-h-svh items-center justify-center p-6">
      <Card className="w-full max-w-md">{children}</Card>
    </div>
  );
}

/**
 * `/invite/m/{token}` — a mükellef invitation sent by an office (D15). Works
 * signed out: sign in, or create the invited account (the token opens
 * registration for that address), then come back via ?next=.
 */
export function TaxpayerInviteAcceptPage({ token }: { token: string }) {
  const { t } = useLocale();
  const router = useRouter();
  const { user, isAuthenticated, bootstrapped, logout, hydrateProfile } =
    useAuth();

  const preview = useQuery({
    queryKey: ["taxpayer-invitation", "preview", token],
    queryFn: () => taxpayerInvitationService.preview(token),
    retry: false,
    staleTime: 30_000,
    // The card explains failures; no global toast on top.
    meta: { silent: true },
  });

  const accept = useAppMutation({
    mutationFn: () => taxpayerInvitationService.accept(token),
  });

  const onAccept = async () => {
    let accepted;
    try {
      accepted = await accept.mutateAsync();
    } catch {
      void preview.refetch(); // shown as a toast; refresh the status
      return;
    }
    await hydrateProfile(); // the new membership appears in /me
    appToast.success(
      t("taxpayer.accept_done", { name: preview.data?.taxpayer.title ?? "" }),
    );
    router.replace(routes.taxpayer.home(accepted.taxpayer_uuid));
  };

  if (preview.isLoading || (!bootstrapped && !user)) {
    return (
      <div className="text-muted-foreground flex min-h-svh items-center justify-center text-sm">
        {t("common.loading")}
      </div>
    );
  }

  if (preview.isError || !preview.data) {
    const notFound =
      preview.error instanceof ApiError &&
      (preview.error.isNotFound || preview.error.isValidation);
    return (
      <Shell>
        <CardHeader>
          <CardTitle>
            {notFound
              ? t("taxpayer.accept_not_found_title")
              : t("taxpayer.gate_error_title")}
          </CardTitle>
          <CardDescription>
            {notFound
              ? t("taxpayer.accept_not_found_description")
              : t("taxpayer.gate_error_description")}
          </CardDescription>
        </CardHeader>
        {!notFound ? (
          <CardContent>
            <Button onClick={() => void preview.refetch()}>
              {t("taxpayer.gate_retry")}
            </Button>
          </CardContent>
        ) : null}
      </Shell>
    );
  }

  const inv = preview.data;
  const role =
    inv.role === "owner"
      ? t("taxpayer.role_owner")
      : t("taxpayer.role_employee");

  if (inv.status !== "pending") {
    return (
      <Shell>
        <CardHeader>
          <CardTitle>{t("taxpayer.accept_unavailable_title")}</CardTitle>
          <CardDescription>
            {t(STATUS_KEYS[inv.status] ?? "taxpayer.accept_status_expired")}
          </CardDescription>
        </CardHeader>
      </Shell>
    );
  }

  const office = inv.organization.name;
  const description = inv.invited_by
    ? t("taxpayer.accept_description", {
        inviter: inv.invited_by,
        office,
        email: inv.email,
        role,
      })
    : t("taxpayer.accept_description_no_inviter", {
        office,
        email: inv.email,
        role,
      });
  const here = routes.inviteTaxpayer(token);
  const sameAccount =
    user && user.email.trim().toLowerCase() === inv.email.toLowerCase();

  return (
    <Shell>
      <CardHeader>
        <CardTitle>
          {t("taxpayer.accept_title", { name: inv.taxpayer.title })}
        </CardTitle>
        <CardDescription>{description}</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        {!isAuthenticated || !user ? (
          <>
            <Button asChild>
              <Link
                href={`${routes.guest.login}?next=${encodeURIComponent(here)}`}
              >
                {t("taxpayer.accept_sign_in")}
              </Link>
            </Button>
            <Button asChild variant="outline">
              <Link
                href={`${routes.guest.register}?invitation=${encodeURIComponent(token)}&next=${encodeURIComponent(here)}`}
              >
                {t("taxpayer.accept_register")}
              </Link>
            </Button>
          </>
        ) : sameAccount ? (
          <Button disabled={accept.isPending} onClick={() => void onAccept()}>
            {t("taxpayer.accept_submit")}
          </Button>
        ) : (
          <>
            <p className="text-muted-foreground text-sm">
              {t("taxpayer.accept_wrong_account", {
                current: user.email,
                email: inv.email,
              })}
            </p>
            <Button
              variant="outline"
              onClick={async () => {
                await logout();
                window.location.replace(
                  `${routes.guest.login}?next=${encodeURIComponent(here)}`,
                );
              }}
            >
              {t("taxpayer.accept_sign_out")}
            </Button>
          </>
        )}
      </CardContent>
    </Shell>
  );
}
