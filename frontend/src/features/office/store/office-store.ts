"use client";

import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";

import {
  DEMO_PERIOD,
  demoInvoices,
  demoMembers,
  demoSuggestions,
  demoTaxpayers,
  demoWarnings,
} from "../data/demo";
import { NotUblError, isValidTaxId, parseUbl } from "../lib/ubl";
import { REVIEW_THRESHOLD, suggestForLine } from "../lib/suggest";
import { severityOf } from "../lib/warnings";
import { readZip } from "../lib/zip";
import type {
  DataWarning,
  Invoice,
  OfficeMember,
  Suggestion,
  Taxpayer,
  UploadFileResult,
  UploadReport,
  WarningStatus,
} from "../types";

/**
 * Ofis paneli DEMO veri kaynağı. Mükellef (engagement, W1 #2), fatura ve
 * uyarı tabloları (W2 #9) ve öneri motoru (W4) birleşene kadar ekranlar bu
 * depodan okur. Durum tarayıcı oturumunda (sessionStorage) ofis başına tutulur;
 * "Demo verisini sıfırla" ilk hale döndürür. API geldiğinde bu dosyanın
 * yerini `services/office.service.ts` + React Query alır; bileşenler
 * `hooks/use-office.ts` üzerinden okuduğu için değişmez.
 */

export type OfficeData = {
  taxpayers: Taxpayer[];
  members: OfficeMember[];
  invoices: Invoice[];
  warnings: DataWarning[];
  suggestions: Suggestion[];
  uploads: UploadReport[];
};

type OfficeActions = {
  importFiles: (taxpayerId: string, files: File[]) => Promise<UploadReport>;
  decideSuggestion: (
    id: string,
    decision: "approved" | "rejected",
    override?: { accountCode: string; accountName: string },
  ) => void;
  /** Kararı geri alır: öneri yeniden bekler, satırın kodu boşalır. */
  reopenSuggestion: (
    id: string,
    restore?: { accountCode: string; accountName: string },
  ) => void;
  setWarningStatus: (id: string, status: WarningStatus) => void;
  setAssignees: (taxpayerId: string, memberIds: string[]) => void;
  reset: () => void;
};

export type OfficeStore = OfficeData & OfficeActions;

const clone = <T>(v: T): T => structuredClone(v);

function initialData(): OfficeData {
  return {
    taxpayers: clone(demoTaxpayers),
    members: clone(demoMembers),
    invoices: clone(demoInvoices),
    warnings: clone(demoWarnings),
    suggestions: clone(demoSuggestions),
    uploads: [],
  };
}

const newId = () => crypto.randomUUID();

function invoiceStatus(
  invoiceId: string,
  suggestions: Suggestion[],
): Invoice["status"] {
  return suggestions.some(
    (s) => s.invoiceId === invoiceId && s.status === "pending",
  )
    ? "in_review"
    : "approved";
}

type XmlFile = { name: string; text: string };

async function expandFiles(
  files: File[],
): Promise<(XmlFile | UploadFileResult)[]> {
  const out: (XmlFile | UploadFileResult)[] = [];
  for (const file of files) {
    const lower = file.name.toLowerCase();
    if (lower.endsWith(".zip")) {
      try {
        const entries = await readZip(await file.arrayBuffer());
        const decoder = new TextDecoder();
        for (const e of entries) {
          const name = `${file.name} › ${e.name}`;
          if (e.name.toLowerCase().endsWith(".xml"))
            out.push({ name, text: decoder.decode(e.data) });
          else
            out.push({ fileName: name, outcome: "not_ubl", warningCodes: [] });
        }
      } catch {
        out.push({
          fileName: file.name,
          outcome: "error",
          warningCodes: [],
          message: "zip",
        });
      }
    } else if (lower.endsWith(".xml")) {
      out.push({ name: file.name, text: await file.text() });
    } else {
      out.push({ fileName: file.name, outcome: "not_ubl", warningCodes: [] });
    }
  }
  return out;
}

