/**
 * Mükellef KDV durumu ve yabancı hizmet faturaları (#780, webdeki mükellef
 * panelinin mobil karşılığı).
 *
 * - `GET /v1/taxpayer/vat-sales`: bu ayın anlık KDV tahmini ve KDV1 son günü.
 *   Ofis paneli kapattıysa 403 `TAXPAYER_VAT_PANEL_DISABLED`, ofis yoksa 404;
 *   kart gizlenir.
 * - `GET /v1/taxpayer/foreign-invoices`: bu ay gönderilen yabancı faturalar,
 *   salt okunur. `enabled: false` ise ofis bölümü kapatmıştır; kart gizlenir.
 * - `POST /v1/taxpayer/foreign-invoices` (multipart `file`, `note`): fotoğraf
 *   ya da PDF ofise gider. Aynı dosya ikinci kez gelirse `replayed`.
 */
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Platform } from 'react-native';

import { api } from '@/core/api/client';
import { ApiError, unwrap, type Schemas } from '@/core/api/envelope';
import { useAuth } from '@/core/auth/auth-provider';
import { session } from '@/core/auth/session';
import { env } from '@/core/config/env';

import { kdvGizli } from './ozet';

export type VatSales = Schemas['TaxpayerVATSales'];
export type ForeignInvoiceList = Schemas['TaxpayerForeignInvoiceList'];
export type ForeignInvoice = Schemas['TaxpayerForeignInvoice'];
type UploadResult = Schemas['EnvelopeTaxpayerForeignInvoiceUpload']['data'];

export const FOREIGN_PATH = '/v1/taxpayer/foreign-invoices';

/** Bu ayın KDV durumu; ofis kapattıysa `hidden`. */
export function useVatSales() {
  const { taxpayer } = useAuth();
  const uuid = taxpayer?.taxpayer.uuid ?? '';
  const q = useQuery({
    queryKey: ['taxpayer-vat-sales', uuid],
    enabled: !!uuid,
    queryFn: () => api.get<VatSales>('/v1/taxpayer/vat-sales', { taxpayer: uuid }),
    retry: (n, err) => !kdvGizli(err) && n < 2,
  });
  return { ...q, hidden: q.isError && kdvGizli(q.error) };
}

/** Bu ay gönderilen yabancı faturalar (dönem boş: sunucu içinde bulunulan ayı alır). */
export function useForeignInvoices() {
  const { taxpayer } = useAuth();
  const uuid = taxpayer?.taxpayer.uuid ?? '';
  const q = useQuery({
    queryKey: ['taxpayer-foreign-invoices', uuid],
    enabled: !!uuid,
    queryFn: () => api.get<ForeignInvoiceList>(FOREIGN_PATH, { taxpayer: uuid }),
    retry: false,
  });
  // Ofis kapattı ya da etkin ofis yok: gönderim gösterilmez.
  const hidden = q.isError || (!!q.data && !q.data.enabled);
  return { ...q, hidden, items: q.data?.items ?? [], count: q.data?.count ?? 0 };
}

export type PickedFile = { uri: string; name: string; mimeType: string };

async function post(taxpayer: string, file: PickedFile, note: string, token: string | null) {
  const form = new FormData();
  if (Platform.OS === 'web') {
    form.append('file', await (await fetch(file.uri)).blob(), file.name);
  } else {
    // React Native FormData dosya tanımı nesnesi alır.
    form.append('file', { uri: file.uri, name: file.name, type: file.mimeType } as unknown as Blob);
  }
  if (note.trim()) form.append('note', note.trim());
  const headers: Record<string, string> = { Accept: 'application/json', 'X-Taxpayer': taxpayer };
  if (token) headers.Authorization = `Bearer ${token}`;
  try {
    const res = await fetch(`${env.apiUrl}${FOREIGN_PATH}`, { method: 'POST', headers, body: form });
    const payload = await res.json().catch(() => undefined);
    return { status: res.status, payload, requestId: res.headers.get('X-Request-ID') ?? undefined };
  } catch {
    throw new ApiError({ status: 0, code: 'NETWORK_ERROR', message: 'Network request failed' });
  }
}

/** Yabancı faturayı gönderir; 401'de oturumu bir kez tazeleyip yeniden dener. */
export async function uploadForeignInvoice(taxpayer: string, file: PickedFile, note: string): Promise<UploadResult> {
  let res = await post(taxpayer, file, note, session.get()?.accessToken ?? null);
  if (res.status === 401) {
    await api.get('/v1/auth/me').catch(() => {});
    res = await post(taxpayer, file, note, session.get()?.accessToken ?? null);
  }
  return unwrap<UploadResult>(res.status, res.payload, res.requestId);
}

export function useUploadForeignInvoice() {
  const { taxpayer } = useAuth();
  const uuid = taxpayer?.taxpayer.uuid ?? '';
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ file, note }: { file: PickedFile; note: string }) => uploadForeignInvoice(uuid, file, note),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ['taxpayer-foreign-invoices', uuid] }),
  });
}
