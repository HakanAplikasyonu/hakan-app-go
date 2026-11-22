"use client";

import {
  useCallback,
  useEffect,
  useId,
  useMemo,
  useSyncExternalStore,
  type CSSProperties,
} from "react";

import { Button } from "@/components/ui/button";
import { useLocale } from "@/providers/locale-provider";

import { TOUR_STEPS } from "../lib/steps";
import {
  closeTour,
  getServerTourState,
  getTourState,
  goToStep,
  subscribeTour,
  tourSeen,
  tourSeenServer,
} from "../lib/tour-store";

type Rect = { top: number; left: number; width: number; height: number };

const PAD = 6;
const CARD_WIDTH = 320;
const GAP = 16;

/** Hedef öğenin ekrandaki yeri; boyut değişince ve kaydırınca yenilenir. */
function subscribeLayout(callback: () => void): () => void {
  window.addEventListener("resize", callback);
  window.addEventListener("scroll", callback, true);
  // Kenar menü kırılımda yeniden çizildiğinde de yakalansın.
  const id = window.setInterval(callback, 400);
  return () => {
    window.removeEventListener("resize", callback);
    window.removeEventListener("scroll", callback, true);
    window.clearInterval(id);
  };
}

function findTarget(target: string | undefined): HTMLElement | null {
  if (!target || typeof document === "undefined") return null;
  const nodes = document.querySelectorAll<HTMLElement>(
    `[data-tour="${target}"]`,
  );
  for (const node of nodes) {
    const r = node.getBoundingClientRect();
    if (r.width > 0 && r.height > 0) return node;
  }
  return null;
}

/** Dizge anlık görüntü: `useSyncExternalStore` değer karşılaştırması yapar. */
function useTargetRect(target: string | undefined): Rect | null {
  const getSnapshot = useCallback(() => {
    const el = findTarget(target);
    if (!el) return "";
    const r = el.getBoundingClientRect();
    return [r.top, r.left, r.width, r.height].map(Math.round).join(",");
  }, [target]);
  const key = useSyncExternalStore(subscribeLayout, getSnapshot, () => "");
  return useMemo(() => {
    if (!key) return null;
    const [top, left, width, height] = key.split(",").map(Number);
    return { top, left, width, height };
  }, [key]);
}

function cardPosition(rect: Rect | null): CSSProperties {
  if (!rect || typeof window === "undefined") {
    return {
      top: "50%",
      left: "50%",
      transform: "translate(-50%, -50%)",
    };
  }
  const vw = window.innerWidth;
  const vh = window.innerHeight;
  const width = Math.min(CARD_WIDTH, vw - 2 * GAP);
  // Kenar menü solda: kart öğenin sağına; yer yoksa altına.
  if (rect.left + rect.width + GAP + width <= vw - GAP) {
    return {
      left: rect.left + rect.width + GAP,
      top: Math.max(GAP, Math.min(rect.top - 8, vh - 280)),
      width,
    };
  }
  const below = rect.top + rect.height + GAP;
  return {
    left: Math.max(GAP, Math.min(rect.left, vw - width - GAP)),
    top: below + 240 < vh ? below : Math.max(GAP, rect.top - 240 - GAP),
    width,
  };
}

/**
 * Rehber turu: kenar menüdeki ilgili öğeyi çerçeveler, kısa açıklama
 * gösterir; İleri / Geri / Kapat. Kabukta bir kez bağlanır; ilk girişte
 * kendiliğinden, Rehber sayfasından elle açılır.
 */
export function GuideTourHost() {
  const state = useSyncExternalStore(
    subscribeTour,
    getTourState,
    getServerTourState,
  );
  const seen = useSyncExternalStore(subscribeTour, tourSeen, tourSeenServer);
  const open = state.manual || (!seen && !state.dismissed);
  if (!open) return null;
  const step = Math.min(state.step, TOUR_STEPS.length - 1);
  return <GuideTourOverlay step={step} />;
}

function GuideTourOverlay({ step }: { step: number }) {
  const { t } = useLocale();
  const titleId = useId();
  const bodyId = useId();
  const current = TOUR_STEPS[step];
  const rect = useTargetRect(current.target);
  const last = step === TOUR_STEPS.length - 1;

  // Hedef menü öğesi kaydırılmış listede kalmışsa görünür alana getir.
  useEffect(() => {
    findTarget(current.target)?.scrollIntoView({ block: "nearest" });
  }, [current.target]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") closeTour();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  return (
    <div className="fixed inset-0 z-[60]" data-testid="guide-tour">
      {rect ? (
        // Çerçeve: dışı karartılır, hedef açık kalır.
        <div
          aria-hidden
          className="ring-brand pointer-events-none fixed rounded-lg ring-2 transition-all"
          style={{
            top: rect.top - PAD,
            left: rect.left - PAD,
            width: rect.width + 2 * PAD,
            height: rect.height + 2 * PAD,
            boxShadow: "0 0 0 9999px rgb(0 0 0 / 0.45)",
          }}
          data-testid="guide-tour-highlight"
        />
      ) : (
        <div aria-hidden className="fixed inset-0 bg-black/45" />
      )}
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        aria-describedby={bodyId}
        className="bg-popover text-popover-foreground fixed flex max-w-[calc(100vw-2rem)] flex-col gap-3 rounded-xl border p-4 shadow-lg"
        style={{ width: CARD_WIDTH, ...cardPosition(rect) }}
      >
        <p className="text-muted-foreground font-mono text-[11px] font-semibold tracking-[0.08em] uppercase">
          {t("help.guide.tour.counter", {
            current: step + 1,
            total: TOUR_STEPS.length,
          })}
        </p>
        <h2 id={titleId} className="text-base font-semibold">
          {t(current.titleKey)}
        </h2>
        <p id={bodyId} className="text-sm leading-relaxed">
          {t(current.bodyKey)}
        </p>
        <div className="flex flex-wrap items-center gap-2 pt-1">
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={closeTour}
            data-testid="guide-tour-close"
          >
            {t("help.guide.tour.close")}
          </Button>
          <div className="ms-auto flex gap-2">
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={step === 0}
              onClick={() => goToStep(step - 1)}
              data-testid="guide-tour-prev"
            >
              {t("help.guide.tour.prev")}
            </Button>
            <Button
              type="button"
              size="sm"
              autoFocus
              onClick={() => (last ? closeTour() : goToStep(step + 1))}
              data-testid="guide-tour-next"
            >
              {last ? t("help.guide.tour.finish") : t("help.guide.tour.next")}
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
}
