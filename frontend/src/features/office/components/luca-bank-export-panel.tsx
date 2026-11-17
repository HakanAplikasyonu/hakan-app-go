"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Download, Loader2 } from "lucide-react";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import { DatePicker } from "@/components/ui/date-picker";
import { Skeleton } from "@/components/ui/skeleton";
import { apiConfig } from "@/config/api";
import type { components } from "@/generated/api";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { isApiError, unwrap } from "@/lib/api";
import { ORGANIZATION_HEADER, tenantRequest } from "@/lib/api/tenant-request";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

import { Figure, FilterSelect, Panel } from "./office-ui";

type CashBox = components["schemas"]["CashBox"];
type LucaExport = components["schemas"]["LucaBankStatementExport"];
type Guide = components["schemas"]["EnvelopeLucaBankStatementGuide"]["data"];
type Layout = "signed" | "two_columns";
type Delimiter = ";" | ",";

const eng = (engagement: string) =>
  `/v1/tenant/engagements/${encodeURIComponent(engagement)}`;

/** First and last day of a YYYY-MM period (this month when none). */
function monthRange(period?: string | null) {
  const p =
    period && /^\d{4}-\d{2}$/.test(period)
      ? period
      : new Date().toISOString().slice(0, 7);
  const [y, m] = p.split("-").map(Number);
  const last = new Date(Date.UTC(y, m, 0)).getUTCDate();
  return { from: `${p}-01`, to: `${p}-${String(last).padStart(2, "0")}` };
}

async function downloadStatement(
  slug: string,
  engagement: string,
  body: Record<string, string>,
) {
  const root = apiConfig.baseUrl.replace(/\/$/, "");
  const response = await fetch(
    `${root}${eng(engagement)}/bank-statement-exports`,
    {
      method: "POST",
      credentials: "include",
      headers: {
        Accept: "text/csv, application/json",
        "Content-Type": "application/json",
        [ORGANIZATION_HEADER]: slug,
      },
      body: JSON.stringify(body),
    },
  );
  if (!response.ok) {
    const payload = await response.json().catch(() => undefined);
    return unwrap<never>({ data: payload, response });
  }
  const name =
    /filename="([^"]+)"/.exec(
      response.headers.get("Content-Disposition") ?? "",
    )?.[1] ?? "luca_banka.csv";
  const repeated = Number(response.headers.get("X-Repeated-Count") ?? "0");
  const blob = await response.blob();
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = name;
  document.body.appendChild(link);
  link.click();
  link.remove();
  setTimeout(() => URL.revokeObjectURL(url), 10_000);
  return { name, repeated };
}

/**
 * Luca "Banka Ekstreleri" CSV (#465): hesap ve tarih aralığı seçilir, dosya
 * Windows-1254 olarak iner; Luca'daki ekstre tanımı için adım adım eşleme
 * rehberi ve indirme geçmişi. Biçim Luca'da doğrulanmadı.
 */
