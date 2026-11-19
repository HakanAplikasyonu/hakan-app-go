import { Skeleton } from "@/components/ui/skeleton";

/**
 * Ofis sayfaları arası geçiş (#576): tıklamadan hemen sonra iskelet
 * görünür; kenar menü ve üst çubuk yerinde kalır. Önceden sayfa 3 sn
 * hiçbir belirti olmadan bekliyordu ve kullanıcı yeniden tıklıyordu.
 */
export default function Loading() {
  return (
    <div className="space-y-4" aria-busy="true" aria-live="polite">
      <span className="sr-only">Yükleniyor…</span>
      <Skeleton className="h-8 w-64" />
      <Skeleton className="h-4 w-96 max-w-full" />
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <Skeleton className="h-24" />
        <Skeleton className="h-24" />
        <Skeleton className="h-24" />
        <Skeleton className="h-24" />
      </div>
      <Skeleton className="h-72 w-full" />
    </div>
  );
}
