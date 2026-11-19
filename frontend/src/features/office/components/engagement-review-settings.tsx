"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { SlidersHorizontal } from "lucide-react";
import { useId, useState } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Slider } from "@/components/ui/slider";
import { Switch } from "@/components/ui/switch";
import { permissions } from "@/config/permissions";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import { engagementOverride, thresholdPercent } from "../lib/review-override";
import { reviewKeys, reviewService } from "../services/review.service";
import { Figure } from "./office-ui";

/**
 * Mükellefe özel otomatik onay eşiği (#518 A5): İnceleme sekmesinin
 * üstünde. Ofis ayarı varsayılandır; mükellef için ezilebilir (otomatik
 * onay aç / kapat, eşik) ve ofis ayarına dönülebilir.
 */
export function EngagementReviewSettings({
  engagement,
}: {
  engagement: string;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const canRead = can(permissions.tenant.review.read);
  const canWrite = can(permissions.tenant.review.settingsWrite);
  const qc = useQueryClient();
  const id = useId();
  const key = reviewKeys.engagementSettings(slug, engagement);
  const q = useQuery({
    queryKey: key,
    queryFn: () => reviewService.engagementSettings(slug, engagement),
    enabled: canRead,
  });
  const [open, setOpen] = useState(false);
  const [autoDraft, setAutoDraft] = useState<boolean | null>(null);
  const [draft, setDraft] = useState<number | null>(null);
  const done = (message: string) => {
    setAutoDraft(null);
    setDraft(null);
    void qc.invalidateQueries({ queryKey: reviewKeys.all(slug) });
    toast.success(message);
  };
  const fail = (e: unknown) =>
    toast.error(isApiError(e) ? e.message : t("office.review_override.failed"));
  const save = useMutation({
    mutationFn: (v: { auto: boolean; threshold: number }) =>
      reviewService.putEngagementSetting(slug, engagement, {
        kind: "*",
        auto_accept: v.auto,
        threshold: (v.threshold / 100).toFixed(2),
      }),
    onSuccess: () => done(t("office.review_override.saved")),
    onError: fail,
  });
  const reset = useMutation({
    mutationFn: () => reviewService.deleteEngagementSetting(slug, engagement),
    onSuccess: () => done(t("office.review_override.reset_done")),
    onError: fail,
  });

  if (!canRead || !q.data) return null;
  const effective = q.data.effective["*"] ?? q.data.default;
  const own = engagementOverride(q.data.rows);
  const current = thresholdPercent(effective.threshold);
  const auto = autoDraft ?? effective.auto_accept;
  const value = draft ?? current;
  const dirty = auto !== effective.auto_accept || value !== current;
  const policy = effective.auto_accept
    ? t("office.review_override.policy_on", { threshold: current })
    : t("office.review_override.policy_off");
  const busy = save.isPending || reset.isPending;
  return (
    <section
      aria-labelledby={`${id}-title`}
      className="bg-surface border-line rounded-xl border p-4"
      data-testid="engagement-review-settings"
    >
      <div className="flex flex-wrap items-center gap-3">
        <SlidersHorizontal aria-hidden className="text-text-3 size-5" />
        <div className="min-w-0 flex-1">
          <h2 id={`${id}-title`} className="text-sm font-semibold">
            {t("office.review_override.title")}
          </h2>
          <p className="text-text-2 text-sm">
            {own
              ? t("office.review_override.custom", { policy })
              : t("office.review_override.inherits", { policy })}
          </p>
        </div>
        {canWrite ? (
          <Button
            variant="outline"
            size="sm"
            className="max-md:min-h-11"
            aria-expanded={open}
            aria-controls={`${id}-form`}
            onClick={() => setOpen((v) => !v)}
          >
            {open
              ? t("office.review_override.hide")
              : t("office.review_override.edit")}
          </Button>
        ) : null}
      </div>
      {open && canWrite ? (
        <div id={`${id}-form`} className="mt-4 space-y-4">
          <div className="flex min-h-11 items-center gap-3">
            <Switch
              id={`${id}-auto`}
              checked={auto}
              onCheckedChange={setAutoDraft}
            />
            <Label htmlFor={`${id}-auto`} className="font-normal">
              {t("office.review_override.auto")}
            </Label>
          </div>
          <div className="flex flex-wrap items-center gap-4">
            <Figure className="font-display text-2xl font-bold">
              %{value}
            </Figure>
            <Slider
              min={80}
              max={99}
              step={1}
              value={[value]}
              onValueChange={(v) => setDraft(v[0] ?? current)}
              disabled={!auto}
              aria-label={t("office.review_override.threshold")}
              className="min-w-56 flex-1 pointer-coarse:py-3"
            />
          </div>
          <p className="text-text-3 text-xs">
            {t("office.review_override.hint")}
          </p>
          <div className="flex flex-wrap gap-2">
            <Button
              size="sm"
              className="max-md:min-h-11"
              disabled={busy || (!dirty && !!own)}
              onClick={() => save.mutate({ auto, threshold: value })}
            >
              {t("office.review_override.save")}
            </Button>
            {own ? (
              <Button
                size="sm"
                variant="outline"
                className="max-md:min-h-11"
                disabled={busy}
                onClick={() => reset.mutate()}
              >
                {t("office.review_override.reset")}
              </Button>
            ) : null}
          </div>
        </div>
      ) : null}
    </section>
  );
}
