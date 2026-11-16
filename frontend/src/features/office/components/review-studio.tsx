"use client";

import {
  ArrowLeft,
  Check,
  CornerDownLeft,
  Keyboard,
  Pencil,
  ScrollText,
  Sparkles,
  Undo2,
  X,
} from "lucide-react";
import { AnimatePresence, motion, useReducedMotion } from "motion/react";
import Link from "next/link";
import {
  useCallback,
  useDeferredValue,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { toast } from "sonner";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
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

import { useEngagementDirectory } from "../hooks/use-engagement-directory";
import {
  ACCOUNT_CODE_RE,
  accountValue,
  confidenceOf,
  correctedAccountValue,
  suggestionContext,
  useAccountPlanAccounts,
  useDecisionErrorText,
  useReviewDecision,
  useReviewSuggestions,
  useReviewThreshold,
  valueLabel,
} from "../hooks/use-review";
import {
  REVIEW_PAGE_MAX,
  type ReviewSuggestion,
} from "../services/review.service";
import { Figure } from "./office-ui";

const ACCOUNT_KIND = "account_code";

type DecisionKind = "accepted" | "corrected" | "rejected";

type Decision = {
  suggestion: ReviewSuggestion;
  kind: DecisionKind;
  code: string;
  /** The server confirmed the decision (only then can it be reverted). */
  settled: boolean;
  reverted: boolean;
};

const revertible = (d: Decision) =>
  d.settled && !d.reverted && d.kind !== "rejected";

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

type Tone = "approved" | "review" | "warning";

function toneOf(c: number, threshold: number): Tone {
  return c >= threshold ? "approved" : c >= 0.6 ? "review" : "warning";
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

/** Oturumdaki ilerleme: karara bağlanan / oturumda görülen. */
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

function ConfidenceMeter({
  value,
  threshold,
  large,
}: {
  value: number;
  threshold: number;
  large?: boolean;
}) {
  const tone = toneOf(value, threshold);
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
            style={{ left: `${threshold * 100}%` }}
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

function directionKey(direction?: string) {
  return direction === "incoming" || direction === "outgoing"
    ? `office.review_api.direction_${direction}`
    : undefined;
}

/** Önerinin kanıt zinciri (G12): kaynak satır → öneren → güven → eşik. */
function EvidenceChain({
  s,
  threshold,
}: {
  s: ReviewSuggestion;
  threshold: number;
}) {
  const { t } = useLocale();
  const ctx = suggestionContext(s);
  const lineBits = [
    ctx.lineType
      ? t("office.review_api.line_type", { type: ctx.lineType })
      : undefined,
    ctx.taxRate ? `KDV %${ctx.taxRate}` : undefined,
    ctx.lineNo
      ? t("office.review_api.invoice_line", { line: ctx.lineNo })
      : undefined,
  ].filter(Boolean);
  const source = s.provenance.rule_ref ?? s.provenance.model;
  const steps = [
    {
      icon: ScrollText,
      title: t("office.studio.ev_line"),
      body: (
        <>
          {ctx.itemName ? `“${ctx.itemName}”` : `${s.target_type}`}
          {lineBits.length ? (
            <span className="text-muted-foreground block text-xs">
              {lineBits.join(" · ")}
            </span>
          ) : null}
        </>
      ),
    },
    {
      icon: Sparkles,
      title: `${t("office.studio.ev_source")}: ${t(`office.review_api.origin_${s.provenance.origin}`)}`,
      body: (
        <>
          {s.rationale || t("office.review_api.no_rationale")}
          {source ? (
            <Figure className="text-muted-foreground block text-[11px]">
              {source}
            </Figure>
          ) : null}
        </>
      ),
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
            className={cn(
              "size-2 rounded-full",
              toneBg[toneOf(confidenceOf(s), threshold)],
            )}
          />
        </span>
        <div className="text-sm">
          <div className="text-muted-foreground text-xs">
            {t("office.studio.ev_confidence", {
              threshold: Math.round(threshold * 100),
            })}
          </div>
          <ConfidenceMeter
            value={confidenceOf(s)}
            threshold={threshold}
            large
          />
        </div>
      </li>
    </ol>
  );
}

/**
 * Hesap kodu seçici: mükellefin etkin hesap planından arar; plan yoksa ya da
 * okunamıyorsa kod elle yazılır.
 */
function CodePicker({
  engagement,
  open,
  onOpenChange,
  onPick,
  children,
}: {
  engagement?: string;
  open: boolean;
  onOpenChange: (v: boolean) => void;
  onPick: (code: string, name?: string) => void;
  children: React.ReactNode;
}) {
  const { t } = useLocale();
  const [q, setQ] = useState("");
  const search = useDeferredValue(q.trim());
  const accounts = useAccountPlanAccounts(engagement, search, open);
  const items = accounts.data?.items ?? [];
  const planMissing =
    accounts.isError || (accounts.isSuccess && !search && items.length === 0);
  const typed = q.trim();
  const custom =
    ACCOUNT_CODE_RE.test(typed) && !items.some((a) => a.code === typed);
  return (
    <Popover open={open} onOpenChange={onOpenChange}>
      <PopoverAnchor asChild>{children}</PopoverAnchor>
      <PopoverContent className="w-96 p-0" align="start">
        <Command shouldFilter={false}>
          <CommandInput
            value={q}
            onValueChange={setQ}
            placeholder={t("office.studio.code_search")}
          />
          <CommandList>
            {planMissing ? (
              <p className="text-muted-foreground px-3 py-2 text-xs">
                {t("office.review_api.plan_missing")}
              </p>
            ) : null}
            {accounts.isFetching && !items.length ? (
              <Loading className="py-3" label={t("common.loading")} />
            ) : null}
            {!custom && !accounts.isFetching ? (
              <CommandEmpty>{t("office.studio.code_empty")}</CommandEmpty>
            ) : null}
            {custom ? (
              <CommandGroup heading={t("office.studio.code_custom")}>
                <CommandItem
                  value={`custom-${typed}`}
                  onSelect={() => onPick(typed)}
                >
                  <Figure className="font-semibold">{typed}</Figure>
                </CommandItem>
              </CommandGroup>
            ) : null}
            {items.length ? (
              <CommandGroup heading={t("office.review_api.plan_accounts")}>
                {items.map((a) => (
                  <CommandItem
                    key={a.code}
                    value={a.code}
                    onSelect={() => onPick(a.code, a.name)}
                    className={cn(!a.is_leaf && "opacity-60")}
                  >
                    <Figure className="w-28 shrink-0 font-semibold">
                      {a.code}
                    </Figure>
                    <span className="truncate">{a.name}</span>
                  </CommandItem>
                ))}
              </CommandGroup>
            ) : null}
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
  const threshold = useReviewThreshold();
  const directory = useEngagementDirectory();
  const query = useReviewSuggestions({
    status: "pending",
    limit: REVIEW_PAGE_MAX,
    offset: 0,
  });
  const decision = useReviewDecision();
  const errorText = useDecisionErrorText();

  /** Decided in this session; hidden until the refetch drops them. */
  const [hidden, setHidden] = useState<ReadonlySet<string>>(new Set());
  const [cursor, setCursor] = useState(0);
  const [log, setLog] = useState<Decision[]>([]);
  const [picking, setPicking] = useState(false);
  const [help, setHelp] = useState(false);
  const [bulkAt, setBulkAt] = useState(70);
  const [bulkBusy, setBulkBusy] = useState(false);
  const [undoBusy, setUndoBusy] = useState(false);
  const [direction, setDirection] = useState<"approved" | "rejected" | "skip">(
    "skip",
  );
  const listRef = useRef<HTMLDivElement>(null);

  const queue = useMemo(
    () => (query.data?.items ?? []).filter((s) => !hidden.has(s.uuid)),
    [query.data, hidden],
  );
  const index = Math.min(cursor, Math.max(queue.length - 1, 0));
  const current = queue[index];
  const currentValue = current ? accountValue(current.proposed_value) : {};
  const canCorrect = !!current && current.kind === ACCOUNT_KIND;
  const bulkCount = queue.filter((s) => confidenceOf(s) * 100 >= bulkAt).length;

  const hide = useCallback(
    (uuid: string, on: boolean) =>
      setHidden((h) => {
        const next = new Set(h);
        if (on) next.add(uuid);
        else next.delete(uuid);
        return next;
      }),
    [],
  );
  const patchLog = useCallback(
    (uuid: string, patch: Partial<Decision> | null) =>
      setLog((l) =>
        patch === null
          ? l.filter((d) => d.suggestion.uuid !== uuid)
          : l.map((d) => (d.suggestion.uuid === uuid ? { ...d, ...patch } : d)),
      ),
    [],
  );
  const { mutateAsync } = decision;

  const decide = useCallback(
    (kind: DecisionKind, override?: { code: string; name?: string }) => {
      if (!current) return;
      const s = current;
      const code = override?.code ?? valueLabel(s.proposed_value);
      setDirection(kind === "rejected" ? "rejected" : "approved");
      hide(s.uuid, true);
      setLog((l) => [
        { suggestion: s, kind, code, settled: false, reverted: false },
        ...l,
      ]);
      // mutateAsync: per-call settlement (mutate's callbacks fire only for
      // the latest call when decisions overlap).
      mutateAsync(
        kind === "corrected" && override
          ? {
              suggestion: s,
              action: "correct",
              value: correctedAccountValue(
                s.proposed_value,
                override.code,
                override.name,
              ),
              reason: t("office.review_api.reason_correct"),
            }
          : kind === "rejected"
            ? {
                suggestion: s,
                action: "reject",
                reason: t("office.review_api.reason_reject"),
              }
            : { suggestion: s, action: "accept" },
      ).then(
        () => patchLog(s.uuid, { settled: true }),
        (err: unknown) => {
          hide(s.uuid, false);
          patchLog(s.uuid, null);
          toast.error(errorText(err));
        },
      );
    },
    [current, mutateAsync, hide, patchLog, errorText, t],
  );

  const undoTarget = log.find(revertible);
  const undo = useCallback(() => {
    if (undoBusy) return;
    if (!undoTarget) {
      if (log.some((d) => d.kind === "rejected" && !d.reverted)) {
        toast.info(t("office.review_api.undo_reject_unsupported"));
      }
      return;
    }
    const s = undoTarget.suggestion;
    setUndoBusy(true);
    const code = undoTarget.code;
    mutateAsync({
      suggestion: s,
      action: "revert",
      reason: t("office.review_api.reason_revert"),
    })
      .then(
        () => {
          patchLog(s.uuid, { reverted: true });
          setDirection("skip");
          toast.success(t("office.review_api.reverted_toast", { code }));
        },
        (err: unknown) => toast.error(errorText(err)),
      )
      .finally(() => setUndoBusy(false));
  }, [undoBusy, undoTarget, log, mutateAsync, patchLog, errorText, t]);

  const bulkApprove = async () => {
    const picked = queue.filter((s) => confidenceOf(s) * 100 >= bulkAt);
    if (!picked.length) return;
    setBulkBusy(true);
    setDirection("approved");
    let ok = 0;
    let failed = 0;
    let lastError: unknown;
    for (const s of picked) {
      hide(s.uuid, true);
      try {
        await mutateAsync({ suggestion: s, action: "accept" });
        ok += 1;
        setLog((l) => [
          {
            suggestion: s,
            kind: "accepted",
            code: valueLabel(s.proposed_value),
            settled: true,
            reverted: false,
          },
          ...l,
        ]);
      } catch (err) {
        failed += 1;
        lastError = err;
        hide(s.uuid, false);
      }
    }
    setBulkBusy(false);
    if (failed) {
      toast.error(
        t("office.review_api.bulk_partial", {
          ok,
          failed,
          reason: errorText(lastError),
        }),
      );
    } else {
      toast.success(t("office.review_api.bulk_done", { count: ok }));
    }
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
      if (e.metaKey || e.ctrlKey || e.altKey || picking || bulkBusy) return;
      const k = e.key.toLowerCase();
      if (k === "j" || e.key === "ArrowDown") {
        setDirection("skip");
        setCursor((c) => Math.min(c + 1, queue.length - 1));
      } else if (k === "k" || e.key === "ArrowUp") {
        setDirection("skip");
        setCursor((c) => Math.max(c - 1, 0));
      } else if (k === "a" || e.key === "Enter") decide("accepted");
      else if (k === "r" || e.key === "Backspace") decide("rejected");
      else if (k === "e") {
        if (canCorrect) setPicking(true);
      } else if (k === "z" || k === "u") undo();
      else if (e.key === "?") setHelp((h) => !h);
      else return;
      e.preventDefault();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [decide, undo, picking, bulkBusy, canCorrect, queue.length]);

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
  const active = log.filter((d) => !d.reverted);
  const rejectedCount = active.filter((d) => d.kind === "rejected").length;
  const approvedCount = active.length - rejectedCount;
  // Pending beyond the loaded page are counted but not shown until the
  // refetch after each decision pulls them in.
  const left =
    queue.length +
    Math.max((query.data?.total ?? 0) - (query.data?.items.length ?? 0), 0);

  const ctx = current ? suggestionContext(current) : undefined;
  const taxpayer = current?.engagement_uuid
    ? directory.entries.get(current.engagement_uuid)
    : undefined;
  const dirKey = directionKey(ctx?.direction);

  let body: React.ReactNode;
  if (query.isLoading) {
    body = (
      <div className="bg-card flex h-full items-center justify-center rounded-xl border">
        <Loading label={t("common.loading")} />
      </div>
    );
  } else if (query.isError) {
    body = (
      <ErrorState
        className="h-full"
        title={t("office.review_api.load_error")}
        description={query.error.message}
        onRetry={() => void query.refetch()}
        retryLabel={t("common.retry")}
      />
    );
  } else {
    body = (
      <AnimatePresence mode="popLayout" initial={false}>
        {current && ctx ? (
          <motion.article
            key={current.uuid}
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
              {taxpayer ? (
                <span className="font-medium">{taxpayer.title}</span>
              ) : null}
              {ctx.counterparty ? (
                <span className="text-muted-foreground">
                  {ctx.counterparty}
                </span>
              ) : null}
              {ctx.invoiceUuid ? (
                <Link
                  href={routes.tenant.invoice(slug, ctx.invoiceUuid)}
                  className="text-brand hover:underline"
                >
                  {t("office.review_api.invoice_link")}
                </Link>
              ) : null}
              <span className="text-muted-foreground ml-auto">
                {[
                  dirKey ? t(dirKey) : undefined,
                  ctx.invoiceType,
                  current.period,
                ]
                  .filter(Boolean)
                  .join(" · ")}
              </span>
            </div>
            <div className="flex flex-1 flex-col justify-center gap-6 px-6 py-8 sm:px-10">
              <div>
                <div className="text-muted-foreground text-xs">
                  {ctx.itemName
                    ? t("office.col_item")
                    : t("office.review_api.target")}
                </div>
                <div className="text-2xl leading-tight font-semibold tracking-tight sm:text-3xl">
                  {ctx.itemName ??
                    (ctx.counterparty
                      ? t("office.review_api.counterparty_account", {
                          name: ctx.counterparty,
                        })
                      : `${current.target_type}.${current.target_field}`)}
                </div>
                {ctx.taxRate || ctx.lineType ? (
                  <Figure className="text-muted-foreground mt-1 block text-sm">
                    {[
                      ctx.lineType
                        ? t("office.review_api.line_type", {
                            type: ctx.lineType,
                          })
                        : undefined,
                      ctx.taxRate ? `KDV %${ctx.taxRate}` : undefined,
                    ]
                      .filter(Boolean)
                      .join(" · ")}
                  </Figure>
                ) : null}
              </div>
              <CodePicker
                engagement={current.engagement_uuid}
                open={picking}
                onOpenChange={setPicking}
                onPick={(code, name) => {
                  setPicking(false);
                  if (code === currentValue.code) decide("accepted");
                  else decide("corrected", { code, name });
                }}
              >
                <button
                  type="button"
                  onClick={() => canCorrect && setPicking(true)}
                  className={cn(
                    "group relative rounded-xl border border-dashed p-5 text-left transition-colors",
                    canCorrect && "hover:border-brand/50",
                  )}
                >
                  <span className="text-muted-foreground flex items-center gap-2 text-xs">
                    {canCorrect
                      ? t("office.studio.suggested")
                      : t("office.review_api.proposed_value")}
                    {canCorrect ? (
                      <Pencil className="size-3 opacity-0 transition-opacity group-hover:opacity-100" />
                    ) : null}
                  </span>
                  <span className="flex flex-wrap items-baseline gap-x-4">
                    <Figure className="text-4xl font-semibold tracking-tight break-all sm:text-5xl">
                      {valueLabel(current.proposed_value)}
                    </Figure>
                    {currentValue.name ? (
                      <span className="text-muted-foreground text-lg">
                        {currentValue.name}
                      </span>
                    ) : null}
                  </span>
                </button>
              </CodePicker>
            </div>
            <div className="grid grid-cols-3 gap-2 border-t p-3">
              <Button
                variant="outline"
                onClick={() => decide("rejected")}
                className="h-11"
                disabled={bulkBusy}
              >
                <X className="size-4" />
                {t("office.reject")} <Kbd>R</Kbd>
              </Button>
              <Button
                variant="outline"
                onClick={() => setPicking(true)}
                className="h-11"
                disabled={!canCorrect || bulkBusy}
              >
                <Pencil className="size-4" />
                {t("office.change_code")} <Kbd>E</Kbd>
              </Button>
              <Button
                onClick={() => decide("accepted")}
                className="bg-status-approved text-status-approved-foreground hover:bg-status-approved/90 h-11"
                disabled={bulkBusy}
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
              {t("office.studio.done_body", { count: active.length })}
            </p>
            <div className="flex gap-2">
              {undoTarget ? (
                <Button variant="outline" onClick={undo} disabled={undoBusy}>
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
    );
  }

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
        <ProgressRing
          done={active.length}
          total={active.length + queue.length}
        />
        <div className="min-w-0 flex-1">
          <h1 className="font-display text-lg leading-tight font-semibold">
            {t("office.studio.title")}
          </h1>
          <p className="text-muted-foreground text-xs">
            {t("office.studio.subtitle", {
              left,
              approved: approvedCount,
              rejected: rejectedCount,
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
            disabled={!bulkCount || bulkBusy}
            onClick={() => void bulkApprove()}
          >
            {bulkBusy
              ? t("office.review_api.bulk_running")
              : t("office.studio.bulk_action", { count: bulkCount })}
          </Button>
        </div>
        <Button
          variant="outline"
          size="sm"
          onClick={undo}
          disabled={!undoTarget || undoBusy}
          title={t("office.review_api.undo_hint")}
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
            <span>{t("office.review_api.undo_hint")}</span>
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
                const c = suggestionContext(s);
                return (
                  <motion.button
                    layout
                    key={s.uuid}
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
                        toneBg[toneOf(confidenceOf(s), threshold)],
                      )}
                      aria-hidden
                    />
                    <span className="min-w-0 flex-1">
                      <span className="block truncate">
                        {c.itemName ?? c.counterparty ?? s.target_field}
                      </span>
                      <Figure className="text-muted-foreground block truncate text-[11px]">
                        {valueLabel(s.proposed_value)}
                        {s.engagement_uuid &&
                        directory.entries.get(s.engagement_uuid)
                          ? ` · ${directory.entries.get(s.engagement_uuid)?.title}`
                          : ""}
                      </Figure>
                    </span>
                  </motion.button>
                );
              })}
            </AnimatePresence>
          </div>
        ) : null}

        {/* Odak kartı */}
        <div className="relative min-h-0">{body}</div>

        {/* Kanıt paneli */}
        <aside className="bg-card hidden min-h-0 flex-col gap-5 overflow-y-auto rounded-xl border p-4 lg:flex">
          {current ? (
            <section>
              <h2 className="mb-3 text-xs font-semibold tracking-wide uppercase">
                {t("office.studio.why")}
              </h2>
              <EvidenceChain s={current} threshold={threshold} />
            </section>
          ) : null}
          {log.length ? (
            <section className="mt-auto">
              <h2 className="mb-2 text-xs font-semibold tracking-wide uppercase">
                {t("office.studio.recent")}
              </h2>
              <ul className="space-y-1">
                {log.slice(0, 6).map((d) => (
                  <motion.li
                    key={d.suggestion.uuid}
                    initial={{ opacity: 0, y: -4 }}
                    animate={{ opacity: 1, y: 0 }}
                    className={cn(
                      "flex items-center gap-2 text-xs",
                      (d.reverted || !d.settled) && "text-muted-foreground",
                    )}
                  >
                    {d.reverted ? (
                      <Undo2 className="size-3.5" />
                    ) : d.kind === "rejected" ? (
                      <X className="text-status-blocking size-3.5" />
                    ) : (
                      <Check className="text-status-approved size-3.5" />
                    )}
                    <Figure className={cn(d.reverted && "line-through")}>
                      {d.code}
                    </Figure>
                    {d.reverted ? (
                      <span>{t("office.review_api.status_reverted")}</span>
                    ) : d.kind === "corrected" ? (
                      <span>{t("office.review_api.status_corrected")}</span>
                    ) : null}
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
  return <StudioInner />;
}
