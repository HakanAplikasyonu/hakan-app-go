"use client";

import { useQuery } from "@tanstack/react-query";
import { Check, Copy, Download, ExternalLink } from "lucide-react";
import { useEffect, useRef, type ReactNode } from "react";

import { EntitySectionCard } from "@/components/entity";
import { Button } from "@/components/ui/button";
import { useLocalStorage } from "@/hooks/use-local-storage";
import { cn } from "@/lib/utils";
import { fileSize, relativeDatetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";
import { authService } from "@/services/auth.service";

import {
  CHROME_EXTENSIONS_URL,
  clampStep,
  EXTENSION_INFO_URL,
  EXTENSION_ZIP_URL,
  extensionStatus,
  parsePackageInfo,
  SETUP_STEPS,
  stepStates,
  type SetupStep,
} from "../lib/whatsapp-setup";

const STEP_KEY = "easytrade.whatsapp.setup-step";
const WHATSAPP_WEB = "https://web.whatsapp.com/";

/**
 * Kolay kurulum (#780): 1. İndir · 2. Chrome'a ekle · 3. Eşleştir · 4. Dene.
 * Seçili adım localStorage'da kalır; uzantının durumu hesabın oturum
 * listesinden (uzantı oturumu) canlı okunur.
 */
export function WhatsAppSetup({ pairing }: { pairing: ReactNode }) {
  const { t, locale } = useLocale();
  const [stored, setStored] = useLocalStorage<number>(STEP_KEY, 0);
  const step = clampStep(stored);

  const sessions = useQuery({
    queryKey: ["auth", "sessions", "whatsapp-extension"],
    queryFn: () => authService.listSessions(),
    // Eşleşme beklenirken sık, bağlıyken seyrek yoklanır.
    refetchInterval: (q) =>
      q.state.data && extensionStatus(q.state.data.items).connected
        ? 60_000
        : 5_000,
  });
  const status = extensionStatus(sessions.data?.items ?? []);

  // Eşleşme bu sayfadayken olursa "Dene" adımına geçilir.
  const wasConnected = useRef<boolean | null>(null);
  useEffect(() => {
    if (!sessions.data) return;
    if (wasConnected.current === false && status.connected) setStored(3);
    wasConnected.current = status.connected;
  }, [sessions.data, status.connected, setStored]);

  const states = stepStates(step, status.connected);

  return (
    <EntitySectionCard title={t("office.whatsapp.setup_title")}>
      <div className="flex flex-col gap-5">
        <StatusLine
          status={status}
          locale={locale}
          loading={sessions.isLoading}
        />
        <ol
          aria-label={t("office.whatsapp.setup_steps_aria")}
          className="grid grid-cols-2 gap-2 md:grid-cols-4"
        >
          {SETUP_STEPS.map((key, i) => (
            <li key={key}>
              <Button
                type="button"
                variant={i === step ? "default" : "outline"}
                className="h-auto w-full justify-start gap-2 py-3 text-left whitespace-normal"
                aria-current={i === step ? "step" : undefined}
                data-state={states[i]}
                onClick={() => setStored(i)}
              >
                <span
                  aria-hidden
                  className={cn(
                    "flex size-6 shrink-0 items-center justify-center rounded-full border text-xs font-semibold",
                    states[i] === "done" &&
                      "bg-ok border-transparent text-white",
                  )}
                >
                  {states[i] === "done" ? (
                    <Check className="size-3.5" />
                  ) : (
                    i + 1
                  )}
                </span>
                <span className="font-medium">
                  {i + 1}. {t(`office.whatsapp.setup_step_${key}`)}
                </span>
              </Button>
            </li>
          ))}
        </ol>

        <div className="grid gap-6 md:grid-cols-[minmax(0,1fr)_260px]">
          <div className="min-w-0 space-y-4" data-testid="whatsapp-setup-step">
            <h3 className="text-base font-semibold">
              {step + 1}. {t(`office.whatsapp.setup_step_${SETUP_STEPS[step]}`)}
            </h3>
            {SETUP_STEPS[step] === "download" ? <DownloadStep /> : null}
            {SETUP_STEPS[step] === "install" ? <InstallStep /> : null}
            {SETUP_STEPS[step] === "pair" ? (
              <div className="space-y-3">
                <p className="text-text-2 text-sm">
                  {t("office.whatsapp.setup_pair_text")}
                </p>
                {pairing}
              </div>
            ) : null}
            {SETUP_STEPS[step] === "test" ? <TestStep /> : null}
          </div>
          <StepArt step={SETUP_STEPS[step]!} />
        </div>

        <div className="flex justify-between gap-2">
          <Button
            type="button"
            variant="outline"
            disabled={step === 0}
            onClick={() => setStored(step - 1)}
          >
            {t("office.whatsapp.setup_back")}
          </Button>
          {step < SETUP_STEPS.length - 1 ? (
            <Button type="button" onClick={() => setStored(step + 1)}>
              {t("office.whatsapp.setup_next")}
            </Button>
          ) : null}
        </div>
      </div>
    </EntitySectionCard>
  );
}

function StatusLine({
  status,
  locale,
  loading,
}: {
  status: ReturnType<typeof extensionStatus>;
  locale: "tr" | "en";
  loading: boolean;
}) {
  const { t } = useLocale();
  if (loading) return null;
  return (
    <p
      role="status"
      data-testid="whatsapp-extension-status"
      className={cn(
        "rounded-md px-4 py-3 text-sm font-medium",
        status.connected ? "bg-ok-tint text-ok" : "bg-warn-tint text-warn",
      )}
    >
      {status.connected
        ? t("office.whatsapp.setup_connected", {
            when: relativeDatetime(status.lastSeen, locale),
          })
        : t("office.whatsapp.setup_waiting")}
    </p>
  );
}

function DownloadStep() {
  const { t } = useLocale();
  const info = useQuery({
    queryKey: ["whatsapp-extension-package"],
    queryFn: async () => {
      const r = await fetch(EXTENSION_INFO_URL, { cache: "no-store" });
      if (!r.ok) return null;
      return parsePackageInfo(await r.json().catch(() => null));
    },
    retry: false,
    staleTime: 5 * 60_000,
  });
  const pkg = info.data ?? null;
  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-4">
        {pkg ? (
          <Button asChild size="lg" className="h-14 px-8 text-base">
            <a href={EXTENSION_ZIP_URL} download="easytrade-whatsapp.zip">
              <Download className="size-5" aria-hidden />
              {t("office.whatsapp.setup_download")}
            </a>
          </Button>
        ) : (
          <Button size="lg" className="h-14 px-8 text-base" disabled>
            <Download className="size-5" aria-hidden />
            {t("office.whatsapp.setup_download")}
          </Button>
        )}
        <span
          className="text-text-2 text-sm"
          data-testid="whatsapp-package-info"
        >
          {info.isLoading
            ? null
            : pkg
              ? t("office.whatsapp.setup_package", {
                  version: pkg.version,
                  size: fileSize(pkg.size),
                })
              : t("office.whatsapp.setup_package_missing")}
        </span>
      </div>
      <ol className="text-text-2 list-decimal space-y-1 pl-5 text-sm">
        <li>{t("office.whatsapp.setup_download_1")}</li>
        <li>{t("office.whatsapp.setup_download_2")}</li>
      </ol>
    </div>
  );
}

