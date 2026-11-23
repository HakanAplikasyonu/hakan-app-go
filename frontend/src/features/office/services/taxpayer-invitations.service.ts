import type { components } from "@/generated/api";
import { tenantRequest } from "@/lib/api/tenant-request";

export type TaxpayerInvitation = components["schemas"]["TaxpayerInvitation"];
export type TaxpayerInvitationLink =
  components["schemas"]["TaxpayerInvitationLink"];
export type TaxpayerInvitationRole = TaxpayerInvitation["role"];

const base = (engagement: string) =>
  `/v1/tenant/engagements/${encodeURIComponent(engagement)}/invitations`;

export const taxpayerInvitationKeys = {
  list: (slug: string, engagement: string) =>
    [
      "tenant",
      slug.toLowerCase(),
      "engagements",
      engagement.toLowerCase(),
      "taxpayer-invitations",
    ] as const,
};

/**
 * Mükellefi uygulamaya davet (D15): ofis davet eder, yanıt davet linkini
 * (invite_url) bir kez döner. E-posta gitmese de link çalışır; müşavir
 * kopyalayıp ya da WhatsApp ile paylaşır.
 */
export const taxpayerInvitationService = {
  list(slug: string, engagement: string) {
    return tenantRequest<{ items: TaxpayerInvitation[] }>(
      slug,
      "GET",
      base(engagement),
    );
  },
  create(
    slug: string,
    engagement: string,
    body: { email: string; role: TaxpayerInvitationRole },
  ) {
    return tenantRequest<TaxpayerInvitationLink>(
      slug,
      "POST",
      base(engagement),
      { body },
    );
  },
  /** Yeni link üretir ve e-postayı yeniden gönderir; eski link çalışmaz. */
  resend(slug: string, engagement: string, uuid: string) {
    return tenantRequest<TaxpayerInvitationLink>(
      slug,
      "POST",
      `${base(engagement)}/${encodeURIComponent(uuid)}/resend`,
    );
  },
  revoke(slug: string, engagement: string, uuid: string) {
    return tenantRequest<unknown>(
      slug,
      "DELETE",
      `${base(engagement)}/${encodeURIComponent(uuid)}`,
    );
  },
};

/** WhatsApp paylaşım bağlantısı: kısa Türkçe mesaj + davet linki. */
export function whatsappShareUrl(message: string): string {
  return `https://wa.me/?text=${encodeURIComponent(message)}`;
}
