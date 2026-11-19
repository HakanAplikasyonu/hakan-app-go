/**
 * Önceki sonucu yalnızca aynı kapsamda (ör. aynı mükellef) tutar (#572).
 * `keepPreviousData` sayfa ya da arama değişince listeyi titretmez; ama
 * mükellef değişince yeni veri gelene kadar ÖNCEKİ mükellefin verisini
 * gösterir: hesap planından yanlış mükellefin kodu seçilebilir. Önceki
 * sorgunun anahtarı aynı kapsamı içermiyorsa yer tutucu verilmez.
 */
export function keepPreviousWithin(scope: string | undefined) {
  return <T>(
    previous: T | undefined,
    previousQuery?: { queryKey: readonly unknown[] },
  ): T | undefined => {
    // Anahtarlar kimliği küçük harfle tutar.
    const want = scope?.toLowerCase();
    return want &&
      previousQuery?.queryKey.some(
        (k) => typeof k === "string" && k.toLowerCase() === want,
      )
      ? previous
      : undefined;
  };
}