function InstallStep() {
  const { t } = useLocale();
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(CHROME_EXTENSIONS_URL);
      appToast.success(t("office.whatsapp.setup_copied"));
    } catch {
      appToast.error(t("office.whatsapp.setup_copy_failed"));
    }
  };
  return (
    <ol className="text-text-2 list-decimal space-y-3 pl-5 text-sm">
      <li className="space-y-2">
        <span>{t("office.whatsapp.setup_install_1")}</span>
        <span className="flex flex-wrap items-center gap-2">
          <code
            data-testid="whatsapp-chrome-url"
            className="bg-muted text-foreground rounded-md px-3 py-1.5 font-mono text-sm"
          >
            {CHROME_EXTENSIONS_URL}
          </code>
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => void copy()}
          >
            <Copy className="size-4" aria-hidden />
            {t("office.whatsapp.setup_copy")}
          </Button>
        </span>
      </li>
      <li>{t("office.whatsapp.setup_install_2")}</li>
      <li>{t("office.whatsapp.setup_install_3")}</li>
      <li>{t("office.whatsapp.setup_install_4")}</li>
    </ol>
  );
}

function TestStep() {
  const { t } = useLocale();
  return (
    <div className="space-y-3">
      <ol className="text-text-2 list-decimal space-y-1 pl-5 text-sm">
        <li>{t("office.whatsapp.setup_test_1")}</li>
        <li>{t("office.whatsapp.setup_test_2")}</li>
        <li>{t("office.whatsapp.setup_test_3")}</li>
      </ol>
      <Button asChild variant="outline">
        <a href={WHATSAPP_WEB} target="_blank" rel="noreferrer">
          <ExternalLink className="size-4" aria-hidden />
          {t("office.whatsapp.setup_open_whatsapp")}
        </a>
      </Button>
    </div>
  );
}

/**
 * Adım görseli: Chrome arayüzünün şeması (ekran görüntüsü değil). Renkler
 * tema belirteçlerinden; süsleyici (aria-hidden), anlatım yandaki metinde.
 */
