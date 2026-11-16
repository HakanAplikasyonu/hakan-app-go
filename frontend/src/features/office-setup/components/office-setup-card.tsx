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
import { SETUP_STEPS, settledCount } from "../lib/steps";
import { trPossessive } from "../lib/format";

/**
 * Office home: a non-blocking reminder while the setup wizard is not
 * finished. Renders nothing without `tenant.settings.write`, while
 * loading, once the setup is complete, or when the state cannot be read.
 */
export function OfficeSetupCard() {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const allowed = can(permissions.tenant.settings.write);
  const query = useOfficeSetup(slug, { enabled: allowed, silent: true });
  const state = query.data;
  if (!allowed || query.isError || !state || state.completed_at) return null;

  const done = settledCount(state);
  return (
    <section
      aria-labelledby="office-setup-card-title"
      className="bg-card text-card-foreground flex flex-wrap items-center justify-between gap-3 rounded-xl border p-4"
    >
      <div className="flex items-start gap-3">
        <ListChecks
          className="text-primary mt-0.5 size-5 shrink-0"
          aria-hidden
        />
        <div>
          <h2 id="office-setup-card-title" className="text-sm font-semibold">
            {t("setup.card_title")}
          </h2>
          <p className="text-muted-foreground text-sm">
            {t("setup.card_progress", {
              total: SETUP_STEPS.length,
              done: locale === "tr" ? trPossessive(done) : done,
            })}
          </p>
        </div>
      </div>
      <Button asChild size="sm">
        <Link href={routes.tenant.setup(slug)}>
          {t("setup.card_continue")}
          <ArrowRight className="size-4" aria-hidden />
        </Link>
      </Button>
    </section>
  );
}
