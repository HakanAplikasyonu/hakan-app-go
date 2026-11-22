"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useRef, useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
  FOREIGN_INVOICE_MAX_BYTES,
  currentForeignPeriod,
  taxpayerForeignInvoiceKeys,
  taxpayerForeignInvoiceService,
} from "@/features/taxpayer/services/foreign-invoices.service";
import { ApiError } from "@/lib/api/errors";
import { useLocale } from "@/providers/locale-provider";

import { formatDate } from "./format";

/**
 * "Yabancı hizmet faturası gönder" (#780 KDV2 A2): Google Ads, AWS, yazılım
 * lisansı gibi yurt dışı hizmet faturalarının fotoğrafı ya da PDF'i ofise
 * gider; ofis inceler, KDV2'yi hesaplar. Altında bu ay gönderilenler, salt
 * okunur. Ofis bölümü kapattıysa kart hiç çizilmez.
 */
export function TaxpayerForeignInvoicesCard({ taxpayer }: { taxpayer: string }) {
  const { t, locale } = useLocale();
  const qc = useQueryClient();
  const input = useRef<HTMLInputElement>(null);
  const [note, setNote] = useState("");
  const [message, setMessage] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const period = currentForeignPeriod();
  const list = useQuery({
    queryKey: taxpayerForeignInvoiceKeys.list(taxpayer, period),
    queryFn: () => taxpayerForeignInvoiceService.list(taxpayer, period),
    retry: false,
  });
  const send = useMutation({
    mutationFn: (file: File) =>
      taxpayerForeignInvoiceService.upload(taxpayer, file, note),
    onSuccess: (res) => {
      setError(null);
      setNote("");
      setMessage(
        res.replayed
          ? t("taxpayer.foreign_sent_again")
          : t("taxpayer.foreign_sent"),
      );
      void qc.invalidateQueries({
        queryKey: taxpayerForeignInvoiceKeys.all(taxpayer),
      });
    },
    onError: (err) => {
      setMessage(null);
      const code = err instanceof ApiError ? err.code : undefined;
      const status = err instanceof ApiError ? err.status : undefined;
      setError(
        code === "NO_ACTIVE_ENGAGEMENT"
          ? t("taxpayer.upload_no_office")
          : status === 400
            ? t("taxpayer.foreign_unsupported")
            : status === 409
              ? t("taxpayer.foreign_off")
              : t("taxpayer.upload_failed"),
      );
    },
  });

  // Ofis bölümü kapattı ya da aktif ofis yok: kart gösterilmez.
  if (list.isError || (list.data && !list.data.enabled)) return null;
  if (!list.data) return null;

  const items = list.data.items;
  return (
    <Card data-testid="foreign-invoice-card">
      <CardHeader>
        <CardTitle>{t("taxpayer.foreign_title")}</CardTitle>
        <CardDescription>{t("taxpayer.foreign_description")}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="space-y-2">
          <Label htmlFor="foreign-invoice-note">
            {t("taxpayer.foreign_note_label")}
          </Label>
          <Textarea
            id="foreign-invoice-note"
            value={note}
            maxLength={1000}
            rows={2}
            placeholder={t("taxpayer.foreign_note_placeholder")}
            onChange={(e) => setNote(e.target.value)}
          />
        </div>
        <input
          ref={input}
          type="file"
          accept="application/pdf,image/jpeg,image/png,image/webp,image/heic,.pdf,.jpg,.jpeg,.png,.webp,.heic"
          className="hidden"
          data-testid="foreign-invoice-file"
          onChange={(e) => {
            const file = e.target.files?.[0];
            e.target.value = "";
            if (!file) return;
            if (file.size > FOREIGN_INVOICE_MAX_BYTES) {
              setMessage(null);
              setError(t("taxpayer.foreign_too_large"));
              return;
            }
            send.mutate(file);
          }}
        />
        <Button
          onClick={() => input.current?.click()}
          disabled={send.isPending}
        >
          {send.isPending
            ? t("taxpayer.upload_sending")
            : t("taxpayer.foreign_choose")}
        </Button>
        {error ? (
          <p className="text-destructive text-sm" role="alert">
            {error}
          </p>
        ) : null}
        {message ? (
          <p className="text-sm" role="status">
            {message}
          </p>
        ) : null}

        <div className="space-y-2 border-t pt-4">
          <p className="text-sm font-medium" data-testid="foreign-invoice-count">
            {t("taxpayer.foreign_received", { count: list.data.count })}
          </p>
          {items.length === 0 ? (
            <p className="text-muted-foreground text-sm">
              {t("taxpayer.foreign_empty")}
            </p>
          ) : (
            <ul className="divide-y" data-testid="foreign-invoice-list">
              {items.map((it) => (
                <li
                  key={it.uuid}
                  className="flex items-center justify-between gap-3 py-2 text-sm"
                >
                  <div>
                    <div>{it.vendor || t("taxpayer.foreign_vendor_pending")}</div>
                    <div className="text-muted-foreground text-xs">
                      {it.invoice_date
                        ? t("taxpayer.foreign_invoice_date", {
                            date: formatDate(it.invoice_date, locale),
                          })
                        : t("taxpayer.foreign_sent_at", {
                            date: formatDate(it.created_at, locale),
                          })}
                    </div>
                  </div>
                  <Badge
                    variant={it.status === "processed" ? "success" : "secondary"}
                  >
                    {t(`taxpayer.foreign_status_${it.status}`)}
                  </Badge>
                </li>
              ))}
            </ul>
          )}
        </div>
      </CardContent>
    </Card>
  );
}