function StepArt({ step }: { step: SetupStep }) {
  return (
    <svg
      viewBox="0 0 260 180"
      aria-hidden
      data-testid={`whatsapp-art-${step}`}
      className="border-border bg-surface text-text-2 hidden w-full rounded-lg border md:block"
    >
      {/* Tarayıcı çerçevesi */}
      <rect
        x="10"
        y="10"
        width="240"
        height="160"
        rx="8"
        fill="none"
        stroke="currentColor"
        strokeOpacity=".35"
      />
      <line
        x1="10"
        y1="34"
        x2="250"
        y2="34"
        stroke="currentColor"
        strokeOpacity=".35"
      />
      <circle cx="22" cy="22" r="3" fill="currentColor" fillOpacity=".35" />
      <circle cx="32" cy="22" r="3" fill="currentColor" fillOpacity=".35" />
      <circle cx="42" cy="22" r="3" fill="currentColor" fillOpacity=".35" />
      <rect
        x="54"
        y="16"
        width="150"
        height="12"
        rx="6"
        fill="currentColor"
        fillOpacity=".12"
      />
      {step === "download" ? (
        <g>
          <rect
            x="95"
            y="60"
            width="70"
            height="80"
            rx="6"
            fill="#2448FF"
            fillOpacity=".12"
            stroke="#2448FF"
          />
          <path
            d="M130 75 v35 m-12 -12 l12 12 l12 -12"
            stroke="#2448FF"
            strokeWidth="4"
            fill="none"
            strokeLinecap="round"
            strokeLinejoin="round"
          />
          <text
            x="130"
            y="132"
            textAnchor="middle"
            fontSize="11"
            fill="#2448FF"
            fontWeight="600"
          >
            .zip
          </text>
        </g>
      ) : null}
      {step === "install" ? (
        <g>
          <text x="60" y="25" fontSize="8" fill="currentColor">
            chrome://extensions
          </text>
          <rect
            x="150"
            y="44"
            width="90"
            height="16"
            rx="8"
            fill="currentColor"
            fillOpacity=".08"
          />
          <rect x="216" y="47" width="20" height="10" rx="5" fill="#1f9d55" />
          <circle cx="231" cy="52" r="4" fill="#fff" />
          <rect
            x="20"
            y="70"
            width="110"
            height="18"
            rx="4"
            fill="#2448FF"
            fillOpacity=".15"
            stroke="#2448FF"
          />
          <rect
            x="20"
            y="100"
            width="220"
            height="56"
            rx="6"
            fill="none"
            stroke="currentColor"
            strokeOpacity=".35"
          />
          <rect x="30" y="112" width="24" height="24" rx="5" fill="#2448FF" />
          <rect
            x="64"
            y="114"
            width="100"
            height="8"
            rx="4"
            fill="currentColor"
            fillOpacity=".3"
          />
          <rect
            x="64"
            y="128"
            width="70"
            height="6"
            rx="3"
            fill="currentColor"
            fillOpacity=".18"
          />
          <path
            d="M214 16 h10 v4 a3 3 0 1 1 0 6 v4 h-10 v-4 a3 3 0 1 0 0 -6 z"
            fill="currentColor"
            fillOpacity=".6"
          />
        </g>
      ) : null}
      {step === "pair" ? (
        <g>
          <rect
            x="120"
            y="40"
            width="120"
            height="120"
            rx="8"
            fill="currentColor"
            fillOpacity=".05"
            stroke="currentColor"
            strokeOpacity=".4"
          />
          <rect x="132" y="54" width="18" height="18" rx="4" fill="#2448FF" />
          <rect
            x="132"
            y="86"
            width="96"
            height="22"
            rx="5"
            fill="none"
            stroke="#2448FF"
            strokeWidth="1.5"
          />
          <text
            x="180"
            y="101"
            textAnchor="middle"
            fontSize="11"
            fontFamily="monospace"
            fill="currentColor"
          >
            K7QM-3XPA
          </text>
          <rect x="132" y="120" width="96" height="22" rx="5" fill="#2448FF" />
          <text
            x="180"
            y="135"
            textAnchor="middle"
            fontSize="10"
            fill="#fff"
            fontWeight="600"
          >
            Eşleştir
          </text>
          <rect
            x="20"
            y="60"
            width="86"
            height="30"
            rx="5"
            fill="currentColor"
            fillOpacity=".1"
          />
          <text
            x="63"
            y="79"
            textAnchor="middle"
            fontSize="10"
            fontFamily="monospace"
            fill="currentColor"
          >
            K7QM-3XPA
          </text>
          <path
            d="M108 75 h10"
            stroke="currentColor"
            strokeWidth="2"
            markerEnd="none"
          />
        </g>
      ) : null}
      {step === "test" ? (
        <g>
          <rect
            x="20"
            y="46"
            width="220"
            height="110"
            rx="6"
            fill="#25D366"
            fillOpacity=".08"
          />
          <rect
            x="34"
            y="62"
            width="120"
            height="22"
            rx="6"
            fill="currentColor"
            fillOpacity=".12"
          />
          <rect
            x="100"
            y="96"
            width="126"
            height="22"
            rx="6"
            fill="#25D366"
            fillOpacity=".3"
          />
          <circle cx="130" cy="138" r="12" fill="#1f9d55" />
          <path
            d="M124 138 l4 4 l8 -8"
            stroke="#fff"
            strokeWidth="2.5"
            fill="none"
            strokeLinecap="round"
          />
        </g>
      ) : null}
    </svg>
  );
}
