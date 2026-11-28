"use client";

import Link from "next/link";

import { routes } from "@/config/routes";
import { useLocale } from "@/providers/locale-provider";

/**
 * `/gizlilik/whatsapp-uzantisi` (#780): what the WhatsApp Web extension
 * reads, sends and stores. Written from extensions/whatsapp/src (api.ts,
 * background.ts, content.ts, intercept.ts, chat.ts); keep it in step with the
 * store form in extensions/whatsapp/store/magaza-bilgileri.md.
 */
const SECTIONS = [
  {
    id: "where",
    title: "legal.wa_privacy_where_title",
    items: ["legal.wa_privacy_where_1", "legal.wa_privacy_where_2"],
  },
  {
    id: "sent",
    title: "legal.wa_privacy_sent_title",
    items: [
      "legal.wa_privacy_sent_1",
      "legal.wa_privacy_sent_2",
      "legal.wa_privacy_sent_3",
      "legal.wa_privacy_sent_4",
      "legal.wa_privacy_sent_5",
      "legal.wa_privacy_sent_6",
    ],
  },
  {
    id: "not_read",
    title: "legal.wa_privacy_not_read_title",
    items: [
      "legal.wa_privacy_not_read_1",
      "legal.wa_privacy_not_read_2",
      "legal.wa_privacy_not_read_3",
      "legal.wa_privacy_not_read_4",
      "legal.wa_privacy_not_read_5",
    ],
  },
  {
    id: "stored",
    title: "legal.wa_privacy_stored_title",
    items: [
      "legal.wa_privacy_stored_1",
      "legal.wa_privacy_stored_2",
      "legal.wa_privacy_stored_3",
      "legal.wa_privacy_stored_4",
    ],
  },
  {
    id: "server",
    title: "legal.wa_privacy_server_title",
    items: [
      "legal.wa_privacy_server_1",
      "legal.wa_privacy_server_2",
      "legal.wa_privacy_server_3",
    ],
  },
  {
    id: "control",
    title: "legal.wa_privacy_control_title",
    items: [
      "legal.wa_privacy_control_1",
      "legal.wa_privacy_control_2",
      "legal.wa_privacy_control_3",
    ],
  },
] as const;

export function WhatsAppExtensionPrivacyPage() {
  const { t } = useLocale();
  return (
    <div className="mx-auto flex min-h-svh w-full max-w-3xl flex-col gap-6 px-4 py-8 sm:px-6">
      <article className="border-border bg-card text-card-foreground space-y-6 rounded-lg border p-5 sm:p-8">
        <header className="space-y-2">
          <h1 className="text-xl font-semibold tracking-tight">
            {t("legal.wa_privacy_title")}
          </h1>
          <p className="text-muted-foreground text-sm">
            {t("legal.wa_privacy_updated")}
          </p>
          <p className="text-sm leading-relaxed">
            {t("legal.wa_privacy_intro")}
          </p>
        </header>
        {SECTIONS.map((s) => (
          <section
            key={s.id}
            aria-labelledby={`wa-${s.id}`}
            className="space-y-2"
          >
            <h2 id={`wa-${s.id}`} className="text-base font-semibold">
              {t(s.title)}
            </h2>
            <ul className="list-disc space-y-1.5 pl-5 text-sm leading-relaxed">
              {s.items.map((key) => (
                <li key={key}>{t(key)}</li>
              ))}
            </ul>
          </section>
        ))}
        <footer className="border-border space-y-2 border-t pt-4 text-sm">
          <p>{t("legal.wa_privacy_limited_use")}</p>
          <p>
            {t("legal.wa_privacy_more")}{" "}
            <Link
              href={routes.legal("privacy_notice")}
              className="text-primary underline underline-offset-2"
            >
              {t("legal.kind_privacy_notice")}
            </Link>
          </p>
          <p className="text-muted-foreground">
            {t("legal.wa_privacy_trademark")}
          </p>
        </footer>
      </article>
    </div>
  );
}
