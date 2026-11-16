"use client";

import { HataDurumu } from "@/components/hakan";
import { useReportError } from "@/lib/observability/use-report-error";
import { useLocale } from "@/providers/locale-provider";

/**
 * Bölüm hata sınırı (#332, Durumlar.dc.html): ekran çizilirken düşen bir
 * hata. Kayıtlı veriye dokunulmaz (yalnızca görüntü düştü), bu yüzden
 * "Hiçbir şey kaybolmadı" denir; teknik kod "Ayrıntı"da.
 */
export default function SectionError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  useReportError(error);
  const { t } = useLocale();
  return (
    <div className="mx-auto w-full max-w-lg py-10">
      <HataDurumu
        baslik={t("common.state_render_error_title")}
        aciklama={t("common.state_render_error_description")}
        verilerGuvende
        kod={error.digest}
        onRetry={reset}
      />
    </div>
  );
}
