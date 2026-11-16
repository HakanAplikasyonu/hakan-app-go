"use client";

import {
  Building2,
  ClipboardCheck,
  FileText,
  Sparkles,
  TriangleAlert,
  Upload,
  Waypoints,
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

import { useEngagementDirectory } from "../hooks/use-engagement-directory";
import { usePendingSuggestionsTotal } from "../hooks/use-office-data";
import { ENGAGEMENT_PARAM } from "./engagement-picker";
import { Figure } from "./office-ui";

/**
 * Ofis komut paleti (⌘J / Ctrl+J): ofis ekranlarına ve mükelleflere
 * (unvan ya da VKN/TCKN) hızlı geçiş. Uygulamanın genel paletinden (⌘K,
 * sayfalar) ayrı: bu, ofisin verisinde arar. Veri yalnızca palet açıkken
 * istenir.
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
  const directory = useEngagementDirectory(open);
  const pending = usePendingSuggestionsTotal(open);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "j") {
        e.preventDefault();
        setOpen((o) => !o);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

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
            onSelect={() => go(routes.tenant.review(slug))}
          >
            <Sparkles />
            {t("office.review_title")}
            {pending.data !== undefined ? (
              <CommandShortcut>
                {t("office.palette.pending", { count: pending.data })}
              </CommandShortcut>
            ) : null}
          </CommandItem>
          <CommandItem
            value="command komuta merkezi matris"
            onSelect={() => go(routes.tenant.command(slug))}
          >
            <Waypoints />
            {t("office.command.title")}
          </CommandItem>
          <CommandItem
            value="upload ubl yükle"
            onSelect={() => go(routes.tenant.upload(slug))}
          >
            <Upload />
            {t("office.nav_upload")}
          </CommandItem>
          <CommandItem
            value="invoices faturalar"
            onSelect={() => go(routes.tenant.invoices(slug))}
          >
            <FileText />
            {t("office.nav_invoices")}
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
        </CommandGroup>
        <CommandGroup heading={t("office.nav_taxpayers")}>
          {directory.list.map((tp) => (
            <CommandItem
              key={tp.engagementUuid}
              value={`taxpayer ${tp.title} ${tp.taxId}`}
              onSelect={() =>
                go(
                  `${routes.tenant.invoices(slug)}?${ENGAGEMENT_PARAM}=${encodeURIComponent(tp.engagementUuid)}`,
                )
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
      </CommandList>
    </CommandDialog>
  );
}
