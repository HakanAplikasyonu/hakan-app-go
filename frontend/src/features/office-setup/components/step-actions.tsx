"use client";

import { useLocale } from "@/providers/locale-provider";
import { Button } from "@/components/ui/button";

/** "Atla" / "Kaydet ve devam" (last step: "Kurulumu bitir"). */
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
  return (
    <div className="mt-6 flex flex-wrap justify-end gap-2 border-t pt-4">
      <Button type="button" variant="outline" disabled={busy} onClick={onSkip}>
        {t("setup.skip")}
      </Button>
      <Button type="button" disabled={busy} onClick={onSave}>
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