export function LucaBankExportPanel({
  engagement,
  period,
}: {
  engagement: string;
  period?: string | null;
}) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const qc = useQueryClient();
  const range = monthRange(period);
  const [box, setBox] = useState("");
  const [from, setFrom] = useState(range.from);
  const [to, setTo] = useState(range.to);
  const [layout, setLayout] = useState<Layout>("signed");
  const [delimiter, setDelimiter] = useState<Delimiter>(";");

  const boxes = useQuery({
    queryKey: ["tenant", slug.toLowerCase(), "cash-boxes", engagement],
    queryFn: () =>
      tenantRequest<{ items: CashBox[] }>(
        slug,
        "GET",
        `${eng(engagement)}/cash-boxes`,
      ),
  });
  const guide = useQuery({
    queryKey: ["tenant", slug.toLowerCase(), "luca-bank-guide", engagement],
    queryFn: () =>
      tenantRequest<Guide>(
        slug,
        "GET",
        `${eng(engagement)}/bank-statement-exports/guide`,
      ),
    staleTime: 10 * 60_000,
  });
  const historyKey = [
    "tenant",
    slug.toLowerCase(),
    "luca-bank-exports",
    engagement,
  ];
  const history = useQuery({
    queryKey: historyKey,
    queryFn: () =>
      tenantRequest<{ items: LucaExport[] }>(
        slug,
        "GET",
        `${eng(engagement)}/bank-statement-exports`,
      ),
  });
  const accounts = (boxes.data?.items ?? []).filter(
    (b) => b.type === "bank_account",
  );
  const selected = box || accounts[0]?.uuid || "";
  const download = useMutation({
    mutationFn: () =>
      downloadStatement(slug, engagement, {
        cash_box_uuid: selected,
        date_from: from,
        date_to: to,
        amount_layout: layout,
        delimiter,
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: historyKey });
      void qc.invalidateQueries({
        queryKey: [
          "tenant",
          slug.toLowerCase(),
          "bank-transactions",
          engagement,
        ],
      });
    },
  });
  const errorText = (err: unknown) => {
    if (isApiError(err)) {
      const key = `office.bank.luca.error_${err.code}`;
      const msg = err.code ? t(key) : key;
      return msg !== key ? msg : err.message || t("office.api.load_error");
    }
    return t("office.api.load_error");
  };
  const columns = guide.data?.columns?.[layout] ?? [];

  return (
    <div className="space-y-4">
      <Panel
        title={t("office.bank.luca.title")}
        description={t("office.bank.luca.description")}
      >
        <div className="space-y-3 p-4 text-sm">
          <p className="bg-status-warning/12 text-status-warning rounded-md px-3 py-2 text-xs">
            {t("office.bank.luca.unverified")}
          </p>
          {boxes.isLoading ? (
            <Skeleton className="h-10" />
          ) : accounts.length === 0 ? (
            <p className="text-muted-foreground text-xs">
              {t("office.bank.luca.no_account")}
            </p>
          ) : (
            <div className="flex flex-wrap items-end gap-3">
              <FilterSelect
                label={t("office.bank.luca.account")}
                value={selected}
                onChange={setBox}
                options={accounts.map((b) => ({
                  value: b.uuid,
                  label: b.name || b.iban || b.uuid,
                }))}
              />
              <label className="flex items-center gap-2 text-xs">
                <span className="text-muted-foreground">
                  {t("office.bank.luca.from")}
                </span>
                <DatePicker
                  className="h-8 w-40"
                  value={from}
                  onChange={(v) => setFrom(v)}
                />
              </label>
              <label className="flex items-center gap-2 text-xs">
                <span className="text-muted-foreground">
                  {t("office.bank.luca.to")}
                </span>
                <DatePicker
                  className="h-8 w-40"
                  value={to}
                  onChange={(v) => setTo(v)}
                />
              </label>
              <FilterSelect<Layout>
                label={t("office.bank.luca.layout")}
                value={layout}
                onChange={setLayout}
                options={[
                  {
                    value: "signed",
                    label: t("office.bank.luca.layout_signed"),
                  },
                  {
                    value: "two_columns",
                    label: t("office.bank.luca.layout_two_columns"),
                  },
                ]}
              />
              <FilterSelect<Delimiter>
                label={t("office.bank.luca.delimiter")}
                value={delimiter}
                onChange={setDelimiter}
                options={[
                  {
                    value: ";",
                    label: t("office.bank.luca.delimiter_semicolon"),
                  },
                  { value: ",", label: t("office.bank.luca.delimiter_comma") },
                ]}
              />
              <Button
                size="sm"
                disabled={!selected || !from || !to || download.isPending}
                onClick={() => download.mutate()}
              >
                {download.isPending ? (
                  <Loader2 className="size-4 animate-spin" />
                ) : (
                  <Download className="size-4" />
                )}
                {t("office.bank.luca.download")}
              </Button>
            </div>
          )}
          {download.data && download.data.repeated > 0 ? (
            <p className="text-status-warning text-xs">
              {t("office.bank.luca.repeated", {
                count: download.data.repeated,
              })}
            </p>
          ) : download.data ? (
            <p className="text-status-approved text-xs">
              {t("office.bank.luca.downloaded", { file: download.data.name })}
            </p>
          ) : null}
          {download.error ? (
            <p className="text-status-blocking text-xs">
              {errorText(download.error)}
            </p>
          ) : null}
        </div>
      </Panel>

      <Panel
        title={t("office.bank.luca.guide_title")}
        description={t("office.bank.luca.guide_description")}
      >
        <div className="space-y-3 p-4 text-sm">
          <ol className="list-decimal space-y-1 ps-5 text-xs">
            <li>{t("office.bank.luca.step_open")}</li>
            <li>{t("office.bank.luca.step_header")}</li>
            <li>
              {t("office.bank.luca.step_format", {
                delimiter:
                  delimiter === ";"
                    ? t("office.bank.luca.delimiter_semicolon")
                    : t("office.bank.luca.delimiter_comma"),
                quote: guide.data?.quote || '"',
              })}
            </li>
            <li>{t("office.bank.luca.step_columns")}</li>
            <li>{t("office.bank.luca.step_save")}</li>
          </ol>
          {guide.isLoading ? (
            <Skeleton className="h-24" />
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-xs">
                <thead>
                  <tr className="text-muted-foreground text-left">
                    <th className="py-1 pe-3 font-medium">#</th>
                    <th className="py-1 pe-3 font-medium">
                      {t("office.bank.luca.col_header")}
                    </th>
                    <th className="py-1 pe-3 font-medium">
                      {t("office.bank.luca.col_field")}
                    </th>
                    <th className="py-1 font-medium">
                      {t("office.bank.luca.col_type")}
                    </th>
                  </tr>
                </thead>
                <tbody className="divide-y">
                  {columns.map((c, i) => (
                    <tr key={c.header}>
                      <td className="py-1 pe-3 tabular-nums">{i + 1}</td>
                      <td className="py-1 pe-3">
                        <Figure>{c.header}</Figure>
                      </td>
                      <td className="py-1 pe-3">{c.program_field}</td>
                      <td className="py-1">{c.data_type}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
          <p className="text-muted-foreground text-xs">
            {layout === "signed"
              ? t("office.bank.luca.sign_signed")
              : t("office.bank.luca.sign_two_columns")}
          </p>
        </div>
      </Panel>

      <Panel title={t("office.bank.luca.history")}>
        <div className="p-4 text-xs">
          {history.isLoading ? (
            <Skeleton className="h-12" />
          ) : (history.data?.items ?? []).length === 0 ? (
            <p className="text-muted-foreground">
              {t("office.bank.luca.history_empty")}
            </p>
          ) : (
            <ul className="divide-y">
              {(history.data?.items ?? []).map((h) => (
                <li key={h.uuid} className="space-y-0.5 py-1.5">
                  <span className="block">
                    <Figure className="font-medium">{h.file_name}</Figure>
                  </span>
                  <span className="text-muted-foreground block">
                    {t("office.bank.luca.history_row", {
                      account: h.cash_box_name,
                      date: datetime(h.created_at, undefined, locale),
                      count: h.line_count,
                    })}
                    {h.repeated_count > 0 ? (
                      <span className="text-status-warning">
                        {" · "}
                        {t("office.bank.luca.history_repeated", {
                          count: h.repeated_count,
                        })}
                      </span>
                    ) : null}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </div>
      </Panel>
    </div>
  );
}