function createOfficeStore(slug: string) {
  return create<OfficeStore>()(
    persist(
      (set, get) => ({
        ...initialData(),

        async importFiles(taxpayerId, files) {
          const taxpayer = get().taxpayers.find((t) => t.id === taxpayerId);
          if (!taxpayer) throw new Error("taxpayer not found");
          const now = new Date().toISOString();
          const results: UploadFileResult[] = [];
          const invoices = [...get().invoices];
          const warnings = [...get().warnings];
          const suggestions = [...get().suggestions];

          for (const item of await expandFiles(files)) {
            if (!("text" in item)) {
              results.push(item);
              continue;
            }
            let parsed;
            try {
              parsed = parseUbl(item.text);
            } catch (err) {
              results.push({
                fileName: item.name,
                outcome: err instanceof NotUblError ? "not_ubl" : "error",
                warningCodes: [],
              });
              continue;
            }
            const { invoice: p, supplier, customer } = parsed;
            const existing = invoices.find(
              (i) =>
                i.taxpayerId === taxpayerId &&
                i.ettn.toLowerCase() === p.ettn.toLowerCase(),
            );
            if (existing) {
              results.push({
                fileName: item.name,
                outcome: "duplicate",
                invoiceId: existing.id,
                ettn: p.ettn,
                number: p.number,
                warningCodes: [],
              });
              continue;
            }

            const codes = [...parsed.warnings];
            let direction: Invoice["direction"] = "purchase";
            let counterparty = supplier;
            if (supplier.taxId === taxpayer.taxId) {
              direction = "sales";
              counterparty = customer;
            } else if (customer.taxId !== taxpayer.taxId) {
              codes.push({
                code: "PARTY_MISMATCH",
                params: {
                  expected: taxpayer.taxId,
                  actual: `${supplier.taxId} / ${customer.taxId}`,
                },
              });
            }
            if (!isValidTaxId(counterparty.taxId))
              codes.push({
                code: "INVALID_TAX_ID",
                params: { value: counterparty.taxId },
              });

            const invoice: Invoice = {
              ...p,
              id: newId(),
              taxpayerId,
              direction,
              counterparty,
              status: "new",
              source: "upload",
              uploadedAt: now,
            };
            const twin = invoices.find(
              (i) =>
                i.taxpayerId === taxpayerId &&
                i.counterparty.taxId === counterparty.taxId &&
                i.issueDate === invoice.issueDate &&
                i.totals.payable === invoice.totals.payable,
            );
            if (twin)
              codes.push({
                code: "POSSIBLE_DUPLICATE",
                params: { count: "1" },
              });

            for (const line of invoice.lines) {
              const s = suggestForLine(invoice, line, newId);
              if (s.confidence >= REVIEW_THRESHOLD) {
                line.accountCode = s.accountCode;
                suggestions.push({ ...s, status: "approved", decidedAt: now });
              } else suggestions.push(s);
            }
            invoice.status = invoiceStatus(invoice.id, suggestions);
            invoices.push(invoice);

            for (const c of codes) {
              warnings.push({
                id: newId(),
                taxpayerId,
                period: invoice.issueDate.slice(0, 7) || DEMO_PERIOD,
                entityType: "invoice",
                entityId: invoice.id,
                code: c.code,
                severity: severityOf(c.code),
                params: c.params,
                producer: "ubl_parser",
                status: "open",
                createdAt: now,
              });
            }
            results.push({
              fileName: item.name,
              outcome: "imported",
              invoiceId: invoice.id,
              ettn: invoice.ettn,
              number: invoice.number,
              warningCodes: codes.map((c) => c.code),
            });
          }

          const report: UploadReport = {
            id: newId(),
            taxpayerId,
            createdAt: now,
            files: results,
          };
          set({
            invoices,
            warnings,
            suggestions,
            uploads: [report, ...get().uploads],
          });
          return report;
        },

        decideSuggestion(id, decision, override) {
          const now = new Date().toISOString();
          const suggestions = get().suggestions.map((s) =>
            s.id === id
              ? { ...s, ...(override ?? {}), status: decision, decidedAt: now }
              : s,
          );
          const target = suggestions.find((s) => s.id === id);
          if (!target) return;
          const invoices = get().invoices.map((inv) => {
            if (inv.id !== target.invoiceId) return inv;
            return {
              ...inv,
              lines: inv.lines.map((l) =>
                l.id === target.lineId
                  ? {
                      ...l,
                      accountCode:
                        decision === "approved" ? target.accountCode : null,
                    }
                  : l,
              ),
              status: invoiceStatus(inv.id, suggestions),
            };
          });
          set({ suggestions, invoices });
        },

        reopenSuggestion(id, restore) {
          const suggestions = get().suggestions.map((s) =>
            s.id === id
              ? {
                  ...s,
                  ...(restore ?? {}),
                  status: "pending" as const,
                  decidedAt: undefined,
                }
              : s,
          );
          const target = suggestions.find((s) => s.id === id);
          if (!target) return;
          const invoices = get().invoices.map((inv) =>
            inv.id !== target.invoiceId
              ? inv
              : {
                  ...inv,
                  lines: inv.lines.map((l) =>
                    l.id === target.lineId ? { ...l, accountCode: null } : l,
                  ),
                  status: invoiceStatus(inv.id, suggestions),
                },
          );
          set({ suggestions, invoices });
        },

        setWarningStatus(id, status) {
          set({
            warnings: get().warnings.map((w) =>
              w.id === id ? { ...w, status } : w,
            ),
          });
        },

        setAssignees(taxpayerId, memberIds) {
          set({
            taxpayers: get().taxpayers.map((t) =>
              t.id === taxpayerId ? { ...t, assigneeIds: memberIds } : t,
            ),
          });
        },

        reset() {
          set(initialData());
        },
      }),
      {
        name: `hakan.office-demo.v1.${slug.toLowerCase()}`,
        storage: createJSONStorage(() => sessionStorage),
        // SSR ile ilk çizim aynı kalsın; OfficeDataGate istemcide yükler.
        skipHydration: true,
        partialize: (s) => ({
          taxpayers: s.taxpayers,
          members: s.members,
          invoices: s.invoices,
          warnings: s.warnings,
          suggestions: s.suggestions,
          uploads: s.uploads,
        }),
      },
    ),
  );
}

type OfficeBoundStore = ReturnType<typeof createOfficeStore>;
const stores = new Map<string, OfficeBoundStore>();

export function officeStore(slug: string): OfficeBoundStore {
  const key = slug.toLowerCase();
  let store = stores.get(key);
  if (!store) {
    store = createOfficeStore(key);
    stores.set(key, store);
  }
  return store;
}
