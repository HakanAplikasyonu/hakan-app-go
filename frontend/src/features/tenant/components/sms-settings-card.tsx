"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { MessageSquareText } from "lucide-react";
import { useMemo, useState } from "react";
import { z } from "zod";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { EntitySectionCard } from "@/components/entity";
import {
  AppForm,
  AppInput,
  AppPassword,
  AppSelect,
  AppSwitch,
} from "@/components/forms";
import { DateCell, StatusPill, type StatusTone } from "@/components/tables";
import { Button } from "@/components/ui/button";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  smsKeys,
  smsService,
  type SmsMessage,
  type SmsSettings,
} from "@/features/tenant/services/sms.service";
import { useAppMutation } from "@/lib/query/mutation";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

type FormValues = {
  enabled: boolean;
  test_mode: boolean;
  usercode: string;
  password: string;
  msgheader: string;
  encoding: "TR" | "UTF-8" | "UNICODE";
  appname: string;
};

const statusTone: Record<SmsMessage["status"], StatusTone> = {
  pending: "info",
  sent: "ok",
  failed: "danger",
  uncertain: "warn",
  dry_run: "neutral",
};

/**
 * Ayarlar → SMS (#780 A, İletişim merkezi): ofisin kendi NetGSM hesabı.
 * Şifre sunucuya gider, geri gelmez; boş bırakmak "değiştirmedim" demektir.
 * Bağlantı testi onaylı gönderici adlarını rozet olarak gösterir; tıklanan
 * forma yazılır. Deneme modunda mesajlar yalnız kaydedilir.
 */
