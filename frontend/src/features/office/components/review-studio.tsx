"use client";

import {
  ArrowLeft,
  Check,
  CornerDownLeft,
  History,
  Keyboard,
  Pencil,
  ScrollText,
  Sparkles,
  Undo2,
  X,
} from "lucide-react";
import { AnimatePresence, motion, useReducedMotion } from "motion/react";
import Link from "next/link";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import { Button } from "@/components/ui/button";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import {
  Popover,
  PopoverAnchor,
  PopoverContent,
} from "@/components/ui/popover";
import { Slider } from "@/components/ui/slider";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { useOffice, useOfficeActions } from "../hooks/use-office";
import { accountName, commonAccounts } from "../lib/accounts";
import { REVIEW_THRESHOLD } from "../lib/suggest";
import type { Invoice, InvoiceLine, Suggestion } from "../types";
import {
  Figure,
  Money,
  OfficeDataGate,
  SeverityBadge,
  useWarningText,
} from "./office-ui";

type Decision = {
  id: string;
  kind: "approved" | "rejected";
  code: string;
  /** Kararın öncesindeki öneri; geri alınca buna döner. */
  prev: { accountCode: string; accountName: string };
};

function Kbd({
  children,
  className,
}: {
  children: React.ReactNode;
  className?: string;
}) {
  return (
    <kbd
      className={cn(
        "bg-muted text-muted-foreground inline-flex h-5 min-w-5 items-center justify-center rounded border px-1 font-mono text-[10px] font-medium",
        className,
      )}
    >
      {children}
    </kbd>
  );
}

function toneOf(c: number) {
  return c >= REVIEW_THRESHOLD ? "approved" : c >= 0.6 ? "review" : "warning";
}

const toneBg = {
  approved: "bg-status-approved",
  review: "bg-status-review",
  warning: "bg-status-warning",
} as const;
const toneText = {
  approved: "text-status-approved",
  review: "text-amber-700 dark:text-status-review",
  warning: "text-status-warning",
} as const;

/** Oturumdaki ilerleme: karara bağlanan / başlangıçta bekleyen. */
function ProgressRing({ done, total }: { done: number; total: number }) {
  const r = 16;
  const c = 2 * Math.PI * r;
  const pct = total ? done / total : 1;
  return (
    <div className="relative size-10" aria-hidden>
      <svg viewBox="0 0 40 40" className="size-10 -rotate-90">
        <circle
          cx="20"
          cy="20"
          r={r}
          className="stroke-muted"
          strokeWidth="4"
          fill="none"
        />
        <motion.circle
          cx="20"
          cy="20"
          r={r}
          className="stroke-status-approved"
          strokeWidth="4"
          fill="none"
          strokeLinecap="round"
          strokeDasharray={c}
          animate={{ strokeDashoffset: c * (1 - pct) }}
          transition={{ type: "spring", stiffness: 120, damping: 20 }}
        />
      </svg>
      <span className="figure absolute inset-0 flex items-center justify-center text-[10px] font-semibold">
        {Math.round(pct * 100)}
      </span>
    </div>
  );
}

function ConfidenceMeter({ value, large }: { value: number; large?: boolean }) {
  const tone = toneOf(value);
  return (
    <div className="flex items-center gap-2">
      <div
        className={cn(
          "bg-muted relative overflow-hidden rounded-full",
          large ? "h-2 w-40" : "h-1.5 w-12",
        )}
      >
        <motion.div
          className={cn("h-full rounded-full", toneBg[tone])}
          initial={{ width: 0 }}
          animate={{ width: `${Math.round(value * 100)}%` }}
          transition={{ duration: 0.5, ease: "easeOut" }}
        />
        {large ? (
          <span
            className="bg-foreground/60 absolute top-[-3px] h-[14px] w-px"
            style={{ left: `${REVIEW_THRESHOLD * 100}%` }}
            aria-hidden
          />
        ) : null}
      </div>
      <Figure className={cn("text-xs", toneText[tone])}>
        %{Math.round(value * 100)}
      </Figure>
    </div>
  );
}

