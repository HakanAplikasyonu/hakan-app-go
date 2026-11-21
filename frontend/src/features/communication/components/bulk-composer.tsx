"use client";

import { Mail, MessageSquareText, Send, TriangleAlert } from "lucide-react";
import { useMemo, useRef, useState } from "react";

import { StatusPill } from "@/components/tables";
import { Button } from "@/components/ui/button";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Hint } from "@/components/ui/hint";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { TabStrip, TabStripItem } from "@/components/ui/tab-strip";
import { Textarea } from "@/components/ui/textarea";
import { useLocale } from "@/providers/locale-provider";

import {
  usePreviewBulkSend,
  useSmsSettings,
  useStartBulkSend,
} from "../hooks/use-communication";
import {
  BULK_SMS_PART_LIMIT,
  MESSAGE_VARIABLES,
  smsLength,
} from "../lib/sms-length";
import type {
  BulkChannel,
  BulkSendInput,
  BulkSendPreview,
  BulkSendTarget,
} from "../services/communication.service";
import { BulkProgress } from "./bulk-progress";

/**
 * Toplu mesaj yazma alanı (#780 B): SMS ya da e-posta sekmesi, değişken
 * etiketleri, canlı SMS boyu sayacı; "Önizle ve gönder" onay penceresini
 * açar (kaç mükellefe gideceği, elenen tekrarlar, atlananlar, uzunluk
 * engeli). Onaydan sonra gönderim arka planda yürür, ilerleme çubuğu akar.
 */