export function SmsSettingsCard({
  slug,
  canWrite,
}: {
  slug: string;
  canWrite: boolean;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const query = useQuery({
    queryKey: smsKeys.settings(slug),
    queryFn: () => smsService.settings(slug),
  });
  const save = useAppMutation({
    mutationFn: (v: FormValues) =>
      smsService.save(slug, {
        enabled: v.enabled,
        test_mode: v.test_mode,
        usercode: v.usercode.trim(),
        password: v.password.trim() === "" ? null : v.password,
        msgheader: v.msgheader.trim(),
        encoding: v.encoding,
        appname: v.appname.trim(),
      }),
    onSuccess: (data) => {
      qc.setQueryData(smsKeys.settings(slug), data);
      appToast.success(t("tenant.sms_saved"));
    },
  });
  const test = useAppMutation({
    mutationFn: () => smsService.test(slug),
    onSuccess: (data) => qc.setQueryData(smsKeys.settings(slug), data),
  });
  const schema = useMemo(
    () =>
      z.object({
        enabled: z.boolean(),
        test_mode: z.boolean(),
        usercode: z
          .string()
          .trim()
          .regex(/^[0-9]{0,20}$/, t("tenant.sms_usercode_invalid")),
        password: z.string(),
        msgheader: z
          .string()
          .trim()
          .refine(
            (v) => v === "" || (v.length >= 3 && v.length <= 11),
            t("tenant.sms_msgheader_invalid"),
          ),
        encoding: z.enum(["TR", "UTF-8", "UNICODE"]),
        appname: z.string().trim().max(64),
      }),
    [t],
  );
  const data = query.data;

  return (
    <EntitySectionCard title={t("tenant.sms_card_title")}>
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {data ? (
        <AppForm
          key={`${data.updated_at ?? "new"}-${data.password_set}`}
          id="sms-settings-form"
          schema={schema}
          defaultValues={{
            enabled: data.enabled,
            test_mode: data.test_mode,
            usercode: data.usercode,
            password: "",
            msgheader: data.msgheader,
            encoding: data.encoding,
            appname: data.appname,
          }}
          onSubmit={async (values: FormValues) => {
            await save.mutateAsync(values);
          }}
        >
          {(form) => (
            <div className="space-y-5" data-testid="sms-settings">
              <p className="text-text-2 text-sm">
                {t("tenant.sms_card_description")}
              </p>
              <div className="flex flex-wrap gap-2">
                <SmsModePill settings={data} />
                {data.password_set ? (
                  <StatusPill tone="ok">
                    {t("tenant.sms_password_set")}
                  </StatusPill>
                ) : null}
              </div>
              <div className="grid gap-4 sm:grid-cols-2">
                <AppSwitch
                  name="enabled"
                  label={t("tenant.sms_enabled_label")}
                  description={t("tenant.sms_enabled_help")}
                  disabled={!canWrite}
                />
                <AppSwitch
                  name="test_mode"
                  label={t("tenant.sms_test_mode_label")}
                  description={
                    data.live_allowed
                      ? t("tenant.sms_test_mode_help")
                      : t("tenant.sms_test_mode_server_off")
                  }
                  disabled={!canWrite}
                />
                <AppInput
                  name="usercode"
                  label={t("tenant.sms_usercode_label")}
                  description={t("tenant.sms_usercode_help")}
                  inputMode="numeric"
                  autoComplete="off"
                  disabled={!canWrite}
                />
                <AppPassword
                  name="password"
                  label={t("tenant.sms_password_label")}
                  description={
                    data.password_set
                      ? t("tenant.sms_password_keep")
                      : t("tenant.sms_password_help")
                  }
                  placeholder={
                    data.password_set
                      ? t("tenant.sms_password_placeholder_set")
                      : ""
                  }
                  autoComplete="new-password"
                />
                <AppInput
                  name="msgheader"
                  label={t("tenant.sms_msgheader_label")}
                  description={t("tenant.sms_msgheader_help")}
                  startIcon={MessageSquareText}
                  autoComplete="off"
                  disabled={!canWrite}
                />
                <AppSelect
                  name="encoding"
                  label={t("tenant.sms_encoding_label")}
                  options={[
                    { value: "TR", label: t("tenant.sms_encoding_tr") },
                    { value: "UTF-8", label: t("tenant.sms_encoding_utf8") },
                    {
                      value: "UNICODE",
                      label: t("tenant.sms_encoding_unicode"),
                    },
                  ]}
                />
                <AppInput
                  name="appname"
                  label={t("tenant.sms_appname_label")}
                  description={t("tenant.sms_appname_help")}
                  autoComplete="off"
                  disabled={!canWrite}
                />
              </div>
              {canWrite ? (
                <div className="flex flex-wrap justify-end gap-2">
                  <Button
                    type="button"
                    variant="outline"
                    disabled={
                      test.isPending ||
                      !data.password_set ||
                      data.usercode === "" ||
                      form.formState.isDirty
                    }
                    onClick={() => test.mutate()}
                  >
                    {test.isPending
                      ? t("tenant.sms_testing")
                      : t("tenant.sms_test_button")}
                  </Button>
                  <Button
                    type="submit"
                    disabled={
                      save.isPending ||
                      form.formState.isSubmitting ||
                      !form.formState.isDirty
                    }
                  >
                    {t("tenant.settings_save")}
                  </Button>
                </div>
              ) : null}
              {data.last_test ? (
                <div
                  className="border-border space-y-2 rounded-md border p-3"
                  data-testid="sms-last-test"
                >
                  <div className="flex flex-wrap items-center gap-2 text-sm">
                    <StatusPill tone={data.last_test.ok ? "ok" : "danger"}>
                      {data.last_test.ok
                        ? t("tenant.sms_test_ok")
                        : t("tenant.sms_test_failed")}
                    </StatusPill>
                    <span className="text-text-2">
                      {data.last_test.message}
                    </span>
                    <DateCell
                      value={data.last_test.at}
                      style="datetime"
                      className="text-text-3"
                    />
                  </div>
                  {data.last_test.headers.length > 0 ? (
                    <div className="space-y-1">
                      <p className="text-text-2 text-xs">
                        {t("tenant.sms_headers_pick")}
                      </p>
                      <div className="flex flex-wrap gap-2">
                        {data.last_test.headers.map((h) => (
                          <Button
                            key={h}
                            type="button"
                            size="sm"
                            variant="secondary"
                            className="rounded-full"
                            disabled={!canWrite}
                            onClick={() =>
                              form.setValue("msgheader", h, {
                                shouldDirty: true,
                                shouldValidate: true,
                              })
                            }
                          >
                            {h}
                          </Button>
                        ))}
                      </div>
                    </div>
                  ) : null}
                  {!data.last_test.ok && data.last_test.code === "30" ? (
                    <Code30Help />
                  ) : null}
                </div>
              ) : null}
              <Collapsible className="text-sm">
                <CollapsibleTrigger className="text-text-2 cursor-pointer">
                  {t("tenant.sms_help_title")}
                </CollapsibleTrigger>
                <CollapsibleContent>
                  <Code30Help />
                </CollapsibleContent>
              </Collapsible>
            </div>
          )}
        </AppForm>
      ) : null}
      {data && canWrite ? (
        <SmsTestMessage slug={slug} disabled={!data.enabled} />
      ) : null}
      {data ? <SmsRecent slug={slug} /> : null}
    </EntitySectionCard>
  );
}

function SmsModePill({ settings }: { settings: SmsSettings }) {
  const { t } = useLocale();
  if (!settings.enabled) {
    return <StatusPill tone="neutral">{t("tenant.sms_mode_off")}</StatusPill>;
  }
  if (settings.test_mode || !settings.live_allowed) {
    return <StatusPill tone="warn">{t("tenant.sms_mode_test")}</StatusPill>;
  }
  return <StatusPill tone="ok">{t("tenant.sms_mode_live")}</StatusPill>;
}

/** "Kod 30" hatası koddan değil hesap ayarından gelir: dört adım. */
function Code30Help() {
  const { t } = useLocale();
  return (
    <ol className="text-text-2 mt-2 list-decimal space-y-1 pl-5 text-sm">
      <li>{t("tenant.sms_help_step1")}</li>
      <li>{t("tenant.sms_help_step2")}</li>
      <li>{t("tenant.sms_help_step3")}</li>
      <li>{t("tenant.sms_help_step4")}</li>
    </ol>
  );
}

function SmsTestMessage({
  slug,
  disabled,
}: {
  slug: string;
  disabled: boolean;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const [phone, setPhone] = useState("");
  const send = useAppMutation({
    mutationFn: () =>
      smsService.sendTest(slug, phone, t("tenant.sms_test_message_text")),
    onSuccess: (m) => {
      void qc.invalidateQueries({ queryKey: smsKeys.messages(slug) });
      if (m.status === "failed") appToast.error(m.error_message);
      else if (m.status === "dry_run")
        appToast.success(t("tenant.sms_test_message_dry_run"));
      else appToast.success(t("tenant.sms_test_message_sent"));
    },
  });
  return (
    <div className="border-border mt-5 space-y-2 border-t pt-4">
      <Label htmlFor="sms-test-phone" className="text-sm font-medium">
        {t("tenant.sms_test_message_label")}
      </Label>
      <p className="text-text-2 text-xs">
        {disabled
          ? t("tenant.sms_test_message_disabled")
          : t("tenant.sms_test_message_help")}
      </p>
      <div className="flex flex-wrap gap-2">
        <Input
          id="sms-test-phone"
          value={phone}
          onChange={(e) => setPhone(e.target.value)}
          placeholder="05XX XXX XX XX"
          inputMode="tel"
          className="max-w-60"
          disabled={disabled}
        />
        <Button
          type="button"
          variant="outline"
          disabled={disabled || phone.trim() === "" || send.isPending}
          onClick={() => send.mutate()}
        >
          {t("tenant.sms_test_message_button")}
        </Button>
      </div>
    </div>
  );
}

function SmsRecent({ slug }: { slug: string }) {
  const { t } = useLocale();
  const query = useQuery({
    queryKey: smsKeys.messages(slug),
    queryFn: () => smsService.messages(slug, 10),
  });
  const items = query.data?.items ?? [];
  return (
    <div className="border-border mt-5 space-y-2 border-t pt-4">
      <h3 className="text-sm font-medium">{t("tenant.sms_recent_title")}</h3>
      {items.length === 0 ? (
        <p className="text-text-2 text-sm">{t("tenant.sms_recent_empty")}</p>
      ) : (
        <ul className="divide-border divide-y" data-testid="sms-recent">
          {items.map((m) => (
            <li key={m.uuid} className="space-y-1 py-2 text-sm">
              <div className="flex flex-wrap items-center gap-2">
                <StatusPill tone={statusTone[m.status]}>
                  {t(`tenant.sms_status_${m.status}`)}
                </StatusPill>
                <span className="figure">{m.phone}</span>
                {m.taxpayer_title ? (
                  <span className="text-text-2">{m.taxpayer_title}</span>
                ) : null}
                <DateCell
                  value={m.created_at}
                  style="datetime"
                  className="text-text-3 ml-auto"
                />
              </div>
              <p className="text-text-2 line-clamp-2">{m.body}</p>
              {m.error_message ? (
                <p className="text-text-3 text-xs">{m.error_message}</p>
              ) : null}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