/** Önerinin kanıt zinciri (G12): satır → kaynak → geçmiş → güven → eşik. */
function EvidenceChain({
  s,
  line,
  invoice,
  history,
}: {
  s: Suggestion;
  line: InvoiceLine;
  invoice: Invoice;
  history: number;
}) {
  const { t } = useLocale();
  const steps = [
    {
      icon: ScrollText,
      title: t("office.studio.ev_line"),
      body: (
        <>
          “{line.name}” ·{" "}
          <Money value={line.lineTotal} currency={invoice.currency} />
        </>
      ),
    },
    {
      icon: Sparkles,
      title: `${t("office.studio.ev_source")}: ${t(`office.origin_${s.origin}`)}`,
      body: t(`office.reason.${s.reason.replace(":", "_")}`),
    },
    {
      icon: History,
      title: t("office.studio.ev_history"),
      body: history
        ? t("office.studio.ev_history_n", {
            count: history,
            code: s.accountCode,
          })
        : t("office.studio.ev_history_none"),
    },
  ];
  return (
    <ol className="relative space-y-4">
      <span
        className="bg-border absolute top-2 bottom-2 left-[11px] w-px"
        aria-hidden
      />
      {steps.map((step, i) => (
        <motion.li
          key={step.title}
          className="relative flex gap-3"
          initial={{ opacity: 0, x: 8 }}
          animate={{ opacity: 1, x: 0 }}
          transition={{ delay: 0.05 * i }}
        >
          <span className="bg-card relative z-10 flex size-6 shrink-0 items-center justify-center rounded-full border">
            <step.icon className="text-muted-foreground size-3.5" aria-hidden />
          </span>
          <div className="min-w-0 text-sm">
            <div className="text-muted-foreground text-xs">{step.title}</div>
            <div className="break-words">{step.body}</div>
          </div>
        </motion.li>
      ))}
      <li className="relative flex gap-3">
        <span className="bg-card relative z-10 flex size-6 shrink-0 items-center justify-center rounded-full border">
          <span
            className={cn("size-2 rounded-full", toneBg[toneOf(s.confidence)])}
          />
        </span>
        <div className="text-sm">
          <div className="text-muted-foreground text-xs">
            {t("office.studio.ev_confidence", {
              threshold: Math.round(REVIEW_THRESHOLD * 100),
            })}
          </div>
          <ConfidenceMeter value={s.confidence} large />
        </div>
      </li>
    </ol>
  );
}

