import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { api, request } from '@/core/api/client';
import type { Schemas } from '@/core/api/envelope';
import { useOrganization } from '@/core/auth/auth-provider';

import type { InvoiceDirection, MonthKind } from './format';
import type { Rejection } from './reject-reason';

export type OfficeInvoice = Schemas['OfficeInvoice'];
export type OfficeInvoicePage = Schemas['OfficeInvoicePage'];
export type OfficeInvoiceDetail = Schemas['OfficeInvoiceDetail'];
export type Expense = Schemas['Expense'];
export type ExpensePage = Schemas['EnvelopeExpensePage']['data'];
export type RetailSale = Schemas['RetailSale'];
export type RetailSalePage = Schemas['EnvelopeRetailSalePage']['data'];
export type BankTransaction = Schemas['BankTransaction'];
export type BankTransactionPage = Schemas['BankTransactionPage'];
export type ExpenseCategory = Schemas['ExpenseCategory'];
export type AccountPlanAccount = Schemas['AccountPlanAccount'];
export type ReceiptCapture = Schemas['ReceiptCapture'];
export type ExpensePostingInput = Schemas['ExpensePostingInput'];

const PAGE = 30;

const eng = (engagement: string) => `/v1/tenant/engagements/${encodeURIComponent(engagement)}`;

/** Liste uçlarının yolu (web'deki dört kartla aynı uçlar). */
const listPath: Record<MonthKind, string> = {
  invoices: '/invoices',
  zreports: '/retail-sales',
  expenses: '/expenses',
  bank: '/bank-transactions',
};

const key = (org: string, engagement: string, ...rest: unknown[]) => ['engagement', org, engagement, 'month', ...rest];

function useCount<T>(engagement: string, kind: MonthKind, period: string, enabled: boolean, extra?: Record<string, string>) {
  const org = useOrganization();
  return useQuery({
    queryKey: key(org, engagement, kind, 'summary', period, extra ?? {}),
    queryFn: () => api.get<T>(`${eng(engagement)}${listPath[kind]}`, { organization: org, query: { period, limit: 1, offset: 0, ...extra } }),
    enabled: enabled && !!engagement,
    staleTime: 30_000,
  });
}

/**
 * Dönemin dört kartının sayıları (`limit=1` liste uçları): faturalar (toplam
 * ve gelen), Z raporları ve masraflar (toplam / bekleyen), banka özeti.
 */
export function useMonthCounts(engagement: string, period: string, can: { invoices: boolean; receipts: boolean; bank: boolean }) {
  return {
    invoices: useCount<OfficeInvoicePage>(engagement, 'invoices', period, can.invoices),
    incoming: useCount<OfficeInvoicePage>(engagement, 'invoices', period, can.invoices, { direction: 'incoming' }),
    zreports: useCount<RetailSalePage>(engagement, 'zreports', period, can.receipts),
    expenses: useCount<ExpensePage>(engagement, 'expenses', period, can.receipts),
    bank: useCount<BankTransactionPage>(engagement, 'bank', period, can.bank),
  };
}

export type MonthItem =
  | { kind: 'invoices'; item: OfficeInvoice }
  | { kind: 'zreports'; item: RetailSale }
  | { kind: 'expenses'; item: Expense }
  | { kind: 'bank'; item: BankTransaction };

type AnyPage = { items: unknown[]; total: number };

function pageTotal(kind: MonthKind, page: unknown): number {
  if (kind === 'bank') return (page as BankTransactionPage).summary.total;
  return (page as AnyPage).total;
}

/**
 * Seçili dönemin listesi, sayfa sayfa (`offset`). Faturada `direction`
 * verilirse yalnız gelen ya da giden faturalar (mükellef sekmeleri).
 */
export function useMonthList(engagement: string, kind: MonthKind, period: string, direction?: InvoiceDirection | null) {
  const org = useOrganization();
  const dir = kind === 'invoices' && direction ? direction : null;
  return useInfiniteQuery({
    queryKey: dir ? key(org, engagement, kind, 'list', period, dir) : key(org, engagement, kind, 'list', period),
    initialPageParam: 0,
    queryFn: async ({ pageParam }) => {
      const query: Record<string, string | number> = { period, limit: PAGE, offset: pageParam };
      if (kind === 'bank') query.match_status = 'all';
      if (dir) query.direction = dir;
      const page = await api.get<unknown>(`${eng(engagement)}${listPath[kind]}`, { organization: org, query });
      return { items: (page as AnyPage).items, total: pageTotal(kind, page), offset: pageParam };
    },
    getNextPageParam: (last) => (last.items.length > 0 && last.offset + last.items.length < last.total ? last.offset + last.items.length : undefined),
  });
}

