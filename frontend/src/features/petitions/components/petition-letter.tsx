"use client";

import type { RenderedPetition } from "@/features/petitions/services/petitions.service";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

/**
 * A rendered petition laid out like the printed letter: date on the right,
 * addressee centred in bold, "Konu:", body paragraphs, then the petitioner
 * with the stamp/signature line and who prepared it.
 */
export function PetitionLetter({
  petition,
  className,
}: {
  petition: RenderedPetition;
  className?: string;
}) {
  const { t } = useLocale();
  const paragraphs = petition.body
    .split("\n")
    .map((p) => p.trim())
    .filter(Boolean);

  return (
    <article
      className={cn(
        "bg-background text-foreground mx-auto w-full max-w-[42rem] rounded-md border px-6 py-8 font-serif text-[0.95rem] leading-relaxed shadow-sm sm:px-10",
        className,
      )}
      aria-label={petition.title}
    >
      <p className="text-end tabular-nums">{petition.date}</p>
      <p className="mt-6 text-center font-bold whitespace-pre-line uppercase">
        {petition.recipient}
      </p>
      <p className="mt-6">
        <span className="font-semibold">{t("petitions.letter_subject")}</span>{" "}
        {petition.subject}
      </p>
      <div className="mt-4 space-y-3">
        {paragraphs.map((p, i) => (
          <p key={i} className="text-justify indent-8">
            {p}
          </p>
        ))}
      </div>
      <div className="mt-10 flex justify-end">
        <div className="min-w-[14rem] text-center">
          <p className="font-semibold whitespace-pre-line">
            {petition.petitioner}
          </p>
          <p className="text-muted-foreground mt-8 border-t pt-1 text-sm">
            {t("petitions.letter_stamp_signature")}
          </p>
        </div>
      </div>
      <p className="text-muted-foreground mt-8 text-sm">
        {t("petitions.letter_prepared_by", { signer: petition.signer })}
      </p>
    </article>
  );
}
