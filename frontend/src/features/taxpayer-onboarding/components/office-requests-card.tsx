"use client";

import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  useApproveOnboardingRequest,
  usePendingOnboardingRequests,
  useRejectOnboardingRequest,
} from "@/features/taxpayer-onboarding/hooks/use-taxpayer-onboarding";
import type { TaxpayerPendingRequest } from "@/features/taxpayer-onboarding/services/taxpayer-onboarding.service";
import { date } from "@/lib/utils/format";
import { useDialogs } from "@/providers/dialog-provider";
import { useLocale } from "@/providers/locale-provider";

/**
 * "Ofis talepleri" on the mükellef home: offices asking to add this company.
 * Only owners can read the list; for anyone else the call is refused and the
 * card stays hidden.
 */
export function OfficeRequestsCard({ taxpayer }: { taxpayer: string }) {
  const { t, locale } = useLocale();
  const { confirm } = useDialogs();
  const query = usePendingOnboardingRequests(taxpayer);
  const approve = useApproveOnboardingRequest(taxpayer);
  const reject = useRejectOnboardingRequest(taxpayer);
  const busy = approve.isPending || reject.isPending;

  if (!query.isSuccess) return null;
  const items = query.data.items;

  const onApprove = async (req: TaxpayerPendingRequest) => {
    const ok = await confirm({
      title: t("onboarding.panel_approve_title", {
        office: req.organization.name,
      }),
      description: t("onboarding.panel_approve_description"),
      confirmLabel: t("onboarding.consent_approve"),
    });
    if (ok) approve.mutate(req.uuid);
  };

  const onReject = async (req: TaxpayerPendingRequest) => {
    const ok = await confirm({
      title: t("onboarding.panel_reject_title", {
        office: req.organization.name,
      }),
      description: t("onboarding.consent_reject_description"),
      confirmLabel: t("onboarding.consent_reject"),
    });
    if (ok) reject.mutate(req.uuid);
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("onboarding.panel_title")}</CardTitle>
        <CardDescription>{t("onboarding.panel_description")}</CardDescription>
      </CardHeader>
      <CardContent>
        {items.length === 0 ? (
          <p className="text-muted-foreground text-sm">
            {t("onboarding.panel_empty")}
          </p>
        ) : (
          <ul className="divide-y">
            {items.map((req) => (
              <li
                key={req.uuid}
                className="flex flex-col gap-2 py-3 first:pt-0 last:pb-0 sm:flex-row sm:items-center sm:justify-between"
              >
                <div className="text-sm">
                  <div className="font-medium">{req.organization.name}</div>
                  <div className="text-muted-foreground text-xs">
                    {req.title}
                    {" · "}
                    <span className="font-mono">{req.tax_id}</span>
                    {" · "}
                    {t("onboarding.panel_expires", {
                      date: date(req.expires_at, "dd.MM.yyyy", locale),
                    })}
                  </div>
                </div>
                <div className="flex gap-2">
                  <Button
                    type="button"
                    size="sm"
                    disabled={busy}
                    onClick={() => void onApprove(req)}
                  >
                    {t("onboarding.consent_approve")}
                  </Button>
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    disabled={busy}
                    onClick={() => void onReject(req)}
                  >
                    {t("onboarding.consent_reject")}
                  </Button>
                </div>
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  );
}
