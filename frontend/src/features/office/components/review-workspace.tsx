"use client";

import {
  ArrowLeft,
  Check,
  Keyboard,
  Pencil,
  ScrollText,
  Sparkles,
  Undo2,
  Wand2,
  X,
} from "lucide-react";
import { AnimatePresence, motion, useReducedMotion } from "motion/react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import {
  Suspense,
  useCallback,
  useDeferredValue,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { toast } from "sonner";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { GuvenOlcegi, TacRozeti } from "@/components/hakan";
import type { GuvenKaynagi } from "@/components/hakan";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
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
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { useIsMobile } from "@/hooks/use-mobile";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import { useAccountMemoryActions } from "../hooks/use-account-memory";
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
import { useOfficePeriod } from "../period";
import {
  REVIEW_PAGE_MAX,
  type ReviewSuggestion,
} from "../services/review.service";
import { FilterSelect, Figure } from "./office-ui";

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
  spoken = false,
}: {
  children: React.ReactNode;
  className?: string;
  /** Kısayol listesinde okunur; düğme içinde süs (ad "Onayla" kalır). */
  spoken?: boolean;
}) {
  return (
    <kbd
      aria-hidden={spoken ? undefined : true}
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
  footer,
  children,
}: {
  engagement?: string;
  open: boolean;
  onOpenChange: (v: boolean) => void;
  onPick: (code: string, name?: string) => void;
  /** Listenin altında (ör. "bu karşı taraf için kural yap"). */
  footer?: React.ReactNode;
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
        {footer ? <div className="border-t p-3">{footer}</div> : null}
      </PopoverContent>
    </Popover>
  );
}

/** Kuyruk süzgeci: güven kademesi (README: düşük < %60, orta %60–90). */
type Tier = "all" | "low" | "mid";

function tierOf(c: number): Exclude<Tier, "all"> | "high" {
  return c < 0.6 ? "low" : c < 0.9 ? "mid" : "high";
}

/** Önerinin kaynağı, güven ölçeğinin ipucu için. */
function kaynakOf(s: ReviewSuggestion): GuvenKaynagi | undefined {
  const ref = s.provenance.rule_ref ?? "";
  if (ref.startsWith("account_rule:") || s.provenance.origin === "rule") {
    return ref.startsWith("history") ? "gecmis" : "kural";
  }
  if (s.provenance.origin === "model") return "model";
  if (s.provenance.origin === "input") return "girdi";
  return undefined;
}

function counterpartyKeyOf(s: ReviewSuggestion) {
  const v = s.provenance.inputs?.counterparty_key;
  return typeof v === "string" && v ? v : undefined;
}

const DECIDED = new Set(["accepted", "corrected", "auto_accepted"]);

/**
 * "Bu karşı tarafla geçmiş": mükellefin son kararlarında aynı karşı taraf
 * için onaylanan kodlar (en çok kullanılan önce). Son 100 karar içinden.
 */
function useCounterpartyHistory(s: ReviewSuggestion | undefined) {
  const key = s ? counterpartyKeyOf(s) : undefined;
  const query = useReviewSuggestions(
    {
      engagement: s?.engagement_uuid ?? undefined,
      status: "all",
      kind: ACCOUNT_KIND,
      limit: REVIEW_PAGE_MAX,
      offset: 0,
    },
    !!key && !!s?.engagement_uuid,
  );
  return useMemo(() => {
    if (!key) return [];
    const counts = new Map<string, number>();
    for (const x of query.data?.items ?? []) {
      if (!DECIDED.has(x.status) || counterpartyKeyOf(x) !== key) continue;
      const code = accountValue(x.applied_value ?? x.proposed_value).code;
      if (code) counts.set(code, (counts.get(code) ?? 0) + 1);
    }
    return [...counts.entries()].sort((a, b) => b[1] - a[1]).slice(0, 4);
  }, [query.data, key]);
}

