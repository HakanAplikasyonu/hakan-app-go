"use client";

import { ArrowLeft } from "lucide-react";
import { createContext, useContext } from "react";

import { Button } from "@/components/ui/button";
import { useLocale } from "@/providers/locale-provider";

/** Sihirbaz gezinmesi: adımlar "Geri"yi buradan alır (OfisKurulum). */
export const WizardNav = createContext<{ onBack?: () => void }>({});

/** "Geri" / "Bu adımı atla" / "Kaydet ve devam et" (son adım: "Kurulumu bitir"). */
export function StepActions({
  isLast,
  busy,
  onSkip,
  onSave,
}: {
  isLast: boolean;
  busy: boolean;
  onSkip: () => void;
  onSave: () => void;
}) {
  const { t } = useLocale();
  const { onBack } = useContext(WizardNav);
  return (
    <div className="mt-8 flex flex-wrap items-center gap-2 border-t pt-4">
      {onBack ? (
        <Button
          type="button"
          variant="ghost"
          className="h-11"
          disabled={busy}
          onClick={onBack}
        >
          <ArrowLeft className="size-4" aria-hidden />
          {t("common.back")}
        </Button>
      ) : null}
      <span className="flex-1" />
      <Button
        type="button"
        variant="outline"
        className="h-11"
        disabled={busy}
        onClick={onSkip}
      >
        {t("setup.skip")}
      </Button>
      <Button
        type="button"
        className="bg-brand text-on-brand hover:bg-brand/90 h-11"
        disabled={busy}
        onClick={onSave}
      >
        {isLast ? t("setup.finish") : t("setup.save_continue")}
      </Button>
    </div>
  );
}

/** Inline error under a step's controls. */
export function StepError({ message }: { message?: string | null }) {
  if (!message) return null;
  return (
    <p role="alert" className="text-destructive mt-2 text-sm">
      {message}
    </p>
  );
}
