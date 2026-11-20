"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";

import {
  legalDocumentService,
  legalKeys,
  myLegalService,
  officeDpaService,
  type LegalKind,
  type LegalLanguage,
} from "@/features/legal/services/legal.service";
import { queryOrNull } from "@/lib/query/or-null";
import { useAppMutation } from "@/lib/query/mutation";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/** Current version of every text, in the app language. */
export function useLegalDocuments() {
  const { locale } = useLocale();
  return useQuery({
    queryKey: legalKeys.documents(locale),
    queryFn: () => legalDocumentService.list(locale),
    staleTime: 5 * 60_000,
    meta: { silent: true },
  });
}

/**
 * One text: the current version (default) or `version`. `lang` defaults to
 * the app language; the API falls back to Turkish (the binding text).
 */
export function useLegalDocument(
  kind: LegalKind,
  opts: { version?: number; lang?: string; enabled?: boolean } = {},
) {
  const { locale } = useLocale();
  const lang = opts.lang ?? locale;
  return useQuery({
    queryKey: legalKeys.document(kind, lang, opts.version),
    queryFn: () =>
      legalDocumentService.get(kind, { lang, version: opts.version }),
    enabled: opts.enabled ?? true,
    retry: false,
    // A published version never changes; the current one rarely does and a
    // stale version is caught by the 409 on submit.
    staleTime: opts.version ? Infinity : 5 * 60_000,
    meta: { silent: true },
  });
}

export function useMyLegalAcceptances(enabled = true) {
  return useQuery({
    queryKey: legalKeys.myAcceptances(),
    queryFn: () => myLegalService.acceptances(),
    enabled,
    meta: { silent: true },
  });
}

export function useOfficeDpa(slug: string, enabled = true) {
  const { locale } = useLocale();
  return useQuery({
    queryKey: legalKeys.officeDpa(slug, locale),
    // Sözleşme henüz yayımlanmadıysa uç 404 döner: hata değil, boş durum
    // (#639). `null` önbelleğe alınır, kart "yayımlanmadı" der.
    queryFn: () => queryOrNull(() => officeDpaService.get(slug, locale)),
    enabled,
    retry: false,
    meta: { silent: true },
  });
}

export function useAcceptOfficeDpa(slug: string) {
  const { t } = useLocale();
  const queryClient = useQueryClient();
  return useAppMutation({
    mutationFn: (body: { version: number; language?: LegalLanguage }) =>
      officeDpaService.accept(slug, body),
    onSuccess: () => appToast.success(t("legal.dpa_toast_accepted")),
    onSettled: () =>
      void queryClient.invalidateQueries({
        queryKey: legalKeys.officeDpaAll(slug),
      }),
  });
}