export function BulkComposer({
  target,
  onSent,
}: {
  /** null: henüz hedef seçilmedi. */
  target: BulkSendTarget | null;
  onSent?: () => void;
}) {
  const { t } = useLocale();
  const sms = useSmsSettings();
  const preview = usePreviewBulkSend();
  const start = useStartBulkSend();
  const [channel, setChannel] = useState<BulkChannel>("sms");
  const [subject, setSubject] = useState("");
  const [body, setBody] = useState("");
  const [check, setCheck] = useState<BulkSendPreview | null>(null);
  const [bulkUuid, setBulkUuid] = useState<string | null>(null);
  const textRef = useRef<HTMLTextAreaElement>(null);

  const encoding = sms.data?.encoding ?? "TR";
  const length = useMemo(() => smsLength(body, encoding), [body, encoding]);
  const smsOff = channel === "sms" && sms.data && !sms.data.enabled;
  const input = (): BulkSendInput | null =>
    target
      ? {
          channel,
          subject: channel === "mail" ? subject : "",
          body,
          target,
        }
      : null;
  const ready =
    Boolean(target) &&
    body.trim() !== "" &&
    (channel === "sms" || subject.trim() !== "") &&
    !smsOff;

  const insertVariable = (v: string) => {
    const el = textRef.current;
    if (!el) {
      setBody((b) => `${b}${v}`);
      return;
    }
    const at = el.selectionStart ?? body.length;
    const end = el.selectionEnd ?? at;
    const next = `${body.slice(0, at)}${v}${body.slice(end)}`;
    setBody(next);
    requestAnimationFrame(() => {
      el.focus();
      el.setSelectionRange(at + v.length, at + v.length);
    });
  };

  const openPreview = () => {
    const i = input();
    if (!i) return;
    preview.mutate(i, { onSuccess: (data) => setCheck(data) });
  };

  const confirm = () => {
    const i = input();
    if (!i) return;
    start.mutate(i, {
      onSuccess: (data) => {
        setCheck(null);
        setBulkUuid(data.uuid);
        onSent?.();
      },
    });
  };

  if (bulkUuid) {
    return (
      <BulkProgress
        uuid={bulkUuid}
        onNew={() => {
          setBulkUuid(null);
          setBody("");
          setSubject("");
        }}
      />
    );
  }

  return (
    <div className="flex min-w-0 flex-col gap-4" data-testid="bulk-composer">
      <TabStrip aria-label={t("communication.channel_label")}>
        <TabStripItem
          selected={channel === "sms"}
          onClick={() => setChannel("sms")}
        >
          <MessageSquareText className="mr-1.5 inline size-4" />
          {t("communication.channel_sms")}
        </TabStripItem>
        <TabStripItem
          selected={channel === "mail"}
          onClick={() => setChannel("mail")}
        >
          <Mail className="mr-1.5 inline size-4" />
          {t("communication.channel_mail")}
        </TabStripItem>
      </TabStrip>

      {smsOff ? (
        <p className="text-warn flex items-start gap-2 text-sm">
          <TriangleAlert className="mt-0.5 size-4 shrink-0" />
          {t("communication.sms_off")}
        </p>
      ) : null}

      {channel === "mail" ? (
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="bulk-subject">{t("communication.subject")}</Label>
          <Input
            id="bulk-subject"
            value={subject}
            maxLength={200}
            onChange={(e) => setSubject(e.target.value)}
            placeholder={t("communication.subject_placeholder")}
          />
        </div>
      ) : null}

      <div className="flex flex-col gap-1.5">
        <Label htmlFor="bulk-body">{t("communication.body")}</Label>
        <Textarea
          id="bulk-body"
          ref={textRef}
          rows={channel === "sms" ? 4 : 8}
          value={body}
          onChange={(e) => setBody(e.target.value)}
          placeholder={t("communication.body_placeholder")}
        />
        <div className="flex flex-wrap items-center gap-1.5">
          <span className="text-muted-foreground text-xs">
            {t("communication.variables")}
          </span>
          {MESSAGE_VARIABLES.map((v) => (
            <Hint key={v} label={t(`communication.var_${v.slice(1)}`)}>
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="h-7 px-2 font-mono text-xs"
                onClick={() => insertVariable(v)}
              >
                {v}
              </Button>
            </Hint>
          ))}
        </div>
        <p className="text-muted-foreground text-xs">
          {t("communication.variables_hint")}
        </p>
        {channel === "sms" ? (
          <p
            className={
              length.parts > BULK_SMS_PART_LIMIT
                ? "text-danger text-sm font-medium"
                : "text-muted-foreground text-sm"
            }
            data-testid="sms-counter"
          >
            {t("communication.counter", {
              chars: length.chars,
              parts: length.parts,
              limit: BULK_SMS_PART_LIMIT,
            })}
          </p>
        ) : null}
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <Button
          onClick={openPreview}
          disabled={!ready || preview.isPending}
          data-testid="bulk-preview"
        >
          <Send className="size-4" />
          {t("communication.preview_send")}
        </Button>
        {!target ? (
          <span className="text-muted-foreground text-sm">
            {t("communication.pick_target_first")}
          </span>
        ) : null}
      </div>

      <ConfirmBulkDialog
        check={check}
        pending={start.isPending}
        onCancel={() => setCheck(null)}
        onConfirm={confirm}
      />
    </div>
  );
}

const SKIP_KEYS: Record<string, string> = {
  no_contact: "communication.skip_no_contact",
  invalid_phone: "communication.skip_invalid_phone",
  invalid_email: "communication.skip_invalid_email",
  campaign_words: "communication.skip_campaign",
};

