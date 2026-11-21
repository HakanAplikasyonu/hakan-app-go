"use client";

import Link from "next/link";
import { useMemo, useState } from "react";

import { EntitySectionCard } from "@/components/entity";
import { DateCell } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import {
  NativeSelect,
  NativeSelectOption,
} from "@/components/ui/native-select";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

import {
  useCommunicationFilters,
  useRecentBulkSends,
  useTaxpayerGroups,
} from "../hooks/use-communication";
import type { BulkSendTarget } from "../services/communication.service";
import { BulkComposer } from "./bulk-composer";

const MODES = ["group", "filter", "recent", "list"] as const;
type Mode = (typeof MODES)[number];

/**
 * "Toplu gönderim" sekmesi: önce hedef (grup, süzgeç, son gönderilenler;
 * tek tek seçmek için mükellef listesi), sonra metin.
 */
export function BulkSendTab({ preset }: { preset: BulkSendTarget | null }) {
  const { t } = useLocale();
  const { slug, permissions: perms } = useTenant();
  const canSend = perms.includes(permissions.tenant.engagements.write);
  const groups = useTaxpayerGroups();
  const filters = useCommunicationFilters();
  const recent = useRecentBulkSends();
  const [mode, setMode] = useState<Mode>("group");
  const [groupUuid, setGroupUuid] = useState(preset?.group_uuid ?? "");
  const [taxKind, setTaxKind] = useState<"" | "kdv1" | "muhsgk">("");
  const [assignee, setAssignee] = useState("");
  const [recentUuid, setRecentUuid] = useState("");

  const target = useMemo<BulkSendTarget | null>(() => {
    switch (mode) {
      case "group":
        return groupUuid ? { kind: "group", group_uuid: groupUuid } : null;
      case "filter": {
        const parts: string[] = [];
        const kind = filters.data?.tax_kinds.find((k) => k.code === taxKind);
        if (kind) parts.push(kind.label);
        const person = filters.data?.assignees.find((a) => a.uuid === assignee);
        if (person)
          parts.push(
            t("communication.filter_assignee_label", { name: person.name }),
          );
        return {
          kind: "filter",
          ...(taxKind ? { tax_kind: taxKind } : {}),
          ...(assignee ? { assignee_uuid: assignee } : {}),
          label: parts.length
            ? parts.join(" · ")
            : t("communication.filter_all_label"),
        };
      }
      case "recent":
        return recentUuid ? { kind: "recent", bulk_uuid: recentUuid } : null;
      default:
        return null;
    }
  }, [mode, groupUuid, taxKind, assignee, recentUuid, filters.data, t]);

  return (
    <div className="grid min-w-0 gap-4 lg:grid-cols-[minmax(0,2fr)_minmax(0,1fr)]">
      <EntitySectionCard title={t("communication.bulk_title")}>
        <p className="text-muted-foreground mb-4 text-sm">
          {t("communication.bulk_description")}
        </p>
        <div className="mb-4 flex flex-col gap-2">
          <span className="text-sm font-medium">
            {t("communication.target")}
          </span>
          <div
            className="flex flex-wrap gap-1.5"
            role="group"
            aria-label={t("communication.target")}
          >
            {MODES.map((m) => (
              <Button
                key={m}
                size="sm"
                variant={mode === m ? "default" : "outline"}
                aria-pressed={mode === m}
                onClick={() => setMode(m)}
                data-testid={`target-mode-${m}`}
              >
                {t(`communication.target_${m}`)}
              </Button>
            ))}
          </div>

          {mode === "group" ? (
            groups.data && groups.data.items.length === 0 ? (
              <p className="text-muted-foreground text-sm">
                {t("communication.no_groups_yet")}
              </p>
            ) : (
              <div className="flex flex-col gap-1.5">
                <Label htmlFor="target-group">
                  {t("communication.pick_group")}
                </Label>
                <NativeSelect
                  id="target-group"
                  wrapperClassName="w-full sm:w-96"
                  value={groupUuid}
                  onChange={(e) => setGroupUuid(e.target.value)}
                >
                  <NativeSelectOption value="">
                    {t("communication.pick_group_placeholder")}
                  </NativeSelectOption>
                  {(groups.data?.items ?? []).map((g) => (
                    <NativeSelectOption key={g.uuid} value={g.uuid}>
                      {t("communication.group_option", {
                        name: g.name,
                        count: g.member_count,
                      })}
                    </NativeSelectOption>
                  ))}
                </NativeSelect>
              </div>
            )
          ) : null}

          {mode === "filter" ? (
            <div className="flex flex-col gap-3 sm:flex-row">
              <div className="flex flex-col gap-1.5">
                <Label htmlFor="target-tax-kind">
                  {t("communication.filter_tax_kind")}
                </Label>
                <NativeSelect
                  id="target-tax-kind"
                  wrapperClassName="w-full sm:w-80"
                  value={taxKind}
                  onChange={(e) =>
                    setTaxKind(e.target.value as "" | "kdv1" | "muhsgk")
                  }
                >
                  <NativeSelectOption value="">
                    {t("communication.filter_any")}
                  </NativeSelectOption>
                  {(filters.data?.tax_kinds ?? []).map((k) => (
                    <NativeSelectOption key={k.code} value={k.code}>
                      {t("communication.option_count", {
                        label: k.label,
                        count: k.count,
                      })}
                    </NativeSelectOption>
                  ))}
                </NativeSelect>
              </div>
              <div className="flex flex-col gap-1.5">
                <Label htmlFor="target-assignee">
                  {t("communication.filter_assignee")}
                </Label>
                <NativeSelect
                  id="target-assignee"
                  wrapperClassName="w-full sm:w-64"
                  value={assignee}
                  onChange={(e) => setAssignee(e.target.value)}
                >
                  <NativeSelectOption value="">
                    {t("communication.filter_any")}
                  </NativeSelectOption>
                  {(filters.data?.assignees ?? []).map((a) => (
                    <NativeSelectOption key={a.uuid} value={a.uuid}>
                      {t("communication.option_count", {
                        label: a.name,
                        count: a.count,
                      })}
                    </NativeSelectOption>
                  ))}
                </NativeSelect>
              </div>
            </div>
          ) : null}
          {mode === "filter" ? (
            <p className="text-muted-foreground text-xs">
              {t("communication.filter_hint")}
            </p>
          ) : null}

          {mode === "recent" ? (
            (recent.data?.items ?? []).length === 0 ? (
              <p className="text-muted-foreground text-sm">
                {t("communication.no_recent")}
              </p>
            ) : (
              <div className="flex flex-col gap-1.5">
                <Label htmlFor="target-recent">
                  {t("communication.pick_recent")}
                </Label>
                <NativeSelect
                  id="target-recent"
                  wrapperClassName="w-full sm:w-96"
                  value={recentUuid}
                  onChange={(e) => setRecentUuid(e.target.value)}
                >
                  <NativeSelectOption value="">
                    {t("communication.pick_recent_placeholder")}
                  </NativeSelectOption>
                  {(recent.data?.items ?? []).map((b) => (
                    <NativeSelectOption key={b.uuid} value={b.uuid}>
                      {t("communication.recent_option", {
                        label: b.target_label,
                        count: b.total,
                        date: new Date(b.created_at).toLocaleDateString(
                          "tr-TR",
                        ),
                      })}
                    </NativeSelectOption>
                  ))}
                </NativeSelect>
              </div>
            )
          ) : null}

          {mode === "list" ? (
            <p className="text-sm">
              {t("communication.list_hint")}{" "}
              <Link
                href={routes.tenant.taxpayers(slug)}
                className="text-brand-text font-medium hover:underline"
              >
                {t("communication.list_link")}
              </Link>
            </p>
          ) : null}
        </div>

        {canSend ? (
          <BulkComposer target={target} />
        ) : (
          <p className="text-muted-foreground text-sm">
            {t("communication.no_permission")}
          </p>
        )}
      </EntitySectionCard>

      <EntitySectionCard title={t("communication.recent_title")}>
        {(recent.data?.items ?? []).length === 0 ? (
          <p className="text-muted-foreground text-sm">
            {t("communication.no_recent")}
          </p>
        ) : (
          <ul className="flex flex-col gap-3" data-testid="recent-bulk-sends">
            {(recent.data?.items ?? []).map((b) => (
              <li key={b.uuid} className="flex flex-col gap-0.5 text-sm">
                <span className="font-medium">{b.target_label}</span>
                <span className="text-muted-foreground text-xs">
                  <DateCell value={b.created_at} /> ·{" "}
                  {b.channel === "sms"
                    ? t("communication.channel_sms")
                    : t("communication.channel_mail")}{" "}
                  ·{" "}
                  {t("communication.progress_counts", {
                    processed: b.processed,
                    total: b.total,
                    sent: b.sent,
                    failed: b.failed,
                  })}
                </span>
                {canSend ? (
                  <Button
                    variant="link"
                    size="sm"
                    className="h-auto self-start p-0"
                    onClick={() => {
                      setMode("recent");
                      setRecentUuid(b.uuid);
                    }}
                  >
                    {t("communication.send_again")}
                  </Button>
                ) : null}
              </li>
            ))}
          </ul>
        )}
      </EntitySectionCard>
    </div>
  );
}
