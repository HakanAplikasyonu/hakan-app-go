import { Skeleton } from "@/components/ui/skeleton";

/**
 * Mükellef sekmeleri arası geçiş (#576): başlık ve sekme çubuğu yerinde
 * kalır, yalnız içerik iskelete döner.
 */
export default function Loading() {
  return (
    <div className="space-y-3" aria-busy="true" aria-live="polite">
      <span className="sr-only">Yükleniyor…</span>
      <Skeleton className="h-10 w-full" />
      <Skeleton className="h-64 w-full" />
    </div>
  );
}
