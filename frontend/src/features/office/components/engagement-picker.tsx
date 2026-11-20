"use client";

import { Check, ChevronsUpDown } from "lucide-react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useEffect, useState } from "react";

import { Button } from "@/components/ui/button";
import {
  Command,
  CommandEmpty,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import {
  useEngagementOptions,
  type EngagementOption,
} from "../hooks/use-office-invoices";

/** URL parameter holding the selected engagement (taxpayer) uuid. */
export const ENGAGEMENT_PARAM = "taxpayer";

/**
 * The selected engagement of the office's active ones. It lives in the URL
 * (?taxpayer=<engagement uuid>) so links keep it; the first one is picked
 * when none is given.
 */
export function useSelectedEngagement(
  slug: string,
  /** Mükellef bağlamında sabit mükellef: URL'ye yazılmaz (#468). */
  fixed?: string | null,
) {
  const params = useSearchParams();
  const router = useRouter();
  const pathname = usePathname();
  const { options, isLoading, isError } = useEngagementOptions(slug);
  const wanted = fixed || (params.get(ENGAGEMENT_PARAM) ?? "");
  const selected =
    options.find((o) => o.engagement === wanted) ?? options[0] ?? null;

  const select = (engagement: string) => {
    const next = new URLSearchParams(params);
    next.set(ENGAGEMENT_PARAM, engagement);
    // Mükellef değişince liste sayfası başa döner (#381).
    next.delete("page");
    next.delete("offset");
    router.replace(`${pathname}?${next.toString()}`, { scroll: false });
  };

  useEffect(() => {
    if (!fixed && !wanted && selected) select(selected.engagement);
    // select depends on params; run only when the default appears.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [fixed, wanted, selected?.engagement]);

  return { options, selected, select, isLoading, isError };
}

/**
 * Mükellef seçici (#573): unvan ya da VKN ile aranır; seçili mükellef
 * düğmede açıkça yazar. Seçim adreste (?taxpayer=) tutulur.
 */
export function EngagementPicker({
  options,
  selected,
  onSelect,
}: {
  options: EngagementOption[];
  selected: EngagementOption | null;
  onSelect: (engagement: string) => void;
}) {
  const { t } = useLocale();
  const [open, setOpen] = useState(false);
  const label = (o: EngagementOption) => o.title || o.taxId || o.engagement;
  const sorted = [...options].sort((a, b) =>
    label(a).localeCompare(label(b), "tr"),
  );
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="outline"
          role="combobox"
          aria-expanded={open}
          aria-label={`${t("office.col_taxpayer")}: ${selected ? label(selected) : ""}`}
          className="h-9 max-w-full min-w-64 justify-between gap-2 font-normal"
        >
          <span className="flex min-w-0 items-baseline gap-2">
            <span className="text-muted-foreground shrink-0 text-xs">
              {t("office.col_taxpayer")}
            </span>
            <span className="truncate font-medium">
              {selected ? label(selected) : t("office.picker.choose")}
            </span>
            {selected?.taxId && selected.title ? (
              <span className="text-muted-foreground shrink-0 font-mono text-xs">
                {selected.taxId}
              </span>
            ) : null}
          </span>
          <ChevronsUpDown className="size-4 shrink-0 opacity-50" aria-hidden />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-80 p-0" align="start">
        <Command
          filter={(value, search) => {
            const o = options.find((x) => x.engagement === value);
            if (!o) return 0;
            const q = search.toLocaleLowerCase("tr").trim();
            return o.title.toLocaleLowerCase("tr").includes(q) ||
              o.taxId.includes(q)
              ? 1
              : 0;
          }}
        >
          <CommandInput placeholder={t("office.picker.search")} />
          <CommandList>
            <CommandEmpty>{t("office.picker.empty")}</CommandEmpty>
            {sorted.map((o) => (
              <CommandItem
                key={o.engagement}
                value={o.engagement}
                onSelect={() => {
                  onSelect(o.engagement);
                  setOpen(false);
                }}
              >
                <Check
                  className={cn(
                    "size-4",
                    o.engagement === selected?.engagement
                      ? "opacity-100"
                      : "opacity-0",
                  )}
                  aria-hidden
                />
                <span className="min-w-0 flex-1 truncate">{label(o)}</span>
                {o.title && o.taxId ? (
                  <span className="text-muted-foreground font-mono text-xs">
                    {o.taxId}
                  </span>
                ) : null}
              </CommandItem>
            ))}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}