function ReviewWorkspaceInner() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const params = useSearchParams();
  const reduce = useReducedMotion();
  const isMobile = useIsMobile();
  const threshold = useReviewThreshold();
  const directory = useEngagementDirectory();
  const { period } = useOfficePeriod();
  const invoiceFilter = params.get("invoice");

  const [engagement, setEngagement] = useState("all");
  const [kind, setKind] = useState("all");
  const [tier, setTier] = useState<Tier>("all");
  const query = useReviewSuggestions({
    status: "pending",
    engagement: engagement === "all" ? undefined : engagement,
    kind: kind === "all" ? undefined : kind,
    period,
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
  const [ruleOnCorrect, setRuleOnCorrect] = useState(false);
  const [help, setHelp] = useState(false);
  const [bulkAt, setBulkAt] = useState(95);
  const [bulkBusy, setBulkBusy] = useState(false);
  const [undoBusy, setUndoBusy] = useState(false);
  const [ruleBusy, setRuleBusy] = useState(false);
  const [detailOpen, setDetailOpen] = useState(false);
  const [direction, setDirection] = useState<"approved" | "rejected" | "skip">(
    "skip",
  );
  const listRef = useRef<HTMLDivElement>(null);
  const detailRef = useRef<HTMLHeadingElement>(null);

  // En düşük güven önce (OfisKuyruk).
  const pending = useMemo(
    () =>
      (query.data?.items ?? [])
        .filter((s) => !hidden.has(s.uuid))
        .filter(
          (s) =>
            !invoiceFilter ||
            suggestionContext(s).invoiceUuid === invoiceFilter,
        )
        .sort((a, b) => confidenceOf(a) - confidenceOf(b)),
    [query.data, hidden, invoiceFilter],
  );
  const tierCounts = useMemo(() => {
    const out = { low: 0, mid: 0 };
    for (const s of pending) {
      const k = tierOf(confidenceOf(s));
      if (k === "low" || k === "mid") out[k] += 1;
    }
    return out;
  }, [pending]);
  const queue = useMemo(
    () =>
      tier === "all"
        ? pending
        : pending.filter((s) => tierOf(confidenceOf(s)) === tier),
    [pending, tier],
  );
  const index = Math.min(cursor, Math.max(queue.length - 1, 0));
  const current = queue[index];
  const currentValue = current ? accountValue(current.proposed_value) : {};
  const canCorrect = !!current && current.kind === ACCOUNT_KIND;
  const cpKey = current ? counterpartyKeyOf(current) : undefined;
  const canRule =
    canCorrect && !!cpKey && can(permissions.tenant.accountRules.write);
  const bulkPicked = queue.filter((s) => confidenceOf(s) * 100 >= bulkAt);
  const history = useCounterpartyHistory(canCorrect ? current : undefined);
  const memory = useAccountMemoryActions(slug, current?.engagement_uuid ?? "");
  const promote = memory.promote.mutateAsync;

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
      if (!current) return Promise.resolve(false);
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
      return mutateAsync(
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
        () => {
          patchLog(s.uuid, { settled: true });
          return true;
        },
        (err: unknown) => {
          hide(s.uuid, false);
          patchLog(s.uuid, null);
          toast.error(errorText(err));
          return false;
        },
      );
    },
    [current, mutateAsync, hide, patchLog, errorText, t],
  );

  /**
   * "Kural yap": karar verilir, ardından bu karşı taraf hep bu koda işlensin
   * diye kural yazılır (kural adayı terfisiyle aynı uç, #310).
   */
  const decideWithRule = useCallback(
    async (override?: { code: string; name?: string }) => {
      if (!current || !cpKey || ruleBusy) return;
      const s = current;
      const ctx = suggestionContext(s);
      const code = override?.code ?? currentValue.code;
      if (!code) return;
      setRuleBusy(true);
      try {
        const ok = await decide(override ? "corrected" : "accepted", override);
        if (!ok) return;
        await promote({
          counterparty_key: cpKey,
          counterparty_name: ctx.counterparty,
          direction:
            ctx.direction === "incoming" || ctx.direction === "outgoing"
              ? ctx.direction
              : undefined,
          account_code: code,
        });
        toast.success(
          t("office.review.rule_created", {
            name: ctx.counterparty ?? "",
            code,
          }),
        );
      } catch (err) {
        toast.error(errorText(err));
      } finally {
        setRuleBusy(false);
      }
    },
    [
      current,
      cpKey,
      ruleBusy,
      currentValue.code,
      decide,
      promote,
      errorText,
      t,
    ],
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
    const picked = bulkPicked;
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

  const select = (i: number) => {
    setDirection("skip");
    setCursor(i);
    setPicking(false);
    if (isMobile) {
      setDetailOpen(true);
      requestAnimationFrame(() => detailRef.current?.focus());
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
      } else if (e.key === "ArrowUp") {
        setDirection("skip");
        setCursor((c) => Math.max(c - 1, 0));
      } else if (k === "a" || e.key === "Enter") void decide("accepted");
      else if (k === "r" || e.key === "Backspace") void decide("rejected");
      else if (k === "e") {
        if (canCorrect) setPicking(true);
      } else if (k === "k") {
        if (canRule) void decideWithRule();
      } else if (k === "z" || k === "u") undo();
      else if (e.key === "?") setHelp((h) => !h);
      else return;
      e.preventDefault();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [
    decide,
    decideWithRule,
    undo,
    picking,
    bulkBusy,
    canCorrect,
    canRule,
    queue.length,
  ]);

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
  // Pending beyond the loaded page are counted but not shown until the
  // refetch after each decision pulls them in.
  const left =
    pending.length +
    Math.max((query.data?.total ?? 0) - (query.data?.items.length ?? 0), 0);

  const ctx = current ? suggestionContext(current) : undefined;
  const taxpayer = current?.engagement_uuid
    ? directory.entries.get(current.engagement_uuid)
    : undefined;
  const dirKey = directionKey(ctx?.direction);
  const showDetail = !isMobile || detailOpen;
  const showList = !isMobile || !detailOpen;

  let detail: React.ReactNode;
  if (query.isLoading) {
    detail = (
      <div className="bg-card flex h-full min-h-80 items-center justify-center rounded-[14px] border">
        <Loading label={t("common.loading")} />
      </div>
    );
  } else if (query.isError) {
    detail = (
      <ErrorState
        className="h-full"
        title={t("office.review_api.load_error")}
        description={query.error.message}
        onRetry={() => void query.refetch()}
        retryLabel={t("common.retry")}
      />
    );
  } else if (!current || !ctx) {
    detail = (
      <EmptyState
        className="bg-card h-full min-h-80 rounded-[14px] border"
        title={
          active.length
            ? t("office.studio.done_title")
            : t("office.review_empty")
        }
        description={
          active.length
            ? t("office.studio.done_body", { count: active.length })
            : t("office.review_empty_description")
        }
        action={
          undoTarget ? (
            <Button variant="outline" onClick={undo} disabled={undoBusy}>
              <Undo2 className="size-4" />
              {t("office.studio.undo")}
            </Button>
          ) : undefined
        }
      />
    );
  } else {
    const ruleLabel = t("office.review.rule_for_counterparty");
    detail = (
      <AnimatePresence mode="popLayout" initial={false}>
        <motion.article
          key={current.uuid}
          aria-labelledby="review-detail-title"
          initial={reduce ? { opacity: 0 } : { opacity: 0, y: 16 }}
          animate={{ opacity: 1, y: 0, x: 0 }}
          exit={reduce ? { opacity: 0 } : { opacity: 0, x: exitX }}
          transition={
            reduce
              ? { duration: 0.12 }
              : { type: "spring", stiffness: 260, damping: 26 }
          }
          className="flex flex-col gap-4"
        >
          <div className="flex flex-wrap items-center gap-2 text-xs">
            {isMobile ? (
              <Button
                variant="ghost"
                size="sm"
                className="-ms-2 h-11"
                onClick={() => setDetailOpen(false)}
              >
                <ArrowLeft className="size-4" />
                {t("office.review.back_to_list")}
              </Button>
            ) : null}
            {ctx.invoiceType ? (
              <span className="bg-brand-tint text-brand-text rounded-full px-2.5 py-0.5 font-semibold">
                {ctx.invoiceType}
              </span>
            ) : null}
            <span className="text-muted-foreground font-mono">
              {[dirKey ? t(dirKey) : undefined, current.period]
                .filter(Boolean)
                .join(" · ")}
            </span>
            <TacRozeti
              durum="yuklendi"
              etiket
              metin={t("office.review.status_ready")}
              className="ms-auto text-xs"
            />
          </div>

          <div className="flex flex-wrap items-end justify-between gap-3">
            <div className="min-w-0">
              <h2
                id="review-detail-title"
                ref={detailRef}
                tabIndex={-1}
                className="font-display text-2xl leading-tight font-semibold tracking-tight outline-none"
              >
                {ctx.counterparty ??
                  ctx.itemName ??
                  `${current.target_type}.${current.target_field}`}
              </h2>
              <p className="text-text-2 text-sm">
                {[
                  taxpayer
                    ? t("office.review.taxpayer", { name: taxpayer.title })
                    : undefined,
                ]
                  .filter(Boolean)
                  .join(" · ")}
                {ctx.invoiceUuid ? (
                  <>
                    {taxpayer ? " · " : null}
                    <Link
                      href={routes.tenant.invoice(
                        slug,
                        ctx.invoiceUuid,
                        current.engagement_uuid ?? undefined,
                      )}
                      className="text-brand-text font-medium hover:underline"
                    >
                      {t("office.review_api.invoice_link")}
                    </Link>
                  </>
                ) : null}
              </p>
            </div>
          </div>

          <div className="grid gap-3 xl:grid-cols-2">
            <section className="bg-card rounded-[14px] border p-4">
              <h3 className="mb-2 text-sm font-semibold">
                {t("office.studio.ev_line")}
              </h3>
              <div className="text-base font-medium break-words">
                {ctx.itemName ??
                  (ctx.counterparty
                    ? t("office.review_api.counterparty_account", {
                        name: ctx.counterparty,
                      })
                    : `${current.target_type}.${current.target_field}`)}
              </div>
              {ctx.taxRate || ctx.lineType || ctx.lineNo ? (
                <Figure className="text-muted-foreground mt-1 block text-xs">
                  {[
                    ctx.lineType
                      ? t("office.review_api.line_type", {
                          type: ctx.lineType,
                        })
                      : undefined,
                    ctx.taxRate ? `KDV %${ctx.taxRate}` : undefined,
                    ctx.lineNo
                      ? t("office.review_api.invoice_line", {
                          line: ctx.lineNo,
                        })
                      : undefined,
                  ]
                    .filter(Boolean)
                    .join(" · ")}
                </Figure>
              ) : null}
            </section>

            <section className="bg-card rounded-[14px] border p-4">
              <div className="mb-2 flex items-center gap-2">
                <h3 className="text-sm font-semibold">
                  {t("office.studio.why")}
                </h3>
                <GuvenOlcegi
                  deger={confidenceOf(current)}
                  kaynak={kaynakOf(current)}
                  etiketli
                  className="ms-auto"
                />
              </div>
              <CodePicker
                engagement={current.engagement_uuid ?? undefined}
                open={picking}
                onOpenChange={setPicking}
                onPick={(code, name) => {
                  setPicking(false);
                  const withRule = ruleOnCorrect && canRule;
                  setRuleOnCorrect(false);
                  if (withRule) {
                    void decideWithRule(
                      code === currentValue.code ? undefined : { code, name },
                    );
                  } else if (code === currentValue.code) {
                    void decide("accepted");
                  } else {
                    void decide("corrected", { code, name });
                  }
                }}
                footer={
                  canRule ? (
                    <label className="flex min-h-11 items-center gap-2 text-sm">
                      <Checkbox
                        checked={ruleOnCorrect}
                        onCheckedChange={(v) => setRuleOnCorrect(v === true)}
                      />
                      {ruleLabel}
                    </label>
                  ) : undefined
                }
              >
                <button
                  type="button"
                  onClick={() => canCorrect && setPicking(true)}
                  aria-label={t("office.review.code_button", {
                    code: valueLabel(current.proposed_value),
                  })}
                  className={cn(
                    "group flex w-full flex-wrap items-baseline gap-x-3 rounded-[10px] border border-dashed p-3 text-start transition-colors",
                    canCorrect && "hover:border-brand",
                  )}
                >
                  <Figure className="text-2xl font-semibold break-all">
                    {valueLabel(current.proposed_value)}
                  </Figure>
                  {currentValue.name ? (
                    <span className="text-text-2 text-sm">
                      {currentValue.name}
                    </span>
                  ) : null}
                  {canCorrect ? (
                    <Pencil
                      className="text-muted-foreground ms-auto size-3.5"
                      aria-hidden
                    />
                  ) : null}
                </button>
              </CodePicker>
              <div className="mt-4">
                <EvidenceChain s={current} threshold={threshold} />
              </div>
            </section>
          </div>

          {canCorrect ? (
            <section className="bg-card rounded-[14px] border p-4">
              <h3 className="mb-2 text-sm font-semibold">
                {t("office.studio.ev_history")}
              </h3>
              {history.length ? (
                <ul className="flex flex-wrap gap-2">
                  {history.map(([code, n]) => (
                    <li
                      key={code}
                      className="bg-surface-2 rounded-full border px-2.5 py-1 text-xs"
                    >
                      <Figure className="font-semibold">{code}</Figure>
                      <span className="text-text-2"> · {n}×</span>
                    </li>
                  ))}
                </ul>
              ) : (
                <p className="text-text-2 text-sm">
                  {t("office.review.history_none")}
                </p>
              )}
            </section>
          ) : null}

          <div className="flex flex-wrap items-center gap-2">
            <Button
              onClick={() => void decide("accepted")}
              className="bg-brand text-on-brand hover:bg-brand/90 h-11 px-5"
              disabled={bulkBusy}
            >
              <Check className="size-4" />
              {t("office.approve")}
              <Kbd className="border-on-brand/30 bg-on-brand/15 text-inherit">
                A
              </Kbd>
            </Button>
            <Button
              variant="outline"
              onClick={() => setPicking(true)}
              className="h-11"
              disabled={!canCorrect || bulkBusy}
            >
              <Pencil className="size-4" />
              {t("office.review.correct")} <Kbd>E</Kbd>
            </Button>
            {canRule ? (
              <Button
                variant="outline"
                onClick={() => void decideWithRule()}
                className="h-11"
                disabled={bulkBusy || ruleBusy}
                title={ruleLabel}
              >
                <Wand2 className="size-4" />
                {t("office.review.make_rule")} <Kbd>K</Kbd>
              </Button>
            ) : null}
            <Button
              variant="ghost"
              onClick={() => void decide("rejected")}
              className="h-11"
              disabled={bulkBusy}
            >
              <X className="size-4" />
              {t("office.reject")} <Kbd>R</Kbd>
            </Button>
            <span className="text-muted-foreground ms-auto hidden text-xs md:inline">
              <Kbd>Z</Kbd> {t("office.review.undo_hint")}
            </span>
          </div>
        </motion.article>
      </AnimatePresence>
    );
  }

  return (
    <div className="flex min-h-0 flex-col gap-4">
      <header className="flex flex-wrap items-center gap-3">
        <h1 className="font-display text-2xl font-semibold tracking-tight">
          {t("office.review_title")}
        </h1>
        <span
          className="bg-surface-2 rounded-full border px-2.5 py-0.5 font-mono text-xs font-semibold tabular-nums"
          aria-live="polite"
        >
          {t("office.review_pending", { count: left })}
        </span>
        <div
          role="group"
          aria-label={t("office.review.tier_label")}
          className="flex flex-wrap gap-1"
        >
          {(
            [
              ["all", t("office.filter_all")],
              ["low", t("office.review.tier_low", { count: tierCounts.low })],
              ["mid", t("office.review.tier_mid", { count: tierCounts.mid })],
            ] as const
          ).map(([value, label]) => (
            <Button
              key={value}
              size="sm"
              variant={tier === value ? "secondary" : "ghost"}
              aria-pressed={tier === value}
              className="pointer-coarse:h-11"
              onClick={() => {
                setTier(value);
                setCursor(0);
              }}
            >
              {label}
            </Button>
          ))}
        </div>
        <div
          className={cn(
            "ms-auto flex flex-wrap items-center gap-3",
            !showList && "hidden",
          )}
        >
          <FilterSelect
            label={t("office.col_taxpayer")}
            value={engagement}
            onChange={(v) => {
              setEngagement(v);
              setCursor(0);
            }}
            options={[
              { value: "all", label: t("office.filter_all") },
              ...directory.list.map((x) => ({
                value: x.engagementUuid,
                label: x.title,
              })),
            ]}
          />
          <FilterSelect
            label={t("office.review_api.kind")}
            value={kind}
            onChange={(v) => {
              setKind(v);
              setCursor(0);
            }}
            options={[
              { value: "all", label: t("office.filter_all") },
              ...Array.from(
                new Set([
                  ACCOUNT_KIND,
                  ...(query.data?.items ?? []).map((s) => s.kind),
                ]),
              ).map((k) => ({
                value: k,
                label:
                  k === ACCOUNT_KIND ? t("office.review_api.kind_account") : k,
              })),
            ]}
          />
          {invoiceFilter ? (
            <Button asChild size="sm" variant="ghost">
              <Link href="?">{t("office.clear_invoice_filter")}</Link>
            </Button>
          ) : null}
          <Button
            variant="outline"
            size="sm"
            onClick={undo}
            disabled={!undoTarget || undoBusy}
            title={t("office.review_api.undo_hint")}
            className="pointer-coarse:h-11"
          >
            <Undo2 className="size-4" />
            {t("office.studio.undo")} <Kbd>Z</Kbd>
          </Button>
          <Button
            variant="ghost"
            size="icon"
            onClick={() => setHelp((h) => !h)}
            aria-label={t("office.studio.shortcuts")}
            aria-expanded={help}
            className="pointer-coarse:size-11"
          >
            <Keyboard className="size-4" />
          </Button>
        </div>
      </header>

      {help ? (
        <div className="bg-card text-text-2 flex flex-wrap gap-x-5 gap-y-1 rounded-[10px] border px-4 py-2 text-xs">
          {[
            ["J / ↓", t("office.studio.key_next")],
            ["↑", t("office.studio.key_prev")],
            ["A / ↵", t("office.approve")],
            ["E", t("office.review.correct")],
            ["K", t("office.review.make_rule")],
            ["R / ⌫", t("office.reject")],
            ["Z", t("office.studio.undo")],
            ["?", t("office.studio.shortcuts")],
          ].map(([k, label]) => (
            <span key={k} className="inline-flex items-center gap-1.5">
              <Kbd spoken>{k}</Kbd>
              {label}
            </span>
          ))}
        </div>
      ) : null}

      {queue.length && showList ? (
        <div className="bg-card flex flex-wrap items-center gap-3 rounded-[14px] border px-4 py-3">
          <GuvenOlcegi deger={bulkAt / 100} />
          <p className="min-w-0 flex-1 text-sm">
            {t("office.review.bulk_text", {
              count: bulkPicked.length,
              threshold: bulkAt,
            })}
          </p>
          <div className="flex items-center gap-2">
            <Slider
              className="w-32"
              min={60}
              max={99}
              step={1}
              value={[bulkAt]}
              onValueChange={(v) => setBulkAt(v[0] ?? 95)}
              aria-label={t("office.studio.bulk_label")}
            />
            <Figure className="w-9 text-xs">%{bulkAt}</Figure>
          </div>
          <Button
            size="sm"
            className="bg-brand text-on-brand hover:bg-brand/90 pointer-coarse:h-11"
            disabled={!bulkPicked.length || bulkBusy}
            onClick={() => void bulkApprove()}
          >
            {bulkBusy
              ? t("office.review_api.bulk_running")
              : t("office.review.bulk_action", { count: bulkPicked.length })}
          </Button>
        </div>
      ) : null}

      <div className="grid min-h-0 grid-cols-[minmax(0,1fr)] gap-4 md:grid-cols-[320px_minmax(0,1fr)] xl:grid-cols-[360px_minmax(0,1fr)]">
        {showList ? (
          <section
            aria-labelledby="review-queue-title"
            className="bg-card flex min-h-0 flex-col rounded-[14px] border md:max-h-[calc(100svh-15rem)]"
          >
            <div className="text-muted-foreground flex items-center gap-1.5 border-b px-4 py-2 text-xs">
              <h2 id="review-queue-title" className="flex-1 font-medium">
                {t("office.review.sorted_low_first")}
              </h2>
              <Kbd>J</Kbd>
              <Kbd>↑</Kbd>
            </div>
            {query.isLoading ? (
              <Loading className="m-4" label={t("common.loading")} />
            ) : query.isError && !isMobile ? (
              // #332: geniş ekranda hatanın ayrıntısı sağ panelde; liste
              // "Bekleyen öneri yok" diyerek yanıltmaz.
              <p className="text-danger p-4 text-sm">
                {t("office.review_api.load_error")}
              </p>
            ) : query.isError ? (
              // Telefonda ayrıntı paneli yok: hata listede gösterilir.
              <ErrorState
                className="m-3 border-0 py-8"
                title={t("office.review_api.load_error")}
                description={query.error.message}
                onRetry={() => void query.refetch()}
                retryLabel={t("common.retry")}
              />
            ) : queue.length === 0 ? (
              <p className="text-text-2 p-4 text-sm">
                {t("office.review_empty")}
              </p>
            ) : (
              <div ref={listRef} className="min-h-0 overflow-y-auto p-1">
                <ul aria-label={t("office.studio.queue")}>
                  {queue.map((s, i) => {
                    const c = suggestionContext(s);
                    const on = i === index;
                    const title =
                      c.itemName ??
                      (c.counterparty
                        ? t("office.review_api.counterparty_account", {
                            name: c.counterparty,
                          })
                        : s.target_field);
                    return (
                      <li key={s.uuid} data-index={i}>
                        <button
                          type="button"
                          aria-current={on ? "true" : undefined}
                          onClick={() => select(i)}
                          className={cn(
                            "flex min-h-14 w-full items-center gap-3 rounded-[10px] px-3 py-2 text-start transition-colors",
                            on
                              ? "bg-brand-tint shadow-[inset_3px_0_0_var(--color-brand)]"
                              : "hover:bg-surface-2",
                          )}
                        >
                          <GuvenOlcegi
                            deger={confidenceOf(s)}
                            kaynak={kaynakOf(s)}
                          />
                          <span className="min-w-0 flex-1">
                            <span className="block truncate text-sm font-semibold">
                              {title}
                            </span>
                            <span className="text-text-2 flex gap-2 text-xs">
                              <span className="min-w-0 flex-1 truncate">
                                {[
                                  c.itemName ? c.counterparty : undefined,
                                  s.engagement_uuid
                                    ? directory.entries.get(s.engagement_uuid)
                                        ?.title
                                    : undefined,
                                ]
                                  .filter(Boolean)
                                  .join(" · ")}
                              </span>
                              <Figure className="shrink-0 font-semibold">
                                {valueLabel(s.proposed_value)}
                              </Figure>
                            </span>
                          </span>
                        </button>
                      </li>
                    );
                  })}
                </ul>
              </div>
            )}
          </section>
        ) : null}

        {showDetail ? <div className="min-h-0 min-w-0">{detail}</div> : null}
      </div>

      {log.length ? (
        <section aria-labelledby="review-recent-title" className="text-xs">
          <h2
            id="review-recent-title"
            className="text-muted-foreground mb-1 font-medium"
          >
            {t("office.studio.recent")}
          </h2>
          <ul className="flex flex-wrap gap-x-4 gap-y-1">
            {log.slice(0, 8).map((d) => (
              <li
                key={d.suggestion.uuid}
                className={cn(
                  "flex items-center gap-1.5",
                  (d.reverted || !d.settled) && "text-muted-foreground",
                )}
              >
                {d.reverted ? (
                  <Undo2 className="size-3.5" aria-hidden />
                ) : d.kind === "rejected" ? (
                  <X className="text-danger size-3.5" aria-hidden />
                ) : (
                  <Check className="text-ok size-3.5" aria-hidden />
                )}
                <Figure className={cn(d.reverted && "line-through")}>
                  {d.code}
                </Figure>
                <span>
                  {d.reverted
                    ? t("office.review_api.status_reverted")
                    : d.kind === "corrected"
                      ? t("office.review_api.status_corrected")
                      : d.kind === "rejected"
                        ? t("office.reject")
                        : t("office.approve")}
                </span>
              </li>
            ))}
          </ul>
        </section>
      ) : null}
    </div>
  );
}

/**
 * İnceleme kuyruğu ve stüdyosu tek ekran (#326, OfisKuyruk): solda en düşük
 * güven önce kuyruk, sağda kaynak satır, "Neden bu kod?", karşı taraf geçmişi
 * ve A / E / K / R eylemleri; Z son kararı geri alır.
 */
export function ReviewWorkspace() {
  return (
    <Suspense>
      <ReviewWorkspaceInner />
    </Suspense>
  );
}
