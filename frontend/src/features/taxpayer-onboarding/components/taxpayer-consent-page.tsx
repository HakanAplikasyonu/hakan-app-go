"use client";

import { useQuery } from "@tanstack/react-query";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { routes } from "@/config/routes";
import { KIND_LABEL_KEYS } from "@/features/taxpayer-onboarding/lib/status";
import {
  onboardingKeys,
  taxpayerConsentService,
} from "@/features/taxpayer-onboarding/services/taxpayer-onboarding.service";
import { ApiError } from "@/lib/api/errors";
import { useAppMutation } from "@/lib/query/mutation";
import { date } from "@/lib/utils/format";
import { useAuth } from "@/providers/auth-provider";
import { useDialogs } from "@/providers/dialog-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

const STATUS_KEYS: Record<string, string> = {
  approved: "onboarding.consent_status_approved",
  rejected: "onboarding.consent_status_rejected",
  cancelled: "onboarding.consent_status_cancelled",
  expired: "onboarding.consent_status_expired",
};

function Shell({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex min-h-svh items-center justify-center p-6">
      <Card className="w-full max-w-md">{children}</Card>
    </div>
  );
}

function Message({
  title,
  description,
}: {
  title: string;
  description: string;
}) {
  return (
    <Shell>
      <CardHeader>
        <CardTitle>{title}</CardTitle>
        <CardDescription>{description}</CardDescription>
      </CardHeader>
    </Shell>
  );
}

type Outcome = "forwarded" | "approved" | "rejected";

/**
 * `/consent/{token}` — an office asks to add the mükellef (issue #7). Works
 * signed out: sign in, or create the account for the contact address (the
 * token opens registration for it), then come back via ?next=.
 */
export function TaxpayerConsentPage({ token }: { token: string }) {
  const { t, locale } = useLocale();
  const router = useRouter();
  const { confirm } = useDialogs();
  const { user, isAuthenticated, bootstrapped, logout, hydrateProfile } =
    useAuth();
  const [outcome, setOutcome] = useState<Outcome | null>(null);

  const preview = useQuery({
    queryKey: onboardingKeys.consentPreview(token),
    queryFn: () => taxpayerConsentService.preview(token),
    retry: false,
    staleTime: 30_000,
    // The card explains failures; no global toast on top.
    meta: { silent: true },
  });

  const approve = useAppMutation({
    mutationFn: () => taxpayerConsentService.approve(token),
  });
  const reject = useAppMutation({
    mutationFn: () => taxpayerConsentService.reject(token),
  });
  const busy = approve.isPending || reject.isPending;

  const onApprove = async () => {
    let result;
    try {
      result = await approve.mutateAsync();
    } catch {
      void preview.refetch(); // shown as a toast; refresh the status
      return;
    }
    if (result.status === "approved" && result.taxpayer_uuid) {
      await hydrateProfile(); // the new company appears in /me
      appToast.success(t("onboarding.consent_approved_toast"));
      router.replace(routes.taxpayer.home(result.taxpayer_uuid));
      return;
    }
    setOutcome(result.status === "forwarded" ? "forwarded" : "approved");
  };

  const onReject = async () => {
    const ok = await confirm({
      title: t("onboarding.consent_reject_title"),
      description: t("onboarding.consent_reject_description"),
      confirmLabel: t("onboarding.consent_reject"),
    });
    if (!ok) return;
    try {
      await reject.mutateAsync();
    } catch {
      void preview.refetch();
      return;
    }
    setOutcome("rejected");
  };

  if (outcome === "forwarded") {
    return (
      <Message
        title={t("onboarding.consent_forwarded_title")}
        description={t("onboarding.consent_forwarded_description")}
      />
    );
  }
  if (outcome === "approved") {
    return (
      <Message
        title={t("onboarding.consent_approved_title")}
        description={t("onboarding.consent_approved_description")}
      />
    );
  }
  if (outcome === "rejected") {
    return (
      <Message
        title={t("onboarding.consent_rejected_title")}
        description={t("onboarding.consent_rejected_description")}
      />
    );
  }

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
              ? t("onboarding.consent_not_found_title")
              : t("onboarding.consent_error_title")}
          </CardTitle>
          <CardDescription>
            {notFound
              ? t("onboarding.consent_not_found_description")
              : t("onboarding.consent_error_description")}
          </CardDescription>
        </CardHeader>
        {!notFound ? (
          <CardContent>
            <Button onClick={() => void preview.refetch()}>
              {t("common.retry")}
            </Button>
          </CardContent>
        ) : null}
      </Shell>
    );
  }

  const req = preview.data;
  if (req.status !== "pending") {
    return (
      <Message
        title={t("onboarding.consent_unavailable_title")}
        description={t(
          STATUS_KEYS[req.status] ?? "onboarding.consent_status_expired",
        )}
      />
    );
  }

  const here = routes.consent(token);
  const sameAccount =
    user &&
    user.email.trim().toLowerCase() === req.contact_email.trim().toLowerCase();

  return (
    <Shell>
      <CardHeader>
        <CardTitle>
          {t("onboarding.consent_title", { office: req.organization.name })}
        </CardTitle>
        <CardDescription>
          {t("onboarding.consent_description", {
            office: req.organization.name,
            name: req.title,
          })}
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm">
          <dt className="text-muted-foreground">
            {t("onboarding.consent_office")}
          </dt>
          <dd className="font-medium">{req.organization.name}</dd>
          <dt className="text-muted-foreground">
            {t("onboarding.consent_taxpayer")}
          </dt>
          <dd className="font-medium">{req.title}</dd>
          <dt className="text-muted-foreground">
            {t(KIND_LABEL_KEYS[req.kind])}
          </dt>
          <dd className="font-mono">{req.tax_id}</dd>
          <dt className="text-muted-foreground">
            {t("onboarding.consent_expires")}
          </dt>
          <dd>{date(req.expires_at, "dd.MM.yyyy", locale)}</dd>
        </dl>
        <p className="text-muted-foreground text-sm">
          {t("onboarding.consent_effect")}
        </p>
        {!isAuthenticated || !user ? (
          <>
            <p className="text-muted-foreground text-sm">
              {t("onboarding.consent_sign_in_hint", {
                email: req.contact_email,
              })}
            </p>
            <Button asChild>
              <Link
                href={`${routes.guest.login}?next=${encodeURIComponent(here)}`}
              >
                {t("onboarding.consent_sign_in")}
              </Link>
            </Button>
            <Button asChild variant="outline">
              <Link
                href={`${routes.guest.register}?invitation=${encodeURIComponent(token)}&next=${encodeURIComponent(here)}`}
              >
                {t("onboarding.consent_register")}
              </Link>
            </Button>
          </>
        ) : sameAccount ? (
          <div className="flex gap-2">
            <Button
              className="flex-1"
              disabled={busy}
              onClick={() => void onApprove()}
            >
              {t("onboarding.consent_approve")}
            </Button>
            <Button
              className="flex-1"
              variant="outline"
              disabled={busy}
              onClick={() => void onReject()}
            >
              {t("onboarding.consent_reject")}
            </Button>
          </div>
        ) : (
          <>
            <p className="text-muted-foreground text-sm">
              {t("onboarding.consent_wrong_account", {
                current: user.email,
                email: req.contact_email,
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
              {t("onboarding.consent_sign_out")}
            </Button>
          </>
        )}
      </CardContent>
    </Shell>
  );
}
