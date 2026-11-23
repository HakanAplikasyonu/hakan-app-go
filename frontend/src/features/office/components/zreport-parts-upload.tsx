"use client";

import { useMutation, useQuery } from "@tanstack/react-query";
import { ArrowDown, ArrowUp, Layers, X } from "lucide-react";
import { useId, useState } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";

import {
  MAX_Z_PARTS,
  receiptsService,
  zPartsSettingsKey,
} from "../services/receipts.service";

/**
 * Parçalı Z raporu (Oğuz'un demo geri bildirimi): uzun bir Z raporu tek
 * karede çekilemez; ofis birkaç fotoğrafı sırasıyla parça 1, 2, 3 … olarak
 * seçer, sıralar ve tek seferde yükler. Sunucu tek Z raporu çıkarır; Z
 * raporunun yan panelinde parçalar sırasıyla görünür. Ofis ayarı kapalıysa
 * bölüm görünmez.
 */
export function ZReportPartsUpload({
  engagement,
  onUploaded,
}: {
  engagement: string;
  onUploaded: () => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const id = useId();
  const [open, setOpen] = useState(false);
  const [files, setFiles] = useState<File[]>([]);
  const settings = useQuery({
    queryKey: zPartsSettingsKey(slug),
    queryFn: () => receiptsService.partsSettings(slug),
    staleTime: 60_000,
  });
  const upload = useMutation({
    mutationFn: (list: File[]) =>
      receiptsService.uploadZReportParts(slug, engagement, list),
    onSuccess: (v) => {
      setFiles([]);
      setOpen(false);
      onUploaded();
      const count = v.parts?.length || 1;
      toast.success(
        v.status === "failed"
          ? t("office.zreports.parts_unreadable")
          : t("office.zreports.parts_uploaded", { count }),
      );
    },
    onError: (e) =>
      toast.error(
        isApiError(e) && e.code === "Z_REPORT_PARTS_OFF"
          ? t("office.zreports.parts_off")
          : t("office.zreports.parts_failed"),
      ),
  });

  if (settings.data?.enabled === false) return null;

  const move = (i: number, by: number) =>
    setFiles((list) => {
      const next = [...list];
      const [f] = next.splice(i, 1);
      next.splice(i + by, 0, f);
      return next;
    });

  if (!open)
    return (
      <Button
        variant="outline"
        size="sm"
        className="max-md:min-h-11"
        onClick={() => setOpen(true)}
        data-testid="zreport-parts-open"
      >
        <Layers className="size-4" aria-hidden />
        {t("office.zreports.parts_open")}
      </Button>
    );

  return (
    <section
      className="bg-surface border-line space-y-3 rounded-xl border p-4"
      aria-label={t("office.zreports.parts_title")}
      data-testid="zreport-parts-upload"
    >
      <div className="space-y-1">
        <h3 className="text-sm font-semibold">
          {t("office.zreports.parts_title")}
        </h3>
        <p className="text-text-2 text-sm">
          {t("office.zreports.parts_hint", { max: MAX_Z_PARTS })}
        </p>
      </div>
      <div className="space-y-1">
        <Label htmlFor={`${id}-files`}>
          {t("office.zreports.parts_pick")}
        </Label>
        <Input
          id={`${id}-files`}
          type="file"
          accept="image/jpeg,image/png,image/webp"
          multiple
          data-testid="zreport-parts-input"
          onChange={(e) => {
            const picked = Array.from(e.target.files ?? []);
            e.target.value = "";
            setFiles((list) => [...list, ...picked].slice(0, MAX_Z_PARTS));
          }}
        />
      </div>
      {files.length ? (
        <ol className="space-y-1.5" data-testid="zreport-parts-list">
          {files.map((f, i) => (
            <li
              key={`${f.name}-${i}`}
              className="border-line flex items-center gap-2 rounded-md border px-2 py-1 text-sm"
              data-testid={`zreport-parts-item-${i + 1}`}
            >
              <span className="font-semibold whitespace-nowrap">
                {t("office.zreport_view.part", { no: i + 1 })}
              </span>
              <span className="text-text-2 min-w-0 flex-1 truncate">
                {f.name}
              </span>
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                disabled={i === 0}
                aria-label={t("office.zreports.parts_up", { no: i + 1 })}
                onClick={() => move(i, -1)}
              >
                <ArrowUp className="size-4" aria-hidden />
              </Button>
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                disabled={i === files.length - 1}
                aria-label={t("office.zreports.parts_down", { no: i + 1 })}
                onClick={() => move(i, 1)}
              >
                <ArrowDown className="size-4" aria-hidden />
              </Button>
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                aria-label={t("office.zreports.parts_remove", { no: i + 1 })}
                onClick={() =>
                  setFiles((list) => list.filter((_, j) => j !== i))
                }
              >
                <X className="size-4" aria-hidden />
              </Button>
            </li>
          ))}
        </ol>
      ) : null}
      <div className="flex flex-wrap gap-2">
        <Button
          size="sm"
          className="max-md:min-h-11"
          disabled={!files.length || upload.isPending}
          onClick={() => upload.mutate(files)}
          data-testid="zreport-parts-submit"
        >
          {t("office.zreports.parts_submit", { count: files.length })}
        </Button>
        <Button
          size="sm"
          variant="outline"
          className="max-md:min-h-11"
          disabled={upload.isPending}
          onClick={() => {
            setFiles([]);
            setOpen(false);
          }}
        >
          {t("office.zreports.parts_cancel")}
        </Button>
      </div>
    </section>
  );
}
