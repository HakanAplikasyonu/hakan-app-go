"use client";

import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { toast } from "sonner";

import { GuvenOlcegi } from "@/components/hakan";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { Skeleton } from "@/components/ui/skeleton";
import { Slider } from "@/components/ui/slider";
import { Switch } from "@/components/ui/switch";
import { permissions } from "@/config/permissions";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { useDebounce } from "@/hooks/use-debounce";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import {
  usePutReviewSetting,
  usePutRuleSettings,
  useRuleSettings,
  useRuleSettingsPreview,
  useThresholdImpact,
} from "../hooks/use-office-settings";
import type { AccountRuleSettingsInput } from "../services/office-settings.service";
import { reviewKeys, reviewService } from "../services/review.service";
import { Figure } from "./office-ui";

const pct = (n: number, d: number) =>
  d > 0
    ? new Intl.NumberFormat("tr-TR", { maximumFractionDigits: 1 }).format(
        (n / d) * 100,
      )
    : "0";

function Card({
  id,
  title,
  description,
  children,
}: {
  id: string;
  title: string;
  description?: string;
  children: React.ReactNode;
}) {
  return (
    <section
      aria-labelledby={`${id}-title`}
      className="bg-card rounded-[14px] border p-5 sm:p-6"
    >
      <h3 id={`${id}-title`} className="text-base font-semibold">
        {title}
      </h3>
      {description ? (
        <p className="text-text-2 mt-1 text-sm">{description}</p>
      ) : null}
      <div className="mt-5">{children}</div>
    </section>
  );
}

function errText(err: unknown, fallback: string) {
  return isApiError(err) ? err.message : fallback;
}

/** Otomatik onay eşiği ve son 30 günde ne olurdu (#333 threshold-impact). */
function ThresholdCard() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const canWrite = can(permissions.tenant.review.settingsWrite);
  const settings = useQuery({
    queryKey: reviewKeys.settings(slug),
    queryFn: () => reviewService.settings(slug),
  });
  const office = settings.data?.effective["*"] ?? settings.data?.default;
  const current = office ? Math.round(Number(office.threshold) * 100) : 95;
  const [draft, setDraft] = useState<number | null>(null);
  const [autoDraft, setAutoDraft] = useState<boolean | null>(null);
  const value = draft ?? current;
  const auto = autoDraft ?? office?.auto_accept ?? true;
  const debounced = useDebounce(value, 250);
  const impact = useThresholdImpact((debounced / 100).toFixed(2), 1, !!office);
  const put = usePutReviewSetting();
  const dirty = !!office && (value !== current || auto !== office.auto_accept);

  if (settings.isLoading) return <Skeleton className="h-56 w-full" />;
  const i = impact.data;
  return (
    <Card
      id="ai-threshold"
      title={t("office.settings.threshold_title")}
      description={t("office.settings.threshold_description")}
    >
      <div className="flex flex-col gap-5">
        <label className="flex min-h-11 items-center gap-3 text-sm">
          <Switch
            checked={auto}
            onCheckedChange={(v) => setAutoDraft(v)}
            disabled={!canWrite}
          />
          {t("office.settings.auto_accept")}
        </label>
        <div className="flex flex-wrap items-center gap-4">
          <GuvenOlcegi deger={value / 100} />
          <Figure className="font-display text-3xl font-bold">%{value}</Figure>
          <div className="min-w-56 flex-1">
            <Slider
              min={80}
              max={99}
              step={1}
              value={[value]}
              onValueChange={(v) => setDraft(v[0] ?? current)}
              disabled={!canWrite || !auto}
              aria-label={t("office.settings.threshold_label")}
              className="pointer-coarse:py-3"
            />
            <div className="text-text-2 mt-2 flex justify-between text-xs">
              <span>{t("office.settings.threshold_low")}</span>
              <span>{t("office.settings.threshold_high")}</span>
            </div>
          </div>
        </div>
        <p
          className="bg-surface-2 rounded-[10px] border p-3 text-sm"
          aria-live="polite"
        >
          {impact.isError ? (
            t("office.settings.impact_unavailable")
          ) : !i ? (
            t("common.loading")
          ) : (
            <>
              {t("office.settings.impact", {
                count: i.would_auto_accept,
                corrected: i.would_auto_accept_then_corrected,
                pct: pct(
                  i.would_auto_accept_then_corrected,
                  i.would_auto_accept,
                ),
              })}{" "}
              {t("office.settings.impact_queue", {
                count: i.would_go_to_queue,
                decided: i.decided,
              })}
            </>
          )}
        </p>
        {canWrite ? (
          <div className="flex justify-end gap-2">
            {dirty ? (
              <Button
                variant="ghost"
                className="h-11"
                onClick={() => {
                  setDraft(null);
                  setAutoDraft(null);
                }}
              >
                {t("common.cancel")}
              </Button>
            ) : null}
            <Button
              className="bg-brand text-on-brand hover:bg-brand/90 h-11"
              disabled={!dirty || put.isPending}
              onClick={() =>
                put.mutate(
                  {
                    kind: "*",
                    auto_accept: auto,
                    threshold: (value / 100).toFixed(2),
                  },
                  {
                    onSuccess: () => {
                      setDraft(null);
                      setAutoDraft(null);
                      toast.success(t("office.settings.saved"));
                    },
                    onError: (e) =>
                      toast.error(errText(e, t("office.settings.save_failed"))),
                  },
                )
              }
            >
              {t("office.settings.save")}
            </Button>
          </div>
        ) : (
          <p className="text-text-2 text-xs">
            {t("office.settings.read_only")}
          </p>
        )}
      </div>
    </Card>
  );
}

