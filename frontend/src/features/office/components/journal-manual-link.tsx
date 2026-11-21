"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link2, Unlink } from "lucide-react";
import { useId, useMemo, useState } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { FilterChip } from "@/components/ui/filter-chip";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";

import {
  journalLinksService,
  type ManualLinkKind,
} from "../services/journal-links.service";
import { journalKeys, journalService } from "../services/journals.service";
import { Money } from "./office-ui";

const KINDS: ManualLinkKind[] = ["invoice", "bank_transaction", "customer"];
const SHOWN = 8;

function useLinkMutation(engagement: string, draft: string) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: { kind: ManualLinkKind; uuid: string; on: boolean }) =>
      journalLinksService.set(slug, engagement, draft, v.kind, v.uuid, v.on),
    onSuccess: (_, v) => {
      // İki yönün ilişkili kayıtları da değişir.
      void qc.invalidateQueries({
        queryKey: ["tenant", slug.toLowerCase(), "related"],
      });
      toast.success(
        t(v.on ? "office.related.link_done" : "office.related.unlink_done"),
      );
    },
    onError: (e) =>
      toast.error(
        isApiError(e) && e.message
          ? e.message
          : t("office.related.link_failed"),
      ),
  });
}

/**
 * Fişin ilişkili kayıtlarında elle bağlanan kaydın bağını kaldırır (#710).
 */
export function ManualUnlinkButton({
  engagement,
  draft,
  kind,
  uuid,
  label,
}: {
  engagement: string;
  draft: string;
  kind: ManualLinkKind;
  uuid: string;
  label: string;
}) {
  const { t } = useLocale();
  const unlink = useLinkMutation(engagement, draft);
  return (
    <Button
      type="button"
      variant="ghost"
      size="sm"
      className="h-7 px-2"
      disabled={unlink.isPending}
      aria-label={t("office.related.unlink_record", { record: label })}
      onClick={() => unlink.mutate({ kind, uuid, on: false })}
    >
      <Unlink className="size-3.5" aria-hidden />
      {t("office.related.unlink")}
    </Button>
  );
}

/**
 * Fişe elle kayıt bağlama (#710): "bu kayda bağlı başka kayıt yok" diyen
 * fişe kullanıcı faturayı ya da banka hareketini kendisi bağlar. Adaylar
 * fişin dönemindeki kayıtlardır; numara, açıklama, karşı taraf ya da tutarla
 * aranır. Bağ fişin satırlarını ve aktarımı değiştirmez.
 */
export function ManualLinkForm({
  engagement,
  draft,
}: {
  engagement: string;
  draft: string;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const [open, setOpen] = useState(false);
  const [kind, setKind] = useState<ManualLinkKind>("invoice");
  const [text, setText] = useState("");
  const searchId = useId();
  const link = useLinkMutation(engagement, draft);

  const detail = useQuery({
    queryKey: journalKeys.draft(slug, engagement, draft),
    queryFn: () => journalService.draft(slug, engagement, draft),
    enabled: open,
  });
  const period = detail.data?.period ?? "";
  const candidates = useQuery({
    queryKey: [
      ...journalKeys.all(slug, engagement),
      "link-candidates",
      kind,
      period,
    ],
    queryFn: () =>
      journalLinksService.candidates(slug, engagement, kind, period),
    // Cari kartlarının dönemi yok; fatura ve banka fişin ayından gelir.
    enabled: open && (kind === "customer" || !!period),
  });
  const shown = useMemo(() => {
    const q = text.trim().toLocaleLowerCase("tr");
    const all = candidates.data ?? [];
    const hit = q
      ? all.filter((c) =>
          [c.label, c.detail, c.amount]
            .join(" ")
            .toLocaleLowerCase("tr")
            .includes(q),
        )
      : all;
    return hit.slice(0, SHOWN);
  }, [candidates.data, text]);

  if (!open)
    return (
      <div className="px-4 pb-4">
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={() => setOpen(true)}
        >
          <Link2 className="size-4" aria-hidden />
          {t("office.related.link_open")}
        </Button>
      </div>
    );

  return (
    <section
      aria-label={t("office.related.link_title")}
      className="border-line space-y-3 border-t px-4 py-3"
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h3 className="text-sm font-semibold">
          {t("office.related.link_title")}
        </h3>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          onClick={() => setOpen(false)}
        >
          {t("office.related.link_close")}
        </Button>
      </div>
      <p className="text-text-2 text-xs">{t("office.related.link_hint")}</p>
      <div
        role="group"
        aria-label={t("office.related.link_kind")}
        className="flex flex-wrap gap-2"
      >
        {KINDS.map((k) => (
          <FilterChip
            key={k}
            pressed={kind === k}
            onPressedChange={() => setKind(k)}
          >
            {t(`office.related.kind.${k}`)}
          </FilterChip>
        ))}
      </div>
      <div className="space-y-1">
        <Label htmlFor={searchId}>{t("office.related.link_search")}</Label>
        <Input
          id={searchId}
          value={text}
          onChange={(e) => setText(e.target.value)}
          placeholder={t("office.related.link_search_hint")}
        />
      </div>
      {candidates.isLoading || detail.isLoading ? (
        <p className="text-text-2 text-sm">
          {t("office.related.link_loading")}
        </p>
      ) : candidates.isError || detail.isError ? (
        <p role="alert" className="text-danger text-sm">
          {t("office.related.link_load_error")}
        </p>
      ) : shown.length === 0 ? (
        <p className="text-text-2 text-sm" data-testid="link-candidates-empty">
          {t("office.related.link_none")}
        </p>
      ) : (
        <ul className="space-y-1" data-testid="link-candidates">
          {shown.map((c) => (
            <li
              key={c.uuid}
              className="flex flex-wrap items-center justify-between gap-2 text-sm"
            >
              <span className="min-w-0">
                <span className="font-medium">{c.label || "—"}</span>
                {c.detail ? (
                  <span className="text-text-2"> · {c.detail}</span>
                ) : null}
                {c.cardType ? (
                  <span className="text-text-2">
                    {" · "}
                    {t(`office.related.status.${c.cardType}`)}
                  </span>
                ) : null}
                {c.amount ? (
                  <>
                    {" · "}
                    <Money value={c.amount} currency={c.currency} />
                  </>
                ) : null}
              </span>
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={link.isPending}
                aria-label={t("office.related.link_record", {
                  record: c.label || c.uuid,
                })}
                onClick={() => link.mutate({ kind, uuid: c.uuid, on: true })}
              >
                {t("office.related.link")}
              </Button>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
