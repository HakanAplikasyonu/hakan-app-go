"use client";

import type { HelpGuide } from "../lib/guides";
import { HelpGuidePage } from "./help-guide-page";
import { HelpHome } from "./help-home";

/**
 * `/yardim` ve `/yardim/{kılavuz}` (#274, #522): yardım merkezi. Ana sayfada
 * arama, role göre kılavuzlar ve sık bakılan konular; kılavuz sayfasında
 * içindekiler, bölümler ve önceki / sonraki gezinme. Kılavuzlar yalnızca
 * Türkçedir; çerçeve dile uyar.
 */
export function HelpPage({ guide }: { guide?: HelpGuide }) {
  return (
    <div className="mx-auto flex min-h-svh w-full max-w-6xl flex-col gap-6 px-4 py-8 sm:px-6">
      {guide ? <HelpGuidePage guide={guide} /> : <HelpHome />}
    </div>
  );
}