function ConfirmBulkDialog({
  check,
  pending,
  onCancel,
  onConfirm,
}: {
  check: BulkSendPreview | null;
  pending: boolean;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  const { t } = useLocale();
  return (
    <Dialog
      open={Boolean(check)}
      onOpenChange={(o) => (!o ? onCancel() : null)}
    >
      <DialogContent className="max-h-[90vh] max-w-xl overflow-y-auto">
        {check ? (
          <>
            <DialogHeader>
              <DialogTitle data-testid="bulk-confirm-title">
                {t("communication.confirm_title", {
                  count: check.recipients,
                })}
              </DialogTitle>
              <DialogDescription>
                {check.target_label} ·{" "}
                {check.channel === "sms"
                  ? t("communication.channel_sms")
                  : t("communication.channel_mail")}
              </DialogDescription>
            </DialogHeader>
            <div className="flex flex-col gap-3 text-sm">
              {check.test_mode ? (
                <StatusPill tone="info">
                  {t("communication.test_mode")}
                </StatusPill>
              ) : null}
              {check.blocked ? (
                <p
                  className="text-danger flex items-start gap-2 font-medium"
                  data-testid="bulk-blocked"
                >
                  <TriangleAlert className="mt-0.5 size-4 shrink-0" />
                  {check.blocked}
                </p>
              ) : null}
              {check.too_long.length > 0 ? (
                <ul className="text-danger list-disc pl-5 text-xs">
                  {check.too_long.map((x) => (
                    <li key={x.title}>
                      {t("communication.too_long_item", {
                        title: x.title,
                        chars: x.chars,
                        parts: x.parts,
                        limit: check.part_limit,
                      })}
                    </li>
                  ))}
                </ul>
              ) : null}
              <p data-testid="bulk-duplicates">
                {check.duplicates.length > 0
                  ? t("communication.duplicates_dropped", {
                      count: check.duplicates.length,
                    })
                  : t("communication.no_duplicates")}
              </p>
              {check.duplicates.length > 0 ? (
                <Collapsible>
                  <CollapsibleTrigger asChild>
                    <Button variant="link" size="sm" className="h-auto p-0">
                      {t("communication.show_list")}
                    </Button>
                  </CollapsibleTrigger>
                  <CollapsibleContent>
                    <ul className="text-muted-foreground list-disc pl-5 text-xs">
                      {check.duplicates.map((d) => (
                        <li key={`${d.title}-${d.contact}`}>
                          {t("communication.duplicate_item", {
                            title: d.title,
                            same: d.same_as,
                            contact: d.contact,
                          })}
                        </li>
                      ))}
                    </ul>
                  </CollapsibleContent>
                </Collapsible>
              ) : null}
              {check.skipped.length > 0 ? (
                <div data-testid="bulk-skipped">
                  <p>
                    {t("communication.skipped_count", {
                      count: check.skipped.length,
                    })}
                  </p>
                  <ul className="text-muted-foreground list-disc pl-5 text-xs">
                    {check.skipped.map((s) => (
                      <li key={`${s.engagement_uuid}-${s.title}`}>
                        {s.title}: {t(SKIP_KEYS[s.reason] ?? s.reason)}
                      </li>
                    ))}
                  </ul>
                </div>
              ) : null}
              {check.sample ? (
                <div className="bg-surface-2 rounded-md border p-3">
                  <p className="text-muted-foreground mb-1 text-xs">
                    {t("communication.sample", {
                      title: check.sample.title,
                      to: check.sample.to,
                    })}
                  </p>
                  {check.sample.subject ? (
                    <p className="font-medium">{check.sample.subject}</p>
                  ) : null}
                  <p className="whitespace-pre-wrap">{check.sample.text}</p>
                  {check.channel === "sms" ? (
                    <p className="text-muted-foreground mt-1 text-xs">
                      {t("communication.counter", {
                        chars: check.sample.chars,
                        parts: check.sample.parts,
                        limit: check.part_limit,
                      })}
                    </p>
                  ) : null}
                </div>
              ) : null}
            </div>
            <DialogFooter>
              <Button variant="outline" onClick={onCancel}>
                {t("common.cancel")}
              </Button>
              <Button
                onClick={onConfirm}
                disabled={Boolean(check.blocked) || pending}
                data-testid="bulk-confirm"
              >
                <Send className="size-4" />
                {t("communication.confirm_send", { count: check.recipients })}
              </Button>
            </DialogFooter>
          </>
        ) : null}
      </DialogContent>
    </Dialog>
  );
}
