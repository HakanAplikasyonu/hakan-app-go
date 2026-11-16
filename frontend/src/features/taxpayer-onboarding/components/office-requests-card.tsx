"use client";

import { ShieldCheck } from "lucide-react";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  isLegalVersionOutdated,
  LegalConsentCheckbox,
  useLegalConsent,
} from "@/features/legal";
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
 * card stays hidden. Approving asks the owner to acknowledge the privacy
 * notice (issue #57) and sends its version; rejecting does not.
 */
export function OfficeRequestsCard({ taxpayer }: { taxpayer: string }) {
  const { t, locale } = useLocale();
  const { confirm } = useDialogs();
  const query = usePendingOnboardingRequests(taxpayer);
  const approve = useApproveOnboardingRequest(taxpayer);
  const reject = useRejectOnboardingRequest(taxpayer);
  const busy = approve.isPending || reject.isPending;
  const notice = useLegalConsent(
    "privacy_notice",
    query.isSuccess && query.data.items.length > 0,
  );
  const [approving, setApproving] = useState<TaxpayerPendingRequest | null>(
    null,
  );

  if (!query.isSuccess) return null;
  const items = query.data.items;

  const closeApprove = () => {
    if (approve.isPending) return;
    setApproving(null);
    notice.reset();
  };

  const onApprove = () => {
    const accepted = notice.acceptance;
    if (!approving || !accepted) return;
    approve.mutate(
      {
        uuid: approving.uuid,
        notice: {
          privacy_notice_version: accepted.version,
          privacy_notice_language: accepted.language,
        },
      },
      {
        onSuccess: () => {
          setApproving(null);
          notice.reset();
        },
        onError: (error) => {
          // The notice changed: keep the dialog open and ask again.
          if (isLegalVersionOutdated(error)) notice.markOutdated();
          else setApproving(null);
        },
      },
    );
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

  // Bekleyen talep yoksa bant hiç görünmez (arayüz 8/9, #331).
  if (items.length === 0) return null;
  return (
    <section
      aria-labelledby="office-requests-title"
      className="border-brand bg-brand-tint flex flex-col gap-4 rounded-xl border p-4 sm:flex-row sm:items-start md:p-5"
    >
      <ShieldCheck
        aria-hidden
        className="text-brand-text size-6 shrink-0"
        strokeWidth={1.8}
      />
      <div className="min-w-0 flex-1">
        <h2
          id="office-requests-title"
          className="font-display text-base font-bold"
        >
          {t("onboarding.panel_title")}
        </h2>
        <p className="text-text-2 mt-1 text-sm">
          {t("onboarding.panel_description")}
        </p>
        <ul className="divide-line mt-3 divide-y">
          {items.map((req) => (
            <li
              key={req.uuid}
              className="flex flex-col gap-2 py-3 first:pt-0 last:pb-0 sm:flex-row sm:items-center sm:justify-between"
            >
              <div className="text-sm">
                <div className="font-semibold">{req.organization.name}</div>
                <div className="text-text-2 text-xs">
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
                  variant="outline"
                  className="min-h-11 md:min-h-9"
                  disabled={busy}
                  onClick={() => void onReject(req)}
                >
                  {t("onboarding.consent_reject")}
                </Button>
                <Button
                  type="button"
                  className="min-h-11 md:min-h-9"
                  disabled={busy}
                  onClick={() => setApproving(req)}
                >
                  {t("onboarding.consent_approve")}
                </Button>
              </div>
            </li>
          ))}
        </ul>
      </div>
      <Dialog
        open={approving !== null}
        onOpenChange={(open) => {
          if (!open) closeApprove();
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {t("onboarding.panel_approve_title", {
                office: approving?.organization.name ?? "",
              })}
            </DialogTitle>
            <DialogDescription>
              {t("onboarding.panel_approve_description")}
            </DialogDescription>
          </DialogHeader>
          <LegalConsentCheckbox
            consent={notice}
            label={t("onboarding.privacy_notice_label")}
            acceptLabel={t("onboarding.privacy_notice_read")}
            disabled={approve.isPending}
          />
          <DialogFooter className="gap-2">
            <Button
              variant="outline"
              onClick={closeApprove}
              disabled={approve.isPending}
            >
              {t("common.cancel")}
            </Button>
            <Button
              onClick={onApprove}
              disabled={approve.isPending || !notice.acceptance}
            >
              {t("onboarding.consent_approve")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </section>
  );
}
