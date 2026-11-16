"use client";

import { useCallback, useState } from "react";

import { useLegalDocument } from "@/features/legal/hooks/use-legal";
import type {
  LegalKind,
  LegalLanguage,
} from "@/features/legal/services/legal.service";

/**
 * State of a "I have read and accept" step in a flow: the current text, the
 * checkbox, and what to send (`version` + `language` of the text shown).
 * On a 409 LEGAL_VERSION_OUTDATED call `markOutdated()`: the text is
 * refetched, the box unticked and the flow asks again.
 */
export function useLegalConsent(kind: LegalKind, enabled = true) {
  const query = useLegalDocument(kind, { enabled });
  const [checked, setCheckedState] = useState(false);
  const [outdated, setOutdated] = useState(false);

  const setChecked = useCallback((value: boolean) => {
    setCheckedState(value);
    if (value) setOutdated(false);
  }, []);

  const { refetch } = query;
  const markOutdated = useCallback(() => {
    setCheckedState(false);
    setOutdated(true);
    void refetch();
  }, [refetch]);

  const reset = useCallback(() => {
    setCheckedState(false);
    setOutdated(false);
  }, []);

  const document = query.data;
  const acceptance: { version: number; language: LegalLanguage } | null =
    document && checked
      ? { version: document.version, language: document.language }
      : null;

  return {
    kind,
    query,
    document,
    checked,
    setChecked,
    outdated,
    markOutdated,
    reset,
    /** Non-null once the box is ticked for a loaded text. */
    acceptance,
  };
}

export type LegalConsentState = ReturnType<typeof useLegalConsent>;
