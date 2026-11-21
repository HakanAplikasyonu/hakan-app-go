"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Download, Eye, Star, Trash2, Upload } from "lucide-react";
import { useRouter, useSearchParams } from "next/navigation";
import { Suspense, useState } from "react";
import { toast } from "sonner";

import { EmptyState } from "@/components/common/empty-state";
import { StatusPill, type StatusTone } from "@/components/tables";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { DatePicker } from "@/components/ui/date-picker";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { FilterChip } from "@/components/ui/filter-chip";
import { Hint } from "@/components/ui/hint";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  NativeSelect,
  NativeSelectOption,
} from "@/components/ui/native-select";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Textarea } from "@/components/ui/textarea";
import { permissions } from "@/config/permissions";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import {
  ARCHIVE_CATEGORIES,
  ARCHIVE_IMPORTANT_KINDS,
  ARCHIVE_ISG_KINDS,
  archiveKeys,
  archiveService,
  type ArchiveCategory,
  type ArchiveDocument,
  type ArchiveImportantKind,
  type ArchiveIsgKind,
  type ArchiveQuery,
} from "../services/archive-documents.service";
import { Panel } from "./office-ui";

const PAGE = 50;

const expiryTone: Record<ArchiveDocument["expiry_status"], StatusTone> = {
  none: "neutral",
  valid: "ok",
  expiring: "warn",
  expired: "danger",
};

/** 1536 → "1,5 KB". */
export function fileSize(bytes: number, locale: string) {
  const units = ["B", "KB", "MB", "GB"];
  let v = bytes;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  const n = v.toLocaleString(locale === "tr" ? "tr-TR" : "en-GB", {
    maximumFractionDigits: i === 0 ? 0 : 1,
  });
  return `${n} ${units[i]}`;
}

function formatDay(value: string | null, locale: string) {
  if (!value) return "";
  const d = new Date(`${value}T00:00:00`);
  return Number.isNaN(d.getTime())
    ? value
    : d.toLocaleDateString(locale === "tr" ? "tr-TR" : "en-GB");
}

/**
 * Mükellefin belge arşivi (#669 I): faaliyet belgesi, ticaret sicil,
 * vergi levhası, sözleşmeler ve İSG belgeleri. Yükle, gör, indir, sil;
 * kategori, arama ve süresi yaklaşan süzgeci adreste.
 */
export function EngagementArchivePage({ engagement }: { engagement: string }) {
  return (
    <Suspense fallback={<Skeleton className="h-64 w-full" />}>
      <ArchiveInner engagement={engagement} />
    </Suspense>
  );
}

