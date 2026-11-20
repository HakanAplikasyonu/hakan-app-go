"use client";

import { Skeleton } from "@/components/ui/skeleton";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetTitle,
} from "@/components/ui/sheet";
import {
  Table,
  TableBody,
  TableCell,
  TableFooter,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

import { useJournalDraft } from "../hooks/use-journals";
import { sub, sum, toMinor } from "../lib/decimal";
import type { JournalDraft } from "../services/journals.service";
import { Figure, Money } from "./office-ui";

/** Tutar sıfırsa hücre boş kalır: kalem yalnız bir tarafta görünür. */
function Side({ value }: { value: string }) {
  return toMinor(value) === BigInt(0) ? null : <Money value={value} />;
}

/**
 * Fişin kalemleri: hesap kodu ve adı, Borç / Alacak ayrı sütunlarda (öbür
 * taraf boş) ve toplam satırı. Listede yalnız fişin borç ve alacak
 * toplamları görünür; dengeli fişte ikisi hep eşittir, hangi hesabın borçlu
 * hangisinin alacaklı olduğu burada okunur.
 */
export function JournalDraftLines({
  engagement,
  draft,
  onClose,
}: {
  engagement: string;
  draft: JournalDraft | null;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const detail = useJournalDraft(slug, engagement, draft?.uuid ?? "");
  const items = detail.data?.items ?? [];
  const debit = sum(items.map((l) => l.debit));
  const credit = sum(items.map((l) => l.credit));
  const diff = sub(debit, credit);
  const entry = draft ? `${draft.period} / ${draft.entry_no}` : "";
  return (
    <Sheet open={!!draft} onOpenChange={(open) => !open && onClose()}>
      <SheetContent
        side="right"
        className="flex w-full flex-col gap-3 overflow-y-auto p-4 sm:max-w-3xl"
      >
        <SheetTitle className="font-display text-lg font-bold">
          {t("office.journals.lines.title", { entry })}
        </SheetTitle>
        <SheetDescription className="text-text-2 text-sm">
          {draft?.description || t("office.journals.lines.hint")}
        </SheetDescription>
        {detail.isLoading ? (
          <Skeleton className="h-32 w-full" />
        ) : detail.isError ? (
          <p className="text-status-blocking text-sm">
            {t("office.api.load_error")}
          </p>
        ) : (
          <>
            <div
              className="bg-card overflow-x-auto rounded-xl border"
              role="region"
              tabIndex={0}
              aria-label={t("office.journals.lines.table")}
            >
              <Table
                aria-label={t("office.journals.lines.table")}
                containerClassName="overflow-x-visible"
              >
                <TableHeader>
                  <TableRow>
                    <TableHead>{t("office.journals.lines.col_code")}</TableHead>
                    <TableHead>{t("office.journals.lines.col_name")}</TableHead>
                    <TableHead>
                      {t("office.journals.lines.col_description")}
                    </TableHead>
                    <TableHead className="text-end">
                      {t("office.journals.debit")}
                    </TableHead>
                    <TableHead className="text-end">
                      {t("office.journals.credit")}
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {items.map((l) => (
                    <TableRow key={l.line_no} className="border-t">
                      <TableCell className="px-3 py-1.5 font-mono whitespace-nowrap">
                        {l.account}
                      </TableCell>
                      <TableCell className="px-3 py-1.5">
                        {l.account_name || "—"}
                      </TableCell>
                      <TableCell className="text-text-2 max-w-56 truncate px-3 py-1.5 text-xs">
                        {l.description}
                      </TableCell>
                      <TableCell className="px-3 py-1.5 text-end">
                        <Side value={l.debit} />
                      </TableCell>
                      <TableCell className="px-3 py-1.5 text-end">
                        <Side value={l.credit} />
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
                <TableFooter>
                  <TableRow>
                    <TableCell className="px-3 py-2" colSpan={3}>
                      {t("office.journals.lines.total")}
                    </TableCell>
                    <TableCell className="px-3 py-2 text-end">
                      <Money value={debit} />
                    </TableCell>
                    <TableCell className="px-3 py-2 text-end">
                      <Money value={credit} />
                    </TableCell>
                  </TableRow>
                </TableFooter>
              </Table>
            </div>
            <p
              className={
                toMinor(diff) === BigInt(0)
                  ? "text-text-2 text-xs"
                  : "text-status-blocking text-xs"
              }
            >
              {toMinor(diff) === BigInt(0) ? (
                t("office.journals.lines.balanced")
              ) : (
                <>
                  {t("office.journals.lines.unbalanced")}{" "}
                  <Figure>{diff}</Figure>
                </>
              )}
            </p>
          </>
        )}
      </SheetContent>
    </Sheet>
  );
}
