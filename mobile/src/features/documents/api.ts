/**
 * Ofisin mükellefe paylaştığı belgeler (#682, #780 B): önemli belgeler
 * (vergi levhası, imza sirküleri…) ve "Mükellef de görsün" işaretli arşiv
 * belgeleri. İkisi de yalnızca şirket sahibine açık; dosya
 * `/v1/taxpayer/archive-documents/{uuid}/file` ile iner.
 */
import { useQuery } from '@tanstack/react-query';

import { api } from '@/core/api/client';
import type { Schemas } from '@/core/api/envelope';
import { useAuth } from '@/core/auth/auth-provider';

import type { AcilacakBelge } from './ac';

export type ArchiveDocument = Schemas['ArchiveDocument'];
type Important = Schemas['TaxpayerImportantDocuments'];
type Page = Schemas['ArchiveDocumentPage'];

export function useImportantDocuments() {
  const { taxpayer } = useAuth();
  const uuid = taxpayer?.taxpayer.uuid ?? '';
  const owner = taxpayer?.role === 'owner';
  return useQuery({
    queryKey: ['important-documents', uuid],
    enabled: owner && !!uuid,
    queryFn: () => api.get<Important>('/v1/taxpayer/important-documents', { taxpayer: uuid }),
  });
}

export function useSharedDocuments() {
  const { taxpayer } = useAuth();
  const uuid = taxpayer?.taxpayer.uuid ?? '';
  const owner = taxpayer?.role === 'owner';
  const q = useQuery({
    queryKey: ['archive-documents', uuid],
    enabled: owner && !!uuid,
    queryFn: async () => (await api.get<Page>('/v1/taxpayer/archive-documents', { taxpayer: uuid, query: { limit: 200 } })).items,
  });
  return { ...q, owner, items: q.data ?? [] };
}

/** Arşiv belgesi görüntüleyicide açılır. */
export function arsivBelgesi(d: ArchiveDocument): AcilacakBelge {
  return {
    title: d.title || d.file_name,
    path: `/v1/taxpayer/archive-documents/${encodeURIComponent(d.uuid)}/file`,
    fileName: d.file_name,
    contentType: d.content_type,
    size: d.size_bytes,
    id: d.uuid,
    prefix: 'arsiv',
  };
}

/** Fatura PDF'i (kayıtlı faturadan çizilir; #780). */
export function faturaBelgesi(f: { uuid: string; invoice_number: string }): AcilacakBelge {
  return {
    title: `Fatura ${f.invoice_number}`,
    path: `/v1/taxpayer/invoices/${encodeURIComponent(f.uuid)}/pdf`,
    contentType: 'application/pdf',
    id: f.uuid,
    prefix: 'fatura',
  };
}

/** Sunucudaki fişin fotoğrafı (#780); yalnızca görsel yüklemeler için. */
export function fisBelgesi(h: { uuid: string; media_type: string; vendor_name?: string }): AcilacakBelge | null {
  if (!h.media_type.toLowerCase().startsWith('image/')) return null;
  return {
    title: h.vendor_name || 'Fiş',
    path: `/v1/taxpayer/receipts/${encodeURIComponent(h.uuid)}/image`,
    contentType: h.media_type,
    id: h.uuid,
    prefix: 'fis',
  };
}

/** Mükellefin gönderdiği yabancı hizmet faturasının dosyası (#780). */
export function yabanciFaturaBelgesi(f: { uuid: string; vendor?: string | null }): AcilacakBelge {
  return {
    title: f.vendor || 'Yabancı fatura',
    path: `/v1/taxpayer/foreign-invoices/${encodeURIComponent(f.uuid)}/file`,
    id: f.uuid,
    prefix: 'yabanci',
  };
}
