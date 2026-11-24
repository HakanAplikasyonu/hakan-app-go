"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Mail, Pencil, Phone, UserRound } from "lucide-react";
import { useState, type FormEvent, type ReactNode } from "react";
import { toast } from "sonner";

import { ErrorState } from "@/components/common/error-state";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { permissions } from "@/config/permissions";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import {
  contactKeys,
  contactService,
  type EngagementContacts,
} from "../services/contact.service";

/** Alan hata kodları (backend `telefon_gecersiz` / `eposta_gecersiz`). */
type FieldErrors = { phone?: string; email?: string };

function Item({
  icon,
  label,
  children,
}: {
  icon: ReactNode;
  label: string;
  children: ReactNode;
}) {
  return (
    <li className="flex min-h-11 items-start gap-3 py-2">
      <span className="text-text-3 mt-0.5" aria-hidden>
        {icon}
      </span>
      <span className="min-w-0 flex-1">
        <span className="text-text-2 block text-xs">{label}</span>
        <span className="block text-sm break-words">{children}</span>
      </span>
    </li>
  );
}

/**
 * Mükellef sayfasında "İletişim bilgileri" (#780, Oğuz: "ne mailini
 * görebiliyorum ne de telefon numarasını"): telefon, e-posta ve yetkili kişi
 * (mükellef hesabının sahibi). Yazma yetkisi olan telefonu ve e-postayı
 * düzenler; salt okur yalnız görür. Yetkili kişi mükellef davetiyle gelir.
 */
export function TaxpayerContactSection({ engagement }: { engagement: string }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const canWrite = can(permissions.tenant.engagements.write);
  const [editing, setEditing] = useState(false);

  const q = useQuery({
    queryKey: contactKeys.detail(slug, engagement),
    queryFn: () => contactService.get(slug, engagement),
  });
  const owners = (q.data?.members ?? []).filter((m) => m.role === "owner");

  return (
    <section
      aria-labelledby="taxpayer-contact-title"
      className="bg-surface border-line rounded-xl border p-4 md:p-5"
      data-testid="taxpayer-contact"
    >
      <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
        <h2
          id="taxpayer-contact-title"
          className="font-display text-base font-bold"
        >
          {t("office.contact.title")}
        </h2>
        {canWrite && q.data && !editing ? (
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => setEditing(true)}
          >
            <Pencil className="size-4" aria-hidden />
            {t("office.contact.edit")}
          </Button>
        ) : null}
      </div>

      {q.isLoading ? (
        <Skeleton className="h-24 w-full" />
      ) : q.isError || !q.data ? (
        <ErrorState
          title={t("office.contact.load_failed")}
          onRetry={() => void q.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : editing ? (
        <ContactForm
          engagement={engagement}
          initial={q.data}
          onDone={() => setEditing(false)}
        />
      ) : (
        <ul className="divide-line divide-y">
          <Item
            icon={<Phone className="size-4" />}
            label={t("office.contact.phone")}
          >
            {q.data.phone ? (
              <a
                className="text-brand font-semibold hover:underline"
                href={`tel:${q.data.phone.replace(/[^\d+]/g, "")}`}
              >
                {q.data.phone}
              </a>
            ) : (
              <span className="text-text-3">{t("office.contact.empty")}</span>
            )}
          </Item>
          <Item
            icon={<Mail className="size-4" />}
            label={t("office.contact.email")}
          >
            {q.data.email ? (
              <a
                className="text-brand font-semibold hover:underline"
                href={`mailto:${q.data.email}`}
              >
                {q.data.email}
              </a>
            ) : (
              <span className="text-text-3">{t("office.contact.empty")}</span>
            )}
          </Item>
          <Item
            icon={<UserRound className="size-4" />}
            label={t("office.contact.person")}
          >
            {owners.length ? (
              owners.map((m) => (
                <span key={m.uuid} className="block">
                  {m.name} {m.surname}
                  {m.email ? (
                    <span className="text-text-2"> · {m.email}</span>
                  ) : null}
                </span>
              ))
            ) : (
              <span className="text-text-3">
                {t("office.contact.person_empty")}
              </span>
            )}
          </Item>
        </ul>
      )}
    </section>
  );
}

function ContactForm({
  engagement,
  initial,
  onDone,
}: {
  engagement: string;
  initial: EngagementContacts;
  onDone: () => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const qc = useQueryClient();
  const [phone, setPhone] = useState(initial.phone ?? "");
  const [email, setEmail] = useState(initial.email ?? "");
  const [errors, setErrors] = useState<FieldErrors>({});

  const save = useMutation({
    mutationFn: () =>
      contactService.update(slug, engagement, {
        phone: phone.trim(),
        email: email.trim(),
      }),
    onSuccess: (data) => {
      qc.setQueryData(contactKeys.detail(slug, engagement), data);
      toast.success(t("office.contact.saved"));
      onDone();
    },
    onError: (e) => {
      const next: FieldErrors = {};
      if (isApiError(e)) {
        for (const d of e.details) {
          if (d.code === "telefon_gecersiz") {
            next.phone = t("office.contact.phone_invalid");
          }
          if (d.code === "eposta_gecersiz") {
            next.email = t("office.contact.email_invalid");
          }
        }
      }
      setErrors(next);
      if (!next.phone && !next.email) toast.error(t("office.contact.failed"));
    },
  });

  const submit = (ev: FormEvent) => {
    ev.preventDefault();
    setErrors({});
    save.mutate();
  };

  return (
    <form className="grid gap-3 sm:grid-cols-2" onSubmit={submit} noValidate>
      <div className="space-y-1.5">
        <Label htmlFor="contact-phone">{t("office.contact.phone")}</Label>
        <Input
          id="contact-phone"
          type="tel"
          inputMode="tel"
          autoComplete="tel"
          value={phone}
          maxLength={40}
          placeholder="0532 123 45 67"
          aria-invalid={!!errors.phone}
          aria-describedby={errors.phone ? "contact-phone-error" : undefined}
          onChange={(e) => setPhone(e.target.value)}
        />
        {errors.phone ? (
          <p
            id="contact-phone-error"
            role="alert"
            className="text-danger text-sm"
          >
            {errors.phone}
          </p>
        ) : null}
      </div>
      <div className="space-y-1.5">
        <Label htmlFor="contact-email">{t("office.contact.email")}</Label>
        <Input
          id="contact-email"
          type="email"
          inputMode="email"
          autoComplete="email"
          value={email}
          maxLength={254}
          placeholder="muhasebe@firma.com.tr"
          aria-invalid={!!errors.email}
          aria-describedby={errors.email ? "contact-email-error" : undefined}
          onChange={(e) => setEmail(e.target.value)}
        />
        {errors.email ? (
          <p
            id="contact-email-error"
            role="alert"
            className="text-danger text-sm"
          >
            {errors.email}
          </p>
        ) : null}
      </div>
      <p className="text-text-2 text-xs sm:col-span-2">
        {t("office.contact.hint")}
      </p>
      <div className="flex justify-end gap-2 sm:col-span-2">
        <Button type="button" variant="outline" onClick={onDone}>
          {t("common.cancel")}
        </Button>
        <Button type="submit" disabled={save.isPending}>
          {t("common.save")}
        </Button>
      </div>
    </form>
  );
}
