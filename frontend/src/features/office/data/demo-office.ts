/**
 * Komuta merkezi (#34) için 100 mükellefli sentetik demo ofis. Kabul kriteri
 * ("100 mükelleflik demo ofiste ekran 1 sn altında") bu veriyle ölçülür.
 *
 * Sabit tohumlu üreteçle her çağrıda aynı ofis çıkar. Unvanlar uydurma
 * kelime birleşimleridir; VKN'ler "9" ile başlayan test numaralarıdır, gerçek
 * bir mükellefe ait değildir. Belge sayımları doğrudan `PeriodFacts` olarak
 * üretilir (100 × 6 dönem için binlerce sahte fatura nesnesi kurulmaz).
 */

import {
  key,
  periodsEndingAt,
  type PeriodFacts,
  type PeriodWorkflow,
} from "../lib/command";
import type { OfficeMember, Taxpayer } from "../types";

export type DemoOffice = {
  taxpayers: Taxpayer[];
  members: OfficeMember[];
  facts: Map<string, PeriodFacts>;
  workflow: PeriodWorkflow;
};

function mulberry32(seed: number) {
  let a = seed;
  return () => {
    a = (a + 0x6d2b79f5) | 0;
    let t = Math.imul(a ^ (a >>> 15), 1 | a);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

const first = [
  "Anka",
  "Bora",
  "Çınar",
  "Defne",
  "Ege",
  "Fırat",
  "Gediz",
  "Hazar",
  "Ilgaz",
  "Kuzey",
  "Lale",
  "Meltem",
  "Nehir",
  "Orkide",
  "Poyraz",
  "Rüzgar",
  "Sedir",
  "Toros",
  "Umut",
  "Vadi",
  "Yamaç",
  "Zeytin",
];
const second = [
  "Gıda",
  "Tekstil",
  "Lojistik",
  "Yapı",
  "Enerji",
  "Otomotiv",
  "Turizm",
  "Kırtasiye",
  "Mobilya",
  "Tarım",
  "Medikal",
  "Yazılım",
  "Ambalaj",
  "Kimya",
];
const suffix = ["Ltd. Şti.", "A.Ş.", "Ticaret Ltd. Şti.", "San. ve Tic. A.Ş."];
const people = [
  "Aylin Er",
  "Barış Tan",
  "Cemre Uz",
  "Deniz Ak",
  "Emre Su",
  "Gül Ay",
  "Hakan Öz",
  "İpek Gür",
  "Kerem Dal",
  "Merve Işık",
];
const offices = [
  "Kadıköy",
  "Çankaya",
  "Konak",
  "Nilüfer",
  "Muratpaşa",
  "Seyhan",
];

export const DEMO_OFFICE_MEMBERS: OfficeMember[] = [
  { id: "demo-member-ayse", name: "Ayşe Demir", role: "Müşavir" },
  { id: "demo-member-can", name: "Can Kaya", role: "Stajyer" },
  { id: "demo-member-elif", name: "Elif Şahin", role: "Müşavir" },
  { id: "demo-member-murat", name: "Murat Çelik", role: "Müşavir" },
];

export function buildDemoOffice(
  endPeriod: string,
  size = 100,
  periodCount = 6,
): DemoOffice {
  const rnd = mulberry32(34);
  const pick = <T>(xs: T[]) => xs[Math.floor(rnd() * xs.length)];
  const periods = periodsEndingAt(endPeriod, periodCount);
  const taxpayers: Taxpayer[] = [];
  const facts = new Map<string, PeriodFacts>();
  const workflow: PeriodWorkflow = {};

  for (let i = 0; i < size; i++) {
    const person = rnd() < 0.2;
    const id = `demo-office-${String(i + 1).padStart(3, "0")}`;
    const title = person
      ? `${pick(people)} (${i + 1})`
      : `${pick(first)} ${pick(second)} ${pick(suffix)}`;
    const assignees: string[] = [];
    const r = rnd();
    if (r < 0.9) assignees.push(DEMO_OFFICE_MEMBERS[i % 4].id);
    if (r < 0.15) assignees.push("demo-member-can");
    taxpayers.push({
      id,
      taxId: person
        ? `1${String(9000000000 + i * 7919).slice(0, 10)}`
        : String(9100000000 + i * 104729).slice(0, 10),
      title,
      taxOffice: pick(offices),
      kind: person ? "person" : "company",
      assigneeIds: [...new Set(assignees)],
      startedAt: "2025-01-01",
    });

    // Yeni başlayan mükellefin eski dönemleri boş kalır.
    const startIdx = rnd() < 0.08 ? Math.floor(rnd() * periodCount) : 0;
    const stages: Record<string, "approved" | "exported" | "declared"> = {};
    periods.forEach((p, idx) => {
      if (idx < startIdx) return;
      const age = periodCount - 1 - idx; // 0 = son dönem
      const invoices = 5 + Math.floor(rnd() * 60);
      const f: PeriodFacts = {
        invoices,
        pendingSuggestions: 0,
        openBlockers: 0,
        openWarnings: 0,
        openInfos: rnd() < 0.3 ? 1 + Math.floor(rnd() * 3) : 0,
      };
      if (age === 0) {
        // Son dönem: iş sürüyor; dağılım karışık.
        const s = rnd();
        if (s < 0.08) f.invoices = 0;
        else if (s < 0.45) {
          f.pendingSuggestions = 1 + Math.floor(rnd() * 12);
          if (rnd() < 0.35) f.openBlockers = 1 + Math.floor(rnd() * 3);
        } else if (s < 0.7) {
          // kontrol edildi
        } else if (s < 0.85) stages[p] = "approved";
        else stages[p] = "exported";
        if (rnd() < 0.4) f.openWarnings = 1 + Math.floor(rnd() * 4);
        // Henüz belge gelmemiş dönemde uyarı da yok.
        if (f.invoices === 0) {
          f.openWarnings = 0;
          f.openInfos = 0;
        }
      } else if (age === 1) {
        const s = rnd();
        if (s < 0.1) {
          f.pendingSuggestions = 1 + Math.floor(rnd() * 4);
          f.openBlockers = rnd() < 0.5 ? 1 : 0;
        } else if (s < 0.25) stages[p] = "exported";
        else stages[p] = "declared";
        // Beyandan sonra açılmış engelleyici: yeniden bakılmalı.
        if (stages[p] === "declared" && rnd() < 0.03) f.openBlockers = 1;
      } else {
        stages[p] = "declared";
      }
      facts.set(key(id, p), f);
    });
    if (Object.keys(stages).length) workflow[id] = stages;
  }
  return { taxpayers, members: DEMO_OFFICE_MEMBERS, facts, workflow };
}
