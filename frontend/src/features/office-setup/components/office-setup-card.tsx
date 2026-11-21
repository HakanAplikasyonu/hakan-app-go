"use client";

import { ArrowRight, ListChecks } from "lucide-react";
import Link from "next/link";

import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import { useOfficeSetup } from "../hooks/use-office-setup";
import { useSetupCardMinimized } from "../hooks/use-setup-card-minimized";
import { SETUP_STEPS, settledCount } from "../lib/steps";
import { trPossessive } from "@/lib/utils";

/**
 * Office home: a non-blocking, one-line reminder while the setup wizard is
 * not finished. It sits below the day's work (#710) and can be minimized to
 * a small link. Renders nothing without `tenant.settings.write`, while
 * loading, once the setup is complete, or when the state cannot be read.
 */
export function OfficeSetupCard() {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const allowed = can(permissions.tenant.settings.write);
  const query = useOfficeSetup(slug, { enabled: allowed, silent: true });
  const [minimized, setMinimized] = useSetupCardMinimized(slug);
  const state = query.data;
  if (!allowed || query.isError || !state || state.completed_at) return null;

  const done = settledCount(state);
  const progress = t("setup.card_progress", {
    total: SETUP_STEPS.length,
    done: locale === "tr" ? trPossessive(done) : done,
  });

  if (minimized) {
    return (
      <div className="text-text-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-sm">
        <Link
          href={routes.tenant.setup(slug)}
          className="text-brand-text inline-flex min-h-6 items-center gap-1.5 font-medium hover:underline max-md:min-h-11"
        >
          <ListChecks className="size-4" aria-hidden />
          {t("setup.card_minimized", { progress })}
        </Link>
        <Button
          type="button"
          size="sm"
          variant="link"
          className="text-text-2 h-auto p-0 text-xs max-md:min-h-11"
          onClick={() => setMinimized(false)}
        >
          {t("setup.card_expand")}
        </Button>
      </div>
    );
  }

  return (
    <section
      aria-labelledby="office-setup-card-title"
      className="bg-card text-card-foreground flex flex-wrap items-center gap-x-3 gap-y-2 rounded-lg border px-3 py-2"
    >
      <ListChecks className="text-primary size-4 shrink-0" aria-hidden />
      <h2 id="office-setup-card-title" className="text-sm font-semibold">
        {t("setup.card_title")}
      </h2>
      <p className="text-text-2 min-w-0 text-sm">
        {progress} · {t("setup.card_why")}
      </p>
      <div className="ml-auto flex items-center gap-2">
        <Button
          type="button"
          size="sm"
          variant="ghost"
          onClick={() => setMinimized(true)}
        >
          {t("setup.card_minimize")}
        </Button>
        <Button asChild size="sm" variant="outline">
          <Link href={routes.tenant.setup(slug)}>
            {t("setup.card_continue")}
            <ArrowRight className="size-4" aria-hidden />
          </Link>
        </Button>
      </div>
    </section>
  );
}
