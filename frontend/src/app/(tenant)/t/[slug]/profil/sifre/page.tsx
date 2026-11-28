import { redirect } from "next/navigation";

import { routes } from "@/config/routes";

type PageProps = { params: Promise<{ slug: string }> };

/** Şifre ve tercihler profil ekranının bölümleri (#890 C). */
export default async function TenantProfileSubRoute({ params }: PageProps) {
  const { slug } = await params;
  let decoded = slug;
  try {
    decoded = decodeURIComponent(slug);
  } catch {
    // Bozuk kaçış: ham parça kullanılır.
  }
  redirect(routes.tenant.profile(decoded));
}
