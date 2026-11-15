"use client";

import {
  Building2,
  ClipboardCheck,
  FileText,
  RotateCcw,
  Sparkles,
  TriangleAlert,
  Upload,
} from "lucide-react";
import { useRouter } from "next/navigation";
import { useEffect, useRef, useState } from "react";

import {
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandShortcut,
} from "@/components/ui/command";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

import { formatAmount } from "../lib/decimal";
import { officeStore } from "../store/office-store";
import { useOffice } from "../hooks/use-office";
import { Figure, Money } from "./office-ui";

/**
 * Ofis komut paleti (⌘J / Ctrl+J). Fatura no, ETTN, VKN, karşı taraf ya da
 * tutarla ("4600" veya "4.600,40") arar; işlemleri tek tuşla başlatır.
 * Uygulamanın genel paletinden (⌘K, sayfalar) ayrı: bu, ofisin verisinde arar.
 */
/** Her arama kelimesi değerin içinde geçmeli (Türkçe küçük harf). */
function tokenFilter(value: string, search: string) {
  const v = value.toLocaleLowerCase("tr");
  const words = search.toLocaleLowerCase("tr").split(/\s+/).filter(Boolean);
  return words.every((w) => v.includes(w)) ? 1 : 0;
}

export function OfficePalette() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const [open, setOpen] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);
  const taxpayers = useOffice((s) => s.taxpayers);
  const invoices = useOffice((s) => s.invoices);
  const pending = useOffice(
    (s) => s.suggestions.filter((x) => x.status === "pending").length,
  );

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "j") {
        e.preventDefault();
        const store = officeStore(slug);
        if (!store.persist.hasHydrated()) void store.persist.rehydrate();
        setOpen((o) => !o);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [slug]);

  const go = (href: string) => {
    setOpen(false);
    router.push(href);
  };

  return (
    <CommandDialog
      filter={tokenFilter}
      open={open}
      onOpenChange={setOpen}
      inputRef={inputRef}
      title={t("office.palette.title")}
      description={t("office.palette.description")}
      className="sm:max-w-2xl"
    >
      <CommandInput
        ref={inputRef}
        placeholder={t("office.palette.placeholder")}
      />
      <CommandList className="max-h-[60vh]">
        <CommandEmpty>{t("office.palette.empty")}</CommandEmpty>
        <CommandGroup heading={t("office.palette.actions")}>
          <CommandItem
            value="studio inceleme stüdyosu review"
            onSelect={() => go(routes.tenant.reviewStudio(slug))}
          >
            <Sparkles />
            {t("office.studio.title")}
            <CommandShortcut>
              {t("office.palette.pending", { count: pending })}
            </CommandShortcut>
          </CommandItem>
          <CommandItem
            value="upload ubl yükle"
            onSelect={() => go(routes.tenant.upload(slug))}
          >
            <Upload />
            {t("office.nav_upload")}
          </CommandItem>
          <CommandItem
            value="warnings uyarılar"
            onSelect={() => go(routes.tenant.warnings(slug))}
          >
            <TriangleAlert />
            {t("office.nav_warnings")}
          </CommandItem>
          <CommandItem
            value="review kuyruk"
            onSelect={() => go(routes.tenant.review(slug))}
          >
            <ClipboardCheck />
            {t("office.nav_review")}
          </CommandItem>
          <CommandItem
            value="reset demo sıfırla"
            onSelect={() => {
              officeStore(slug).getState().reset();
              setOpen(false);
            }}
          >
            <RotateCcw />
            {t("office.demo_reset")}
          </CommandItem>
        </CommandGroup>
        <CommandGroup heading={t("office.nav_taxpayers")}>
          {taxpayers.map((tp) => (
            <CommandItem
              key={tp.id}
              value={`taxpayer ${tp.title} ${tp.taxId} ${tp.taxOffice}`}
              onSelect={() =>
                go(`${routes.tenant.invoices(slug)}?taxpayer=${tp.id}`)
              }
            >
              <Building2 />
              <span className="flex-1 truncate">{tp.title}</span>
              <Figure className="text-muted-foreground text-xs">
                {tp.taxId}
              </Figure>
            </CommandItem>
          ))}
        </CommandGroup>
        <CommandGroup heading={t("office.nav_invoices")}>
          {invoices.map((inv) => (
            <CommandItem
              key={inv.id}
              value={`invoice ${inv.number} ${inv.ettn} ${inv.counterparty.title} ${inv.counterparty.taxId} ${inv.totals.payable} ${formatAmount(inv.totals.payable)} ${inv.taxes.map((x) => `${x.amount} ${formatAmount(x.amount)}`).join(" ")} ${inv.issueDate}`}
              onSelect={() => go(routes.tenant.invoice(slug, inv.id))}
            >
              <FileText />
              <span className="min-w-0 flex-1">
                <Figure className="block truncate font-medium">
                  {inv.number}
                </Figure>
                <span className="text-muted-foreground block truncate text-xs">
                  {inv.counterparty.title}
                </span>
              </span>
              <Money
                value={inv.totals.payable}
                currency={inv.currency}
                className="text-xs"
              />
            </CommandItem>
          ))}
        </CommandGroup>
      </CommandList>
    </CommandDialog>
  );
}