/** Kural adayı eşiği (#333): kaç tutarlı karardan sonra aday olur. */
function RuleCandidateCard() {
  const { t } = useLocale();
  const { can } = usePermission();
  const canWrite = can(permissions.tenant.settings.write);
  const settings = useRuleSettings();
  const s = settings.data;
  const [draft, setDraft] = useState<AccountRuleSettingsInput | null>(null);
  const value: AccountRuleSettingsInput = draft ?? {
    rule_candidate_min_count: s?.rule_candidate_min_count ?? 3,
    rule_candidate_decisions:
      s?.rule_candidate_decisions ?? "corrections_and_accepts",
  };
  // Nesne değil anahtar geciktirilir (her çizimde yeni nesne döngü yapar).
  const key = useDebounce(JSON.stringify(value), 250);
  const preview = useRuleSettingsPreview(
    JSON.parse(key) as AccountRuleSettingsInput,
    !!s,
  );
  const put = usePutRuleSettings();
  const dirty =
    !!s &&
    (value.rule_candidate_min_count !== s.rule_candidate_min_count ||
      value.rule_candidate_decisions !== s.rule_candidate_decisions);

  if (settings.isLoading) return <Skeleton className="h-56 w-full" />;
  const p = preview.data;
  return (
    <Card
      id="ai-rule-candidates"
      title={t("office.settings.candidates_title")}
      description={t("office.settings.candidates_description")}
    >
      <div className="flex flex-col gap-5">
        <div className="flex flex-wrap items-center gap-4">
          <Figure className="font-display text-3xl font-bold">
            {value.rule_candidate_min_count}×
          </Figure>
          <div className="min-w-56 flex-1">
            <Slider
              min={2}
              max={10}
              step={1}
              value={[value.rule_candidate_min_count ?? 3]}
              onValueChange={(v) =>
                setDraft({ ...value, rule_candidate_min_count: v[0] ?? 3 })
              }
              disabled={!canWrite}
              aria-label={t("office.settings.candidates_count_label")}
              className="pointer-coarse:py-3"
            />
            <div className="text-text-2 mt-2 flex justify-between text-xs">
              <span>{t("office.settings.candidates_low")}</span>
              <span>{t("office.settings.candidates_high")}</span>
            </div>
          </div>
        </div>
        <RadioGroup
          value={value.rule_candidate_decisions}
          onValueChange={(v) =>
            setDraft({
              ...value,
              rule_candidate_decisions:
                v as AccountRuleSettingsInput["rule_candidate_decisions"],
            })
          }
          disabled={!canWrite}
          aria-label={t("office.settings.candidates_decisions_label")}
          className="gap-1"
        >
          {(["corrections_and_accepts", "corrections_only"] as const).map(
            (k) => (
              <Label
                key={k}
                className="flex min-h-11 items-center gap-3 font-normal"
              >
                <RadioGroupItem value={k} />
                {t(
                  k === "corrections_only"
                    ? "office.settings.decisions_corrections_only"
                    : "office.settings.decisions_corrections_and_accepts",
                )}
              </Label>
            ),
          )}
        </RadioGroup>
        <p
          className="bg-surface-2 rounded-[10px] border p-3 text-sm"
          aria-live="polite"
        >
          {preview.isError
            ? t("office.settings.impact_unavailable")
            : p
              ? t("office.settings.candidates_preview", {
                  now: p.current.candidates,
                  nowEng: p.current.engagements,
                  next: p.proposed.candidates,
                  nextEng: p.proposed.engagements,
                })
              : t("common.loading")}
        </p>
        {canWrite ? (
          <div className="flex justify-end gap-2">
            {dirty ? (
              <Button
                variant="ghost"
                className="h-11"
                onClick={() => setDraft(null)}
              >
                {t("common.cancel")}
              </Button>
            ) : null}
            <Button
              className="bg-brand text-on-brand hover:bg-brand/90 h-11"
              disabled={!dirty || put.isPending}
              onClick={() =>
                put.mutate(value, {
                  onSuccess: () => {
                    setDraft(null);
                    toast.success(t("office.settings.saved"));
                  },
                  onError: (e) =>
                    toast.error(errText(e, t("office.settings.save_failed"))),
                })
              }
            >
              {t("office.settings.save")}
            </Button>
          </div>
        ) : (
          <p className="text-text-2 text-xs">
            {t("office.settings.read_only_owner")}
          </p>
        )}
      </div>
    </Card>
  );
}

/** Ayarlar → Yapay zekâ ve onay (OfisAyarlar). */
export function AiApprovalSettings() {
  const { can } = usePermission();
  return (
    <div className="flex flex-col gap-4">
      {can(permissions.tenant.review.read) ? <ThresholdCard /> : null}
      {can(permissions.tenant.accountRules.read) ? <RuleCandidateCard /> : null}
    </div>
  );
}