function ArchiveInner({ engagement }: { engagement: string }) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const qc = useQueryClient();
  const router = useRouter();
  const search = useSearchParams();
  const [uploading, setUploading] = useState(false);
  const [removing, setRemoving] = useState<ArchiveDocument | null>(null);
  const [marking, setMarking] = useState<ArchiveDocument | null>(null);
  const [q, setQ] = useState(search.get("q") ?? "");

  const rawCategory = search.get("kategori");
  const category = ARCHIVE_CATEGORIES.includes(rawCategory as ArchiveCategory)
    ? (rawCategory as ArchiveCategory)
    : undefined;
  const expiring = search.get("yaklasan") === "1";
  const offset = Math.max(0, Number(search.get("sayfa") ?? 0) || 0) * PAGE;
  const query: ArchiveQuery = {
    category,
    q: search.get("q") ?? undefined,
    expiring,
    limit: PAGE,
    offset,
  };
  const setParams = (values: Record<string, string | null>) => {
    const next = new URLSearchParams(search.toString());
    for (const [k, v] of Object.entries(values)) {
      if (v === null || v === "") next.delete(k);
      else next.set(k, v);
    }
    if (!("sayfa" in values)) next.delete("sayfa");
    router.replace(`?${next.toString()}`, { scroll: false });
  };

  const list = useQuery({
    queryKey: archiveKeys.list(slug, engagement, query),
    queryFn: () => archiveService.list(slug, engagement, query),
  });
  const page = list.data;
  const items = page?.items ?? [];
  const canWrite = can(permissions.tenant.documents.write);

  const remove = useMutation({
    mutationFn: (d: ArchiveDocument) =>
      archiveService.remove(slug, engagement, d.uuid),
    onSuccess: () => {
      void qc.invalidateQueries({
        queryKey: archiveKeys.all(slug, engagement),
      });
      toast.success(t("office.archive.deleted"));
    },
    onError: (err) => toast.error(err.message || t("office.archive.error")),
    onSettled: () => setRemoving(null),
  });

  // Önemli işaretini kaldırma (#780 B); işaretleme tür seçtiren pencerede.
  const unmark = useMutation({
    mutationFn: (d: ArchiveDocument) =>
      archiveService.update(slug, engagement, d.uuid, { important_kind: "" }),
    onSuccess: () => {
      void qc.invalidateQueries({
        queryKey: archiveKeys.all(slug, engagement),
      });
      toast.success(t("office.important.unmarked"));
    },
    onError: (err) => toast.error(err.message || t("office.archive.error")),
  });

  const act = (fn: () => Promise<unknown>) =>
    void fn().catch((err: Error) =>
      toast.error(err?.message || t("office.archive.file_error")),
    );

  const counts = page?.counts;
  return (
    <div className="space-y-4">
      <Panel
        title={t("office.archive.title")}
        description={t("office.archive.description")}
        actions={
          canWrite ? (
            <Button size="sm" onClick={() => setUploading(true)}>
              <Upload className="size-4" aria-hidden />
              {t("office.archive.upload")}
            </Button>
          ) : null
        }
      >
        <div className="space-y-3 p-4">
          <div
            role="group"
            aria-label={t("office.archive.categories")}
            className="flex flex-wrap gap-2"
          >
            <FilterChip
              pressed={!category}
              onPressedChange={() => setParams({ kategori: null })}
            >
              {t("office.archive.category_all")}
              {counts ? ` (${counts.total})` : ""}
            </FilterChip>
            {ARCHIVE_CATEGORIES.map((c) => (
              <FilterChip
                key={c}
                pressed={category === c}
                onPressedChange={() => setParams({ kategori: c })}
              >
                {t(`office.archive.category_${c}`)}
              </FilterChip>
            ))}
          </div>
          <div className="flex flex-wrap items-end gap-4">
            <form
              className="space-y-1"
              onSubmit={(e) => {
                e.preventDefault();
                setParams({ q: q.trim() || null });
              }}
            >
              <Label htmlFor="archive-search">
                {t("office.archive.search")}
              </Label>
              <Input
                id="archive-search"
                type="search"
                className="h-9 w-64"
                value={q}
                maxLength={100}
                onChange={(e) => setQ(e.target.value)}
                onBlur={() => setParams({ q: q.trim() || null })}
              />
            </form>
            <Label className="flex h-9 w-fit items-center gap-2 text-sm leading-normal font-normal">
              <Checkbox
                checked={expiring}
                onCheckedChange={(v) =>
                  setParams({ yaklasan: v === true ? "1" : null })
                }
              />
              {t("office.archive.expiring", {
                days: page?.expiring_days ?? 30,
              })}
            </Label>
            {counts && counts.expired + counts.expiring > 0 ? (
              <span className="text-status-warning text-sm">
                {t("office.archive.expiry_summary", {
                  expired: counts.expired,
                  expiring: counts.expiring,
                })}
              </span>
            ) : null}
          </div>
        </div>
      </Panel>

      {list.isLoading ? (
        <Skeleton className="h-64 w-full" />
      ) : list.isError || !page ? (
        <p className="text-danger text-sm">{t("office.api.load_error")}</p>
      ) : items.length === 0 ? (
        <EmptyState
          title={t("office.archive.empty_title")}
          description={t(
            category || expiring || query.q
              ? "office.archive.empty_filtered"
              : "office.archive.empty_body",
          )}
        />
      ) : (
        <div
          className="bg-card overflow-x-auto rounded-xl border"
          role="region"
          tabIndex={0}
          aria-label={t("office.archive.table")}
        >
          <Table
            aria-label={t("office.archive.table")}
            containerClassName="overflow-x-visible"
          >
            <TableHeader>
              <TableRow>
                <TableHead>{t("office.archive.col_title")}</TableHead>
                <TableHead>{t("office.archive.col_category")}</TableHead>
                <TableHead>{t("office.archive.col_date")}</TableHead>
                <TableHead>{t("office.archive.col_valid_until")}</TableHead>
                <TableHead>{t("office.archive.col_file")}</TableHead>
                <TableHead>{t("office.archive.col_visible")}</TableHead>
                <TableHead className="text-end">
                  {t("office.archive.col_actions")}
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {items.map((d) => (
                <TableRow key={d.uuid} className="border-t">
                  <TableCell className="px-3 py-2">
                    <span className="font-medium">{d.title}</span>
                    {d.important_kind ? (
                      <span className="text-brand-text block text-xs font-semibold">
                        {t("office.important.badge", {
                          kind: t(`office.important.kind_${d.important_kind}`),
                        })}
                      </span>
                    ) : null}
                    {d.note ? (
                      <span className="text-muted-foreground block text-xs">
                        {d.note}
                      </span>
                    ) : null}
                  </TableCell>
                  <TableCell className="px-3 py-2 text-sm">
                    {t(`office.archive.category_${d.category}`)}
                    {d.isg_kind ? (
                      <span className="text-muted-foreground block text-xs">
                        {t(`office.archive.isg_${d.isg_kind}`)}
                      </span>
                    ) : null}
                  </TableCell>
                  <TableCell className="px-3 py-2 text-sm whitespace-nowrap">
                    {formatDay(d.document_date, locale)}
                  </TableCell>
                  <TableCell className="px-3 py-2 text-sm whitespace-nowrap">
                    {d.valid_until ? (
                      <span className="flex flex-col items-start gap-1">
                        {formatDay(d.valid_until, locale)}
                        <StatusPill tone={expiryTone[d.expiry_status]}>
                          {d.expiry_status === "expired"
                            ? t("office.archive.expired", {
                                count: -(d.days_left ?? 0),
                              })
                            : d.expiry_status === "expiring"
                              ? t("office.archive.expiring_in", {
                                  count: d.days_left ?? 0,
                                })
                              : t("office.archive.valid")}
                        </StatusPill>
                      </span>
                    ) : null}
                  </TableCell>
                  <TableCell className="px-3 py-2 text-sm">
                    <span className="block max-w-56 truncate">
                      {d.file_name}
                    </span>
                    <span className="text-muted-foreground text-xs">
                      {fileSize(d.size_bytes, locale)}
                    </span>
                  </TableCell>
                  <TableCell className="px-3 py-2 text-sm">
                    {d.visible_to_taxpayer
                      ? t("office.archive.visible_yes")
                      : t("office.archive.visible_no")}
                  </TableCell>
                  <TableCell className="px-3 py-2">
                    <span className="flex justify-end gap-1">
                      <Hint label={t("office.archive.view_short")}>
                        <Button
                          size="icon"
                          variant="ghost"
                          aria-label={t("office.archive.view", {
                            title: d.title,
                          })}
                          onClick={() =>
                            act(() => archiveService.view(slug, engagement, d))
                          }
                        >
                          <Eye className="size-4" aria-hidden />
                        </Button>
                      </Hint>
                      <Hint label={t("office.archive.download_short")}>
                        <Button
                          size="icon"
                          variant="ghost"
                          aria-label={t("office.archive.download", {
                            title: d.title,
                          })}
                          onClick={() =>
                            act(() =>
                              archiveService.download(slug, engagement, d),
                            )
                          }
                        >
                          <Download className="size-4" aria-hidden />
                        </Button>
                      </Hint>
                      {canWrite ? (
                        <Hint
                          label={
                            d.important_kind
                              ? t("office.important.unmark_short")
                              : t("office.important.mark_short")
                          }
                        >
                          <Button
                            size="icon"
                            variant="ghost"
                            aria-label={
                              d.important_kind
                                ? t("office.important.unmark", {
                                    title: d.title,
                                  })
                                : t("office.important.mark", {
                                    title: d.title,
                                  })
                            }
                            aria-pressed={!!d.important_kind}
                            disabled={unmark.isPending}
                            onClick={() =>
                              d.important_kind
                                ? unmark.mutate(d)
                                : setMarking(d)
                            }
                          >
                            <Star
                              className={
                                d.important_kind
                                  ? "fill-warn text-warn size-4"
                                  : "size-4"
                              }
                              aria-hidden
                            />
                          </Button>
                        </Hint>
                      ) : null}
                      {canWrite ? (
                        <Hint label={t("office.archive.delete_short")}>
                          <Button
                            size="icon"
                            variant="ghost"
                            aria-label={t("office.archive.delete", {
                              title: d.title,
                            })}
                            onClick={() => setRemoving(d)}
                          >
                            <Trash2 className="size-4" aria-hidden />
                          </Button>
                        </Hint>
                      ) : null}
                    </span>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
          {page.total > PAGE ? (
            <div className="flex items-center justify-end gap-2 border-t p-2 text-sm">
              <span className="text-muted-foreground">
                {t("office.archive.page", {
                  from: offset + 1,
                  to: offset + items.length,
                  total: page.total,
                })}
              </span>
              <Button
                size="sm"
                variant="outline"
                disabled={offset === 0}
                onClick={() =>
                  setParams({ sayfa: String(offset / PAGE - 1 || "") })
                }
              >
                {t("office.archive.prev")}
              </Button>
              <Button
                size="sm"
                variant="outline"
                disabled={offset + items.length >= page.total}
                onClick={() => setParams({ sayfa: String(offset / PAGE + 1) })}
              >
                {t("office.archive.next")}
              </Button>
            </div>
          ) : null}
        </div>
      )}

      {uploading ? (
        <UploadDialog
          engagement={engagement}
          defaultCategory={category ?? "general"}
          onClose={() => setUploading(false)}
        />
      ) : null}

      {marking ? (
        <MarkImportantDialog
          engagement={engagement}
          doc={marking}
          onClose={() => setMarking(null)}
        />
      ) : null}

      <AlertDialog
        open={!!removing}
        onOpenChange={(open) => {
          if (!open && !remove.isPending) setRemoving(null);
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t("office.archive.delete_title")}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t("office.archive.delete_body", {
                title: removing?.title ?? "",
              })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={remove.isPending}>
              {t("office.archive.cancel")}
            </AlertDialogCancel>
            <AlertDialogAction
              disabled={remove.isPending}
              onClick={(e) => {
                e.preventDefault();
                if (removing) remove.mutate(removing);
              }}
            >
              {t("office.archive.delete_confirm")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

/**
 * Arşivdeki belgeyi "önemli" işaretler (#780 B): tür seçilir; mükellef
 * sayfasındaki "Önemli belgeler" bölümüne düşer. "Mükellef de görsün"
 * buradan açılabilir.
 */
function MarkImportantDialog({
  engagement,
  doc,
  onClose,
}: {
  engagement: string;
  doc: ArchiveDocument;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const qc = useQueryClient();
  const [kind, setKind] = useState<ArchiveImportantKind>("other");
  const [visible, setVisible] = useState(doc.visible_to_taxpayer);
  const save = useMutation({
    mutationFn: () =>
      archiveService.update(slug, engagement, doc.uuid, {
        important_kind: kind,
        visible_to_taxpayer: visible,
      }),
    onSuccess: () => {
      void qc.invalidateQueries({
        queryKey: archiveKeys.all(slug, engagement),
      });
      toast.success(t("office.important.marked"));
      onClose();
    },
    onError: (err) => toast.error(err.message || t("office.archive.error")),
  });
  return (
    <Dialog open onOpenChange={(open) => !open && !save.isPending && onClose()}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t("office.important.mark_title")}</DialogTitle>
          <DialogDescription>
            {t("office.important.mark_hint", { title: doc.title })}
          </DialogDescription>
        </DialogHeader>
        <form
          className="space-y-3"
          onSubmit={(e) => {
            e.preventDefault();
            save.mutate();
          }}
        >
          <div className="space-y-1">
            <Label htmlFor="important-mark-kind">
              {t("office.important.kind")}
            </Label>
            <NativeSelect
              id="important-mark-kind"
              value={kind}
              onChange={(e) =>
                setKind(e.target.value as ArchiveImportantKind)
              }
            >
              {ARCHIVE_IMPORTANT_KINDS.map((k) => (
                <NativeSelectOption key={k} value={k}>
                  {t(`office.important.kind_${k}`)}
                </NativeSelectOption>
              ))}
            </NativeSelect>
          </div>
          <Label className="flex w-fit items-center gap-2 text-sm leading-normal font-normal">
            <Checkbox
              checked={visible}
              onCheckedChange={(v) => setVisible(v === true)}
            />
            {t("office.archive.visible_to_taxpayer")}
          </Label>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              disabled={save.isPending}
              onClick={onClose}
            >
              {t("office.archive.cancel")}
            </Button>
            <Button type="submit" disabled={save.isPending}>
              {t("office.important.mark_confirm")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

/**
 * Arşive belge yükleme penceresi. defaultImportant (#780 B): "Önemli
 * belgeler" bölümünden açılınca tür seçili ve "Mükellef de görsün" açık gelir.
 */
export function UploadDialog({
  engagement,
  defaultCategory,
  defaultImportant,
  onClose,
}: {
  engagement: string;
  defaultCategory: ArchiveCategory;
  defaultImportant?: ArchiveImportantKind;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const qc = useQueryClient();
  const [file, setFile] = useState<File | null>(null);
  const [category, setCategory] = useState<ArchiveCategory>(defaultCategory);
  const [isgKind, setIsgKind] = useState<ArchiveIsgKind>("isg_training");
  const [title, setTitle] = useState("");
  const [note, setNote] = useState("");
  const [documentDate, setDocumentDate] = useState("");
  const [validUntil, setValidUntil] = useState("");
  const [visible, setVisible] = useState(!!defaultImportant);
  const [important, setImportant] = useState<ArchiveImportantKind | "">(
    defaultImportant ?? "",
  );

  const upload = useMutation({
    mutationFn: () =>
      archiveService.upload(slug, engagement, {
        file: file!,
        category,
        isgKind: category === "isg" ? isgKind : undefined,
        title,
        note,
        documentDate,
        validUntil,
        visibleToTaxpayer: visible,
        importantKind: important || undefined,
      }),
    onSuccess: () => {
      void qc.invalidateQueries({
        queryKey: archiveKeys.all(slug, engagement),
      });
      toast.success(t("office.archive.uploaded"));
      onClose();
    },
    onError: (err) => toast.error(err.message || t("office.archive.error")),
  });

  return (
    <Dialog
      open
      onOpenChange={(open) => !open && !upload.isPending && onClose()}
    >
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t("office.archive.upload_title")}</DialogTitle>
          <DialogDescription>
            {t("office.archive.upload_hint")}
          </DialogDescription>
        </DialogHeader>
        <form
          className="space-y-3"
          onSubmit={(e) => {
            e.preventDefault();
            if (file) upload.mutate();
          }}
        >
          <div className="space-y-1">
            <Label htmlFor="archive-file">{t("office.archive.file")}</Label>
            <Input
              id="archive-file"
              type="file"
              accept=".pdf,.jpg,.jpeg,.png,application/pdf,image/jpeg,image/png"
              onChange={(e) => setFile(e.target.files?.[0] ?? null)}
            />
          </div>
          <div className="grid gap-3 sm:grid-cols-2">
            <div className="space-y-1">
              <Label htmlFor="archive-category">
                {t("office.archive.col_category")}
              </Label>
              <NativeSelect
                id="archive-category"
                value={category}
                onChange={(e) => setCategory(e.target.value as ArchiveCategory)}
              >
                {ARCHIVE_CATEGORIES.map((c) => (
                  <NativeSelectOption key={c} value={c}>
                    {t(`office.archive.category_${c}`)}
                  </NativeSelectOption>
                ))}
              </NativeSelect>
            </div>
            {category === "isg" ? (
              <div className="space-y-1">
                <Label htmlFor="archive-isg-kind">
                  {t("office.archive.isg_kind")}
                </Label>
                <NativeSelect
                  id="archive-isg-kind"
                  value={isgKind}
                  onChange={(e) => setIsgKind(e.target.value as ArchiveIsgKind)}
                >
                  {ARCHIVE_ISG_KINDS.map((k) => (
                    <NativeSelectOption key={k} value={k}>
                      {t(`office.archive.isg_${k}`)}
                    </NativeSelectOption>
                  ))}
                </NativeSelect>
              </div>
            ) : null}
          </div>
          <div className="space-y-1">
            <Label htmlFor="archive-important">
              {t("office.important.kind_field")}
            </Label>
            <NativeSelect
              id="archive-important"
              value={important}
              onChange={(e) =>
                setImportant(e.target.value as ArchiveImportantKind | "")
              }
            >
              <NativeSelectOption value="">
                {t("office.important.kind_none")}
              </NativeSelectOption>
              {ARCHIVE_IMPORTANT_KINDS.map((k) => (
                <NativeSelectOption key={k} value={k}>
                  {t(`office.important.kind_${k}`)}
                </NativeSelectOption>
              ))}
            </NativeSelect>
          </div>
          <div className="space-y-1">
            <Label htmlFor="archive-title">
              {t("office.archive.col_title")}
            </Label>
            <Input
              id="archive-title"
              value={title}
              maxLength={200}
              placeholder={file?.name ?? ""}
              onChange={(e) => setTitle(e.target.value)}
            />
          </div>
          <div className="grid gap-3 sm:grid-cols-2">
            <div className="space-y-1">
              <Label htmlFor="archive-document-date">
                {t("office.archive.col_date")}
              </Label>
              <DatePicker
                id="archive-document-date"
                value={documentDate}
                onChange={setDocumentDate}
              />
            </div>
            <div className="space-y-1">
              <Label htmlFor="archive-valid-until">
                {t("office.archive.col_valid_until")}
              </Label>
              <DatePicker
                id="archive-valid-until"
                value={validUntil}
                onChange={setValidUntil}
              />
            </div>
          </div>
          <div className="space-y-1">
            <Label htmlFor="archive-note">{t("office.archive.note")}</Label>
            <Textarea
              id="archive-note"
              value={note}
              maxLength={1000}
              rows={2}
              onChange={(e) => setNote(e.target.value)}
            />
          </div>
          <Label className="flex w-fit items-center gap-2 text-sm leading-normal font-normal">
            <Checkbox
              checked={visible}
              onCheckedChange={(v) => setVisible(v === true)}
            />
            {t("office.archive.visible_to_taxpayer")}
          </Label>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              disabled={upload.isPending}
              onClick={onClose}
            >
              {t("office.archive.cancel")}
            </Button>
            <Button type="submit" disabled={!file || upload.isPending}>
              {t("office.archive.save")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
