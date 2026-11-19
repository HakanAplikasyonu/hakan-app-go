"use client";

import { CloudOff, Lock } from "lucide-react";
import { useState, type ReactNode } from "react";

import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { KristalYukleyici } from "./kristal-yukleyici";
import { TacRozeti } from "./tac-rozeti";

/*
 * Ortak durumlar (#332, tuval: Durumlar.dc.html). Kural:
 *   boş durum = ne olduğu + neden + tek düğme (BosDurum, bos-durum.tsx);
 *   hata = ne oldu + veri güvende mi + ne yapmalı; teknik kod yalnızca
 *   "Ayrıntı"da.
 */

function DurumKarti({
  children,
  className,
  role,
}: {
  children: ReactNode;
  className?: string;
  role?: "alert" | "status";
}) {
  return (
    <div
      role={role}
      className={cn(
        "bg-card text-card-foreground flex flex-col items-center gap-3.5 rounded-xl border px-7 py-9 text-center",
        className,
      )}
    >
      {children}
    </div>
  );
}

function DurumIkonu({
  children,
  ton,
}: {
  children: ReactNode;
  ton: "tehlike" | "notr";
}) {
  return (
    <span
      aria-hidden="true"
      className={cn(
        "flex size-16 items-center justify-center rounded-xl",
        ton === "tehlike"
          ? "bg-danger-tint text-danger"
          : "bg-surface-3 text-text-2",
      )}
    >
      {children}
    </span>
  );
}

type HataDurumuProps = {
  /** Ne oldu (varsayılan: "Sunucuya ulaşılamıyor"). */
  baslik?: string;
  /** Ne yapmalı / ek açıklama. */
  aciklama?: string;
  /**
   * Veri güvende mi: true ise "Hiçbir şey kaybolmadı" söylenir. Bilinmiyorsa
   * verilmez; yanlış güvence verilmez.
   */
  verilerGuvende?: boolean;
  /** Teknik kod (ör. NETWORK_TIMEOUT, istek kimliği); yalnızca "Ayrıntı"da. */
  kod?: string;
  onRetry?: () => void;
  className?: string;
};

/** Hata: ne oldu + veri güvende mi + yeniden dene. Kod "Ayrıntı"da. */
export function HataDurumu({
  baslik,
  aciklama,
  verilerGuvende,
  kod,
  onRetry,
  className,
}: HataDurumuProps) {
  const { t } = useLocale();
  const [ayrinti, setAyrinti] = useState(false);
  return (
    <DurumKarti role="alert" className={className}>
      <DurumIkonu ton="tehlike">
        <CloudOff className="size-7" strokeWidth={1.75} />
      </DurumIkonu>
      <h3 className="font-display text-[19px] font-bold">
        {baslik ?? t("common.state_error_title")}
      </h3>
      <p className="text-text-2 max-w-[340px] text-sm">
        {aciklama ?? t("common.state_error_description")}
        {verilerGuvende ? ` ${t("common.state_error_data_safe")}` : ""}
      </p>
      {onRetry ? (
        <Button onClick={onRetry}>{t("common.state_retry")}</Button>
      ) : null}
      {kod ? (
        <div className="text-text-2 text-xs">
          <Button
            variant="link"
            className="text-text-2 hover:text-text-2 h-auto p-0 text-xs font-normal underline underline-offset-2"
            aria-expanded={ayrinti}
            onClick={() => setAyrinti((v) => !v)}
          >
            {t("common.state_details")}
          </Button>
          {ayrinti ? (
            <span className="ms-1.5 font-mono" data-visual-mask>
              {kod}
            </span>
          ) : null}
        </div>
      ) : null}
    </DurumKarti>
  );
}

type YetkiYokProps = {
  /** Kullanıcının rol adı (biliniyorsa gösterilir). */
  rol?: string;
  /** Ne için yetki gerektiği ve kimden istenmeli. */
  aciklama?: string;
  /** Tek eylem (ör. "Ana sayfaya dön" bağlantısı). */
  eylem?: ReactNode;
  className?: string;
};

/** Yetki 403: bu sayfa için yetkin yok + rolün + kimden istemeli. */
export function YetkiYok({ rol, aciklama, eylem, className }: YetkiYokProps) {
  const { t } = useLocale();
  return (
    <DurumKarti className={className}>
      <DurumIkonu ton="notr">
        <Lock className="size-7" strokeWidth={1.75} />
      </DurumIkonu>
      <h3 className="font-display text-[19px] font-bold">
        {t("common.state_forbidden_title")}
      </h3>
      <p className="text-text-2 max-w-[340px] text-sm">
        {rol ? `${t("common.state_forbidden_role", { role: rol })} ` : ""}
        {aciklama ?? t("common.state_forbidden_description")}
      </p>
      {eylem ? <div>{eylem}</div> : null}
    </DurumKarti>
  );
}

type DonemTamamProps = {
  /** Ör. "Ekim dönemi tamam". */
  baslik: string;
  /** Ör. "38 mükellefin tamamı Luca'ya aktarıldı. Dönem kilitlendi." */
  aciklama?: string;
  /** Tek eylem (ör. "Kasım'a geç"). */
  eylem?: ReactNode;
  className?: string;
};

/**
 * "Dönem tamam" kutlaması: taçtaki üç kristal dolu. Tek kutlama anı; hareket
 * yok (README, "Hareket").
 */
export function DonemTamam({
  baslik,
  aciklama,
  eylem,
  className,
}: DonemTamamProps) {
  return (
    <DurumKarti role="status" className={className}>
      <TacRozeti durum="aktarildi" boyut={72} />
      <h3 className="font-display text-[19px] font-bold">{baslik}</h3>
      {aciklama ? (
        <p className="text-text-2 max-w-[340px] text-sm">{aciklama}</p>
      ) : null}
      {eylem ? <div>{eylem}</div> : null}
    </DurumKarti>
  );
}

type YuklemeDurumuProps = {
  /** Ne yükleniyor (ör. "Luca dosyası hazırlanıyor · 186 fiş"). */
  etiket?: string;
  /** İskelet satır sayısı. */
  satir?: number;
  className?: string;
};

/**
 * Yükleme: iskelet + 400 ms sonra kristal (kısa işlemlerde kristal
 * görünmez). Listeler yalnızca iskelet kullanır; bu, bir ekranın tamamı
 * beklerken içindir.
 */
export function YuklemeDurumu({
  etiket,
  satir = 3,
  className,
}: YuklemeDurumuProps) {
  const { t } = useLocale();
  return (
    <div
      className={cn(
        "bg-card flex flex-col gap-3 rounded-xl border px-7 py-7",
        className,
      )}
    >
      <div className="flex items-center gap-3" aria-hidden="true">
        <Skeleton className="size-8 rounded-md" />
        <div className="flex grow flex-col gap-1.5">
          <Skeleton className="h-3 w-3/5" />
          <Skeleton className="h-3 w-2/5" />
        </div>
      </div>
      <hr className="border-border" />
      {Array.from({ length: satir }, (_, i) => (
        <div key={i} className="flex items-center gap-3" aria-hidden="true">
          <Skeleton className="size-4" />
          <Skeleton className="h-3 grow" />
          <Skeleton className="h-3 w-[70px]" />
        </div>
      ))}
      <div className="flex flex-col items-center gap-3 pt-4">
        <KristalYukleyici etiket={etiket ?? t("common.loading")} />
        {etiket ? <span className="text-text-2 text-sm">{etiket}</span> : null}
      </div>
    </div>
  );
}