/** Tek kaydın ayrıntısı; banka hareketinin ayrı ucu yok, listeden gelir. */
export function useMonthDetail<T>(engagement: string, kind: Exclude<MonthKind, 'bank'>, uuid: string) {
  const org = useOrganization();
  return useQuery({
    queryKey: key(org, engagement, kind, 'detail', uuid),
    queryFn: () => api.get<T>(`${eng(engagement)}${listPath[kind]}/${encodeURIComponent(uuid)}`, { organization: org }),
    enabled: !!uuid,
  });
}

/**
 * Masrafın / Z raporunun fiş fotoğrafı: görsel fiş yüklemesindedir; yükleme
 * taslağı (`draft.uuid`) bu kayda bağlı olanı aranır. Önce kaydın dönemi,
 * bulunamazsa son yüklemeler.
 */
export function useDraftCapture(engagement: string, draft: string, period: string | null | undefined, enabled: boolean) {
  const org = useOrganization();
  return useQuery({
    queryKey: key(org, engagement, 'capture-of', draft, period ?? ''),
    queryFn: async () => {
      const find = async (query: Record<string, string | number>) => {
        const page = await api.get<{ items: ReceiptCapture[] }>(`${eng(engagement)}/receipts`, { organization: org, query: { limit: 100, ...query } });
        return page.items.find((r) => r.draft?.uuid === draft) ?? null;
      };
      return (period ? await find({ period }) : null) ?? (await find({}));
    },
    enabled: enabled && !!draft,
    staleTime: 5 * 60_000,
  });
}

/** Mükellefin masraf kategorileri (hesap seçimi için). */
export function useExpenseCategories(engagement: string, enabled: boolean) {
  const org = useOrganization();
  return useQuery({
    queryKey: ['engagement', org, engagement, 'expense-categories'],
    queryFn: () => api.get<{ items: ExpenseCategory[] }>(`${eng(engagement)}/expense-categories`, { organization: org }),
    enabled,
    staleTime: 5 * 60_000,
  });
}

/** Mükellefin hesap planında arama (kod öneki ya da ad). */
export function useAccountSearch(engagement: string, q: string | undefined, enabled: boolean) {
  const org = useOrganization();
  return useQuery({
    queryKey: ['engagement', org, engagement, 'account-plan', q ?? ''],
    queryFn: () => api.get<{ items: AccountPlanAccount[]; total: number }>(`${eng(engagement)}/account-plan/accounts`, { organization: org, query: { q, limit: 20 } }),
    enabled: enabled && !!q,
    staleTime: 60_000,
  });
}

function useInvalidateMonth(engagement: string) {
  const org = useOrganization();
  const qc = useQueryClient();
  return () => {
    void qc.invalidateQueries({ queryKey: key(org, engagement) });
    void qc.invalidateQueries({ queryKey: ['dashboard', org] });
  };
}

/** Bekleyen masrafın kategorisi / hesabı (`PATCH .../expenses/{uuid}`). */
export function useExpensePosting(engagement: string, expense: string) {
  const org = useOrganization();
  const invalidate = useInvalidateMonth(engagement);
  return useMutation({
    mutationFn: (input: ExpensePostingInput) =>
      request<Expense>('PATCH', `${eng(engagement)}/expenses/${encodeURIComponent(expense)}`, { organization: org, body: input }),
    onSettled: invalidate,
  });
}

/**
 * Masraf ya da Z raporu kararı (`POST .../approve|reject`): onay yevmiye
 * taslağını kurar; ret gerekçesiyle gider (#780) ve mükellef onu görür.
 */
export function useReceiptDecision(engagement: string, kind: 'expenses' | 'zreports', uuid: string) {
  const org = useOrganization();
  const invalidate = useInvalidateMonth(engagement);
  return useMutation({
    mutationFn: (d: { approve: true } | { approve: false; reason: Rejection }) =>
      api.post<Expense | RetailSale>(
        `${eng(engagement)}${listPath[kind]}/${encodeURIComponent(uuid)}/${d.approve ? 'approve' : 'reject'}`,
        d.approve ? undefined : d.reason,
        { organization: org },
      ),
    onSettled: invalidate,
  });
}
