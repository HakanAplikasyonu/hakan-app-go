/** Türkiye saatinde (Europe/Istanbul) şu anki saat, 0–23. */
export function istanbulHour(now: Date): number {
  return Number(
    new Intl.DateTimeFormat("en-GB", {
      timeZone: "Europe/Istanbul",
      hour: "2-digit",
      hourCycle: "h23",
    }).format(now),
  );
}

/**
 * Ana sayfa selamlaması, kullanıcının cihaz saatine değil Türkiye saatine
 * göre: 05–12 Günaydın, 12–18 İyi günler, 18–22 İyi akşamlar, 22–05 İyi
 * geceler.
 */
export function greetingKey(now: Date = new Date()): string {
  const h = istanbulHour(now);
  if (h >= 5 && h < 12) return "office.home.greeting_morning";
  if (h >= 12 && h < 18) return "office.home.greeting_day";
  if (h >= 18 && h < 22) return "office.home.greeting_evening";
  return "office.home.greeting_night";
}
