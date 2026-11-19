import Link from "next/link";

import { brand } from "@/config/brand";

/**
 * Bulunamayan sayfa (#576): Türkçe ve markalı; kullanıcıyı ana sayfaya
 * döndürür. Dil sağlayıcısına bağlı değil: her yerde (oturum açılmamışken
 * de) çalışır.
 */
export default function NotFound() {
  return (
    <main className="bg-background text-foreground flex min-h-screen flex-col items-center justify-center gap-4 p-6 text-center">
      <p className="text-text-3 font-mono text-sm">404</p>
      <h1 className="font-display text-2xl font-semibold">
        Aradığınız sayfa bulunamadı
      </h1>
      <p className="text-text-2 max-w-md text-sm">
        Adres yanlış yazılmış ya da sayfa kaldırılmış olabilir. {brand.name}{" "}
        ana sayfasından devam edebilirsiniz.
      </p>
      <Link
        href="/"
        className="bg-brand text-on-brand rounded-md px-4 py-2 text-sm font-medium hover:opacity-90"
      >
        Ana sayfaya dön
      </Link>
    </main>
  );
}
