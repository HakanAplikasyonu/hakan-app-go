"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Contact, Mail, Pencil, Phone, UserRound } from "lucide-react";
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

/** Formun alanları; hata kodları backend'in alan adlarıyla eşleşir. */
type Field =
  "phone" | "email" | "person_name" | "person_phone" | "person_email";
type FieldErrors = Partial<Record<Field, string>>;

const ERROR_KEYS: Record<string, string> = {
  telefon_gecersiz: "office.contact.phone_invalid",
  eposta_gecersiz: "office.contact.email_invalid",
  ad_gecersiz: "office.contact.person_name_invalid",
};

function PhoneLink({ phone }: { phone: string }) {
  return (
    <a
      className="text-brand font-semibold hover:underline"
      href={`tel:${phone.replace(/[^\d+]/g, "")}`}
    >
      {phone}
    </a>
  );
}

function EmailLink({ email }: { email: string }) {
  return (
    <a
      className="text-brand font-semibold hover:underline"
      href={`mailto:${email}`}
    >
      {email}
    </a>
  );
}

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
 * görebiliyorum ne de telefon numarasını"): telefon, e-posta, yetkili kişi
 * (muhasebe muhatabı: ad, telefon, e-posta) ve hesap sahibi (mükellef
 * hesabının sahibi, davetle gelir). Yazma yetkisi olan telefonu, e-postayı ve
 * yetkili kişiyi düzenler; salt okur yalnız görür.
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
              <PhoneLink phone={q.data.phone} />
            ) : (
              <span className="text-text-3">{t("office.contact.empty")}</span>
            )}
          </Item>
          <Item
            icon={<Mail className="size-4" />}
            label={t("office.contact.email")}
          >
            {q.data.email ? (
              <EmailLink email={q.data.email} />
            ) : (
              <span className="text-text-3">{t("office.contact.empty")}</span>
            )}
          </Item>
          {q.data.person ? (
            <Item
              icon={<Contact className="size-4" />}
              label={t("office.contact.person")}
            >
              <span data-testid="contact-person" className="block">
                {q.data.person.name ? (
                  <span className="block font-semibold">
                    {q.data.person.name}
                  </span>
                ) : null}
                {q.data.person.phone ? (
                  <span className="block">
                    <PhoneLink phone={q.data.person.phone} />
                  </span>
                ) : null}
                {q.data.person.email ? (
                  <span className="block">
                    <EmailLink email={q.data.person.email} />
                  </span>
                ) : null}
              </span>
            </Item>
          ) : null}
          <Item
            icon={<UserRound className="size-4" />}
            label={t("office.contact.owner")}
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
  const [values, setValues] = useState<Record<Field, string>>({
    phone: initial.phone ?? "",
    email: initial.email ?? "",
    person_name: initial.person?.name ?? "",
    person_phone: initial.person?.phone ?? "",
    person_email: initial.person?.email ?? "",
  });
  const [errors, setErrors] = useState<FieldErrors>({});

  const save = useMutation({
    mutationFn: () =>
      contactService.update(slug, engagement, {
        phone: values.phone.trim(),
        email: values.email.trim(),
        person_name: values.person_name.trim(),
        person_phone: values.person_phone.trim(),
        person_email: values.person_email.trim(),
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
          const key = d.code ? ERROR_KEYS[d.code] : undefined;
          if (key && d.field && d.field in values) {
            next[d.field as Field] = t(key);
          }
        }
      }
      setErrors(next);
      if (Object.keys(next).length === 0) {
        toast.error(t("office.contact.failed"));
      }
    },
  });

  const field = (
    name: Field,
    label: string,
    props: {
      type: "tel" | "email" | "text";
      autoComplete: string;
      maxLength: number;
      placeholder: string;
    },
  ) => {
    const id = `contact-${name.replace("_", "-")}`;
    const err = errors[name];
    return (
      <div className="space-y-1.5">
        <Label htmlFor={id}>{label}</Label>
        <Input
          id={id}
          type={props.type}
          inputMode={props.type === "text" ? undefined : props.type}
          autoComplete={props.autoComplete}
          value={values[name]}
          maxLength={props.maxLength}
          placeholder={props.placeholder}
          aria-invalid={!!err}
          aria-describedby={err ? `${id}-error` : undefined}
          onChange={(e) => setValues((v) => ({ ...v, [name]: e.target.value }))}
        />
        {err ? (
          <p id={`${id}-error`} role="alert" className="text-danger text-sm">
            {err}
          </p>
        ) : null}
      </div>
    );
  };

  const submit = (ev: FormEvent) => {
    ev.preventDefault();
    setErrors({});
    save.mutate();
  };

  return (
    <form className="grid gap-3 sm:grid-cols-2" onSubmit={submit} noValidate>
      {field("phone", t("office.contact.phone"), {
        type: "tel",
        autoComplete: "tel",
        maxLength: 40,
        placeholder: "0532 123 45 67",
      })}
      {field("email", t("office.contact.email"), {
        type: "email",
        autoComplete: "email",
        maxLength: 254,
        placeholder: "muhasebe@firma.com.tr",
      })}
      <h3 className="text-text-2 pt-2 text-sm font-semibold sm:col-span-2">
        {t("office.contact.person_heading")}
      </h3>
      <div className="sm:col-span-2">
        {field("person_name", t("office.contact.person_name"), {
          type: "text",
          autoComplete: "off",
          maxLength: 120,
          placeholder: t("office.contact.person_name_placeholder"),
        })}
      </div>
      {field("person_phone", t("office.contact.person_phone"), {
        type: "tel",
        autoComplete: "off",
        maxLength: 40,
        placeholder: "0530 111 22 33",
      })}
      {field("person_email", t("office.contact.person_email"), {
        type: "email",
        autoComplete: "off",
        maxLength: 254,
        placeholder: "ad.soyad@firma.com.tr",
      })}
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
