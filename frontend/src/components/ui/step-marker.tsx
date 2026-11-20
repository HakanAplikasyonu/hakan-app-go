import { cn } from "@/lib/utils";

/**
 * Akış adımının durumu (EasyTrade dili): tamamlanan sakin ana renkte dolu,
 * sıradaki halka, bekleyen gri halka, hata küçük kırmızı nokta. İşaret
 * süsleyicidir (aria-hidden); durumu her zaman yanındaki metin söyler.
 */
export type StepState = "done" | "current" | "pending" | "error";

const markerClass: Record<StepState, string> = {
  done: "bg-step-done",
  current: "bg-surface ring-step-current ring-[1.5px] ring-inset",
  pending: "bg-surface ring-step-pending ring-1 ring-inset",
  error: "bg-step-error",
};

export function StepMarker({
  state,
  className,
}: {
  state: StepState;
  className?: string;
}) {
  return (
    <span
      aria-hidden
      data-step={state}
      className={cn(
        "inline-block size-2 shrink-0 rounded-full",
        markerClass[state],
        className,
      )}
    />
  );
}

/**
 * Yatay ince adım çizgisi: küçük işaretler 1 px çizgiyle bağlanır, adım adı
 * işaretin altında. Tamamlanan adımlar arası çizgi ana renkte.
 */
export function StepTrack({
  steps,
  label,
  className,
}: {
  steps: { key: string; label: string; state: StepState }[];
  label: string;
  className?: string;
}) {
  return (
    <ol aria-label={label} className={cn("flex items-start", className)}>
      {steps.map((s, i) => (
        <li key={s.key} className="flex min-w-0 flex-1 flex-col gap-1.5">
          <span className="flex items-center">
            <StepMarker state={s.state} />
            {i < steps.length - 1 ? (
              <span
                aria-hidden
                className={cn(
                  "mx-1 h-px flex-1",
                  s.state === "done" ? "bg-step-done/60" : "bg-step-line",
                )}
              />
            ) : null}
          </span>
          <span
            className={cn(
              "truncate text-[11px] font-medium tracking-wide uppercase",
              s.state === "pending" ? "text-text-3" : "text-text-2",
            )}
          >
            {s.label}
          </span>
        </li>
      ))}
    </ol>
  );
}