function CodePicker({
  open,
  onOpenChange,
  onPick,
  children,
}: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
  onPick: (code: string, name: string) => void;
  children: React.ReactNode;
}) {
  const { t } = useLocale();
  const [q, setQ] = useState("");
  const custom =
    /^\d{3}(\.\d{2}){0,2}(\.\d{3})?$/.test(q.trim()) &&
    !commonAccounts.some((a) => a.code === q.trim());
  return (
    <Popover open={open} onOpenChange={onOpenChange}>
      <PopoverAnchor asChild>{children}</PopoverAnchor>
      <PopoverContent className="w-80 p-0" align="start">
        <Command
          filter={(value, search) =>
            value
              .toLocaleLowerCase("tr")
              .includes(search.trim().toLocaleLowerCase("tr"))
              ? 1
              : 0
          }
        >
          <CommandInput
            value={q}
            onValueChange={setQ}
            placeholder={t("office.studio.code_search")}
          />
          <CommandList>
            <CommandEmpty>{t("office.studio.code_empty")}</CommandEmpty>
            {custom ? (
              <CommandGroup heading={t("office.studio.code_custom")}>
                <CommandItem
                  value={`custom-${q}`}
                  onSelect={() => onPick(q.trim(), t("office.manual_code"))}
                >
                  <Figure className="font-semibold">{q.trim()}</Figure>
                </CommandItem>
              </CommandGroup>
            ) : null}
            <CommandGroup heading={t("office.studio.code_common")}>
              {commonAccounts.map((a) => (
                <CommandItem
                  key={a.code}
                  value={`${a.code} ${a.name}`}
                  onSelect={() => onPick(a.code, a.name)}
                >
                  <Figure className="w-24 shrink-0 font-semibold">
                    {a.code}
                  </Figure>
                  <span className="truncate">{a.name}</span>
                </CommandItem>
              ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}

function StudioInner() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const reduce = useReducedMotion();
  const suggestions = useOffice((s) => s.suggestions);
  const invoices = useOffice((s) => s.invoices);
  const warnings = useOffice((s) => s.warnings);
  const { decideSuggestion, reopenSuggestion } = useOfficeActions();
  const warningText = useWarningText();

  const queue = useMemo(
    () =>
      suggestions
        .filter((s) => s.status === "pending")
        .sort((a, b) => a.confidence - b.confidence),
    [suggestions],
  );
  const [initial] = useState(() => queue.length);
  const [cursor, setCursor] = useState(0);
  const [log, setLog] = useState<Decision[]>([]);
  const [picking, setPicking] = useState(false);
  const [help, setHelp] = useState(false);
  const [bulkAt, setBulkAt] = useState(70);
  const [direction, setDirection] = useState<"approved" | "rejected" | "skip">(
    "skip",
  );
  const listRef = useRef<HTMLDivElement>(null);

  const index = Math.min(cursor, Math.max(queue.length - 1, 0));
  const current = queue[index];
  const invoice = current
    ? invoices.find((i) => i.id === current.invoiceId)
    : undefined;
  const line = invoice?.lines.find((l) => l.id === current?.lineId);
  const invWarnings = invoice
    ? warnings.filter((w) => w.entityId === invoice.id && w.status === "open")
    : [];
  const history = current
    ? suggestions.filter((s) => {
        if (
          s.status !== "approved" ||
          s.id === current.id ||
          s.accountCode !== current.accountCode
        )
          return false;
        const inv = invoices.find((i) => i.id === s.invoiceId);
        return inv?.counterparty.taxId === invoice?.counterparty.taxId;
      }).length
    : 0;
  const bulkCount = queue.filter((s) => s.confidence * 100 >= bulkAt).length;

  const decide = useCallback(
    (
      kind: "approved" | "rejected",
      override?: { accountCode: string; accountName: string },
    ) => {
      if (!current) return;
      setDirection(kind);
      decideSuggestion(current.id, kind, override);
      setLog((l) => [
        {
          id: current.id,
          kind,
          code: override?.accountCode ?? current.accountCode,
          prev: {
            accountCode: current.accountCode,
            accountName: current.accountName,
          },
        },
        ...l,
      ]);
    },
    [current, decideSuggestion],
  );

  const undo = useCallback(() => {
    const [last, ...rest] = log;
    if (!last) return;
    reopenSuggestion(last.id, last.prev);
    setLog(rest);
    setDirection("skip");
    const at = suggestions
      .filter((s) => s.status === "pending" || s.id === last.id)
      .sort((a, b) => a.confidence - b.confidence)
      .findIndex((s) => s.id === last.id);
    if (at >= 0) setCursor(at);
  }, [log, reopenSuggestion, suggestions]);

  const bulkApprove = () => {
    const picked = queue.filter((s) => s.confidence * 100 >= bulkAt);
    for (const s of picked) decideSuggestion(s.id, "approved");
    setLog((l) => [
      ...picked.map((s) => ({
        id: s.id,
        kind: "approved" as const,
        code: s.accountCode,
        prev: { accountCode: s.accountCode, accountName: s.accountName },
      })),
      ...l,
    ]);
  };

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const el = e.target as HTMLElement | null;
      if (
        el &&
        (el.isContentEditable ||
          ["INPUT", "TEXTAREA", "SELECT"].includes(el.tagName))
      )
        return;
      if (e.metaKey || e.ctrlKey || e.altKey || picking) return;
      const k = e.key.toLowerCase();
      if (k === "j" || e.key === "ArrowDown") {
        setDirection("skip");
        setCursor((c) => Math.min(c + 1, queue.length - 1));
      } else if (k === "k" || e.key === "ArrowUp") {
        setDirection("skip");
        setCursor((c) => Math.max(c - 1, 0));
      } else if (k === "a" || e.key === "Enter") decide("approved");
      else if (k === "r" || e.key === "Backspace") decide("rejected");
      else if (k === "e") setPicking(true);
      else if (k === "z" || k === "u") undo();
      else if (e.key === "?") setHelp((h) => !h);
      else return;
      e.preventDefault();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [decide, undo, picking, queue.length]);

  useEffect(() => {
    listRef.current
      ?.querySelector(`[data-index="${index}"]`)
      ?.scrollIntoView({ block: "nearest" });
  }, [index]);

  const exitX = reduce
    ? 0
    : direction === "approved"
      ? 80
      : direction === "rejected"
        ? -80
        : 0;
  const approvedCount = log.filter((d) => d.kind === "approved").length;

  return (
    <div className="flex h-[calc(100svh-7rem)] min-h-[560px] flex-col gap-3">
      <header className="flex flex-wrap items-center gap-3">
        <Button
          asChild
          variant="ghost"
          size="icon"
          aria-label={t("office.back_to_review")}
        >
          <Link href={routes.tenant.review(slug)}>
            <ArrowLeft className="size-4" />
          </Link>
        </Button>
        <ProgressRing done={log.length} total={Math.max(initial, log.length)} />
        <div className="min-w-0 flex-1">
          <h1 className="font-display text-lg leading-tight font-semibold">
            {t("office.studio.title")}
          </h1>
          <p className="text-muted-foreground text-xs">
            {t("office.studio.subtitle", {
              left: queue.length,
              approved: approvedCount,
              rejected: log.length - approvedCount,
            })}
          </p>
        </div>
        <div className="bg-card flex items-center gap-3 rounded-lg border px-3 py-1.5">
          <span className="text-muted-foreground text-xs whitespace-nowrap">
            {t("office.studio.bulk_label")}
          </span>
          <Slider
            className="w-32"
            min={40}
            max={95}
            step={1}
            value={[bulkAt]}
            onValueChange={(v) => setBulkAt(v[0] ?? 70)}
            aria-label={t("office.studio.bulk_label")}
          />
          <Figure className="w-9 text-xs">%{bulkAt}</Figure>
          <Button
            size="sm"
            variant="outline"
            disabled={!bulkCount}
            onClick={bulkApprove}
          >
            {t("office.studio.bulk_action", { count: bulkCount })}
          </Button>
        </div>
        <Button
          variant="outline"
          size="sm"
          onClick={undo}
          disabled={!log.length}
        >
          <Undo2 className="size-4" />
          {t("office.studio.undo")} <Kbd>Z</Kbd>
        </Button>
        <Button
          variant="ghost"
          size="icon"
          onClick={() => setHelp((h) => !h)}
          aria-label={t("office.studio.shortcuts")}
        >
          <Keyboard className="size-4" />
        </Button>
      </header>

      <AnimatePresence>
        {help ? (
          <motion.div
            initial={{ opacity: 0, y: -6 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0, y: -6 }}
            className="bg-card text-muted-foreground flex flex-wrap gap-x-5 gap-y-1 rounded-lg border px-4 py-2 text-xs"
          >
            {[
              ["J / ↓", t("office.studio.key_next")],
              ["K / ↑", t("office.studio.key_prev")],
              ["A / ↵", t("office.approve")],
              ["R / ⌫", t("office.reject")],
              ["E", t("office.change_code")],
              ["Z", t("office.studio.undo")],
              ["?", t("office.studio.shortcuts")],
            ].map(([k, label]) => (
              <span key={k} className="inline-flex items-center gap-1.5">
                <Kbd>{k}</Kbd>
                {label}
              </span>
            ))}
          </motion.div>
        ) : null}
      </AnimatePresence>

      <div
        className={cn(
          "grid min-h-0 flex-1 gap-3",
          queue.length
            ? "lg:grid-cols-[260px_minmax(0,1fr)_320px]"
            : "lg:grid-cols-[minmax(0,1fr)_320px]",
        )}
      >
        {/* Kuyruk */}
        {queue.length ? (
          <div
            ref={listRef}
            className="bg-card hidden min-h-0 overflow-y-auto rounded-xl border p-1 lg:block"
            role="listbox"
            aria-label={t("office.studio.queue")}
          >
            <AnimatePresence initial={false}>
              {queue.map((s, i) => {
                const inv = invoices.find((x) => x.id === s.invoiceId);
                const l = inv?.lines.find((x) => x.id === s.lineId);
                return (
                  <motion.button
                    layout
                    key={s.id}
                    data-index={i}
                    role="option"
                    aria-selected={i === index}
                    initial={{ opacity: 0 }}
                    animate={{ opacity: 1 }}
                    exit={{ opacity: 0, height: 0 }}
                    onClick={() => {
                      setDirection("skip");
                      setCursor(i);
                    }}
                    className={cn(
                      "flex w-full items-center gap-2 rounded-lg px-2.5 py-2 text-left text-sm",
                      i === index ? "bg-accent" : "hover:bg-muted/60",
                    )}
                  >
                    <span
                      className={cn(
                        "size-2 shrink-0 rounded-full",
                        toneBg[toneOf(s.confidence)],
                      )}
                      aria-hidden
                    />
                    <span className="min-w-0 flex-1">
                      <span className="block truncate">{l?.name}</span>
                      <Figure className="text-muted-foreground block truncate text-[11px]">
                        {s.accountCode} · {inv?.number}
                      </Figure>
                    </span>
                  </motion.button>
                );
              })}
            </AnimatePresence>
          </div>
        ) : null}

        {/* Odak kartı */}
        <div className="relative min-h-0">
          <AnimatePresence mode="popLayout" initial={false}>
            {current && invoice && line ? (
              <motion.article
                key={current.id}
                initial={
                  reduce ? { opacity: 0 } : { opacity: 0, y: 24, scale: 0.98 }
                }
                animate={{ opacity: 1, y: 0, scale: 1, x: 0, rotate: 0 }}
                exit={
                  reduce
                    ? { opacity: 0 }
                    : { opacity: 0, x: exitX, rotate: exitX / 40 }
                }
                transition={{ type: "spring", stiffness: 260, damping: 26 }}
                className="bg-card flex h-full flex-col overflow-hidden rounded-xl border shadow-sm"
              >
                <div className="flex flex-wrap items-center gap-x-3 gap-y-1 border-b px-6 py-3 text-xs">
                  <Link
                    href={routes.tenant.invoice(slug, invoice.id)}
                    className="hover:underline"
                  >
                    <Figure className="font-medium">{invoice.number}</Figure>
                  </Link>
                  <span className="text-muted-foreground">
                    {invoice.counterparty.title}
                  </span>
                  <Figure className="text-muted-foreground">
                    {invoice.counterparty.taxId}
                  </Figure>
                  <span className="text-muted-foreground ml-auto">
                    {t(`office.direction_${invoice.direction}`)} ·{" "}
                    {invoice.profile}
                  </span>
                </div>
                <div className="flex flex-1 flex-col justify-center gap-6 px-6 py-8 sm:px-10">
                  <div>
                    <div className="text-muted-foreground text-xs">
                      {t("office.col_item")}
                    </div>
                    <div className="text-2xl leading-tight font-semibold tracking-tight sm:text-3xl">
                      {line.name}
                    </div>
                    <div className="mt-1 flex flex-wrap items-baseline gap-x-4">
                      <Money
                        value={line.lineTotal}
                        currency={invoice.currency}
                        className="text-xl"
                      />
                      <Figure className="text-muted-foreground text-sm">
                        {line.quantity} {line.unit} · KDV %{line.vatPercent}
                      </Figure>
                    </div>
                  </div>
                  <CodePicker
                    open={picking}
                    onOpenChange={setPicking}
                    onPick={(code, name) => {
                      setPicking(false);
                      decide("approved", {
                        accountCode: code,
                        accountName: accountName(code) ?? name,
                      });
                    }}
                  >
                    <button
                      type="button"
                      onClick={() => setPicking(true)}
                      className="group hover:border-brand/50 relative rounded-xl border border-dashed p-5 text-left transition-colors"
                    >
                      <span className="text-muted-foreground flex items-center gap-2 text-xs">
                        {t("office.studio.suggested")}
                        <Pencil className="size-3 opacity-0 transition-opacity group-hover:opacity-100" />
                      </span>
                      <span className="flex flex-wrap items-baseline gap-x-4">
                        <Figure className="text-4xl font-semibold tracking-tight sm:text-5xl">
                          {current.accountCode}
                        </Figure>
                        <span className="text-muted-foreground text-lg">
                          {current.accountName}
                        </span>
                      </span>
                    </button>
                  </CodePicker>
                </div>
                <div className="grid grid-cols-3 gap-2 border-t p-3">
                  <Button
                    variant="outline"
                    onClick={() => decide("rejected")}
                    className="h-11"
                  >
                    <X className="size-4" />
                    {t("office.reject")} <Kbd>R</Kbd>
                  </Button>
                  <Button
                    variant="outline"
                    onClick={() => setPicking(true)}
                    className="h-11"
                  >
                    <Pencil className="size-4" />
                    {t("office.change_code")} <Kbd>E</Kbd>
                  </Button>
                  <Button
                    onClick={() => decide("approved")}
                    className="bg-status-approved text-status-approved-foreground hover:bg-status-approved/90 h-11"
                  >
                    <Check className="size-4" />
                    {t("office.approve")}{" "}
                    <Kbd className="border-white/30 bg-white/15 text-inherit">
                      A
                    </Kbd>
                  </Button>
                </div>
              </motion.article>
            ) : (
              <motion.div
                key="done"
                initial={{ opacity: 0, scale: 0.96 }}
                animate={{ opacity: 1, scale: 1 }}
                className="bg-card flex h-full flex-col items-center justify-center gap-3 rounded-xl border p-8 text-center"
              >
                <motion.div
                  initial={{ scale: 0, rotate: -30 }}
                  animate={{ scale: 1, rotate: 0 }}
                  transition={{
                    type: "spring",
                    stiffness: 200,
                    damping: 12,
                    delay: 0.1,
                  }}
                  className="bg-status-approved/12 text-status-approved flex size-16 items-center justify-center rounded-full"
                >
                  <Check className="size-8" />
                </motion.div>
                <h2 className="text-xl font-semibold">
                  {t("office.studio.done_title")}
                </h2>
                <p className="text-muted-foreground max-w-sm text-sm">
                  {t("office.studio.done_body", { count: log.length })}
                </p>
                <div className="flex gap-2">
                  {log.length ? (
                    <Button variant="outline" onClick={undo}>
                      <Undo2 className="size-4" />
                      {t("office.studio.undo")}
                    </Button>
                  ) : null}
                  <Button asChild>
                    <Link href={routes.tenant.home(slug)}>
                      {t("office.studio.to_overview")}
                    </Link>
                  </Button>
                </div>
              </motion.div>
            )}
          </AnimatePresence>
        </div>

        {/* Kanıt paneli */}
        <aside className="bg-card hidden min-h-0 flex-col gap-5 overflow-y-auto rounded-xl border p-4 lg:flex">
          {current && invoice && line ? (
            <>
              <section>
                <h2 className="mb-3 text-xs font-semibold tracking-wide uppercase">
                  {t("office.studio.why")}
                </h2>
                <EvidenceChain
                  s={current}
                  line={line}
                  invoice={invoice}
                  history={history}
                />
              </section>
              {invWarnings.length ? (
                <section>
                  <h2 className="mb-2 text-xs font-semibold tracking-wide uppercase">
                    {t("office.warnings_on_invoice")}
                  </h2>
                  <ul className="space-y-2">
                    {invWarnings.map((w) => (
                      <li key={w.id} className="space-y-1 text-sm">
                        <SeverityBadge severity={w.severity} />
                        <p>{warningText(w)}</p>
                      </li>
                    ))}
                  </ul>
                </section>
              ) : null}
            </>
          ) : null}
          {log.length ? (
            <section className="mt-auto">
              <h2 className="mb-2 text-xs font-semibold tracking-wide uppercase">
                {t("office.studio.recent")}
              </h2>
              <ul className="space-y-1">
                {log.slice(0, 6).map((d) => (
                  <motion.li
                    key={d.id}
                    initial={{ opacity: 0, y: -4 }}
                    animate={{ opacity: 1, y: 0 }}
                    className="flex items-center gap-2 text-xs"
                  >
                    {d.kind === "approved" ? (
                      <Check className="text-status-approved size-3.5" />
                    ) : (
                      <X className="text-status-blocking size-3.5" />
                    )}
                    <Figure>{d.code}</Figure>
                  </motion.li>
                ))}
              </ul>
            </section>
          ) : null}
          <p className="text-muted-foreground flex items-center gap-1.5 text-[11px]">
            <CornerDownLeft className="size-3" /> {t("office.studio.hint")}
          </p>
        </aside>
      </div>
    </div>
  );
}

export function ReviewStudioPage() {
  return (
    <OfficeDataGate>
      <StudioInner />
    </OfficeDataGate>
  );
}
