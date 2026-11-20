"use client";

import { FileText } from "lucide-react";
import { useId, useState } from "react";

import { StatusChip } from "@/components/common/status-chip";
import { EntitySectionCard } from "@/components/entity";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { permissions } from "@/config/permissions";
import { LegalDocumentDialog } from "@/features/legal/components/legal-document-dialog";
import { LegalDraftNotice } from "@/features/legal/components/legal-draft-notice";
import {
  useAcceptOfficeDpa,
  useOfficeDpa,
} from "@/features/legal/hooks/use-legal";
import { isLegalVersionOutdated } from "@/features/legal/services/legal.service";
import { date } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

/**
 * "Veri İşleyen Sözleşmesi" on the office settings (issue #57, KVKK md.
 * 12/2): status of the agreement between the office (data controller) and
 * the platform (data processor). Owners/admins (`tenant.legal.accept`)
 * accept the current version on behalf of the office; everyone else who
 * can read the settings sees the status only.
 */
export function OfficeDpaCard({ slug }: { slug: string }) {
  const { t, locale } = useLocale();
  const { can } = usePermission();
  const canAccept = can(permissions.tenant.legal.accept);
  const query = useOfficeDpa(slug);
  const accept = useAcceptOfficeDpa(slug);
  const checkboxId = useId();
  const [open, setOpen] = useState(false);
  const [agreed, setAgreed] = useState(false);
  const [outdated, setOutdated] = useState(false);

  const title = t("legal.dpa_title");

  if (query.isLoading) {
    return (
      <EntitySectionCard title={title}>
        <p className="text-muted-foreground text-sm">{t("common.loading")}</p>
      </EntitySectionCard>
    );
  }
  if (query.data === null) {
    return (
      <EntitySectionCard title={title}>
        <p className="text-muted-foreground text-sm">
          {t("legal.dpa_not_published")}
        </p>
      </EntitySectionCard>
    );
  }
  if (!query.data) {
    return (
      <EntitySectionCard title={title}>
        <div className="flex flex-wrap items-center gap-2 text-sm">
          <span className="text-destructive">{t("legal.load_error")}</span>
          <Button
            size="sm"
            variant="outline"
            onClick={() => void query.refetch()}
          >
            {t("common.retry")}
          </Button>
        </div>
      </EntitySectionCard>
    );
  }

  const { document, accepted, up_to_date: upToDate } = query.data;
  const acceptedLine = accepted
    ? t("legal.dpa_accepted_by", {
        version: accepted.version,
        user: accepted.accepted_by,
        date: date(accepted.accepted_at, "dd.MM.yyyy", locale),
      })
    : null;

  const onAccept = () => {
    accept.mutate(
      { version: document.version, language: document.language },
      {
        onSuccess: () => {
          setAgreed(false);
          setOutdated(false);
        },
        onError: (error) => {
          if (isLegalVersionOutdated(error)) {
            setAgreed(false);
            setOutdated(true);
            void query.refetch();
          }
        },
      },
    );
  };

  return (
    <EntitySectionCard title={title}>
      <div className="space-y-4">
        <p className="text-muted-foreground text-sm">
          {t("legal.dpa_description")}
        </p>

        <div className="flex flex-wrap items-center gap-2">
          {!accepted ? (
            <StatusChip label={t("legal.dpa_status_none")} tone="warning" />
          ) : upToDate ? (
            <StatusChip
              label={t("legal.dpa_status_accepted", {
                version: accepted.version,
              })}
              tone="success"
            />
          ) : (
            <StatusChip
              label={t("legal.dpa_status_new_version", {
                version: document.version,
              })}
              tone="warning"
            />
          )}
          {acceptedLine ? (
            <span className="text-muted-foreground text-sm">
              {acceptedLine}
            </span>
          ) : null}
        </div>

        {document.legal_review_pending ? <LegalDraftNotice /> : null}

        <div className="flex flex-wrap items-center gap-2">
          <Button type="button" variant="outline" onClick={() => setOpen(true)}>
            <FileText className="size-4" aria-hidden />
            {t("legal.dpa_read", { version: document.version })}
          </Button>
        </div>

        {canAccept && !upToDate ? (
          <div className="border-border space-y-3 border-t pt-4">
            <div className="flex items-start gap-2">
              <Checkbox
                id={checkboxId}
                className="mt-0.5"
                checked={agreed}
                disabled={accept.isPending}
                onCheckedChange={(value) => {
                  setAgreed(value === true);
                  if (value === true) setOutdated(false);
                }}
              />
              <Label htmlFor={checkboxId} className="leading-snug font-normal">
                {t("legal.dpa_checkbox", { version: document.version })}
              </Label>
            </div>
            {outdated ? (
              <p
                role="alert"
                className="text-sm text-amber-700 dark:text-amber-300"
              >
                {t("legal.outdated_message")}
              </p>
            ) : null}
            <Button
              type="button"
              disabled={!agreed || accept.isPending}
              onClick={onAccept}
            >
              {t("legal.dpa_accept")}
            </Button>
          </div>
        ) : !canAccept && !upToDate ? (
          <p className="text-muted-foreground text-sm">
            {t("legal.dpa_ask_admin")}
          </p>
        ) : null}
      </div>

      <LegalDocumentDialog
        open={open}
        onOpenChange={setOpen}
        kind="data_processing_agreement"
        document={document}
        acceptLabel={t("legal.read_and_accept")}
        onAccept={
          canAccept && !upToDate && !accept.isPending
            ? () => {
                setAgreed(true);
                setOutdated(false);
              }
            : undefined
        }
      />
    </EntitySectionCard>
  );
}
