import { useQuery } from '@tanstack/react-query';

import { api } from '@/core/api/client';
import type { Schemas } from '@/core/api/envelope';
import { useAuth } from '@/core/auth/auth-provider';
import type { AcilacakBelge } from '@/features/documents/ac';

export type PayrollDocument = Schemas['PayrollDocument'];
export { groupByPeriod, periodLabel } from './group';
type Page = Schemas['PayrollDocumentPage'];

/**
 * Bordro PDF'leri (`GET /v1/taxpayer/payroll/documents`, #382): ofisin
 * mükellefe açtığı belgeler, salt okuma. Yalnızca şirketin sahibi görür.
 */
export function usePayrollDocuments() {
  const { taxpayer } = useAuth();
  const uuid = taxpayer?.taxpayer.uuid ?? '';
  const owner = taxpayer?.role === 'owner';
  const q = useQuery({
    queryKey: ['payroll-documents', uuid],
    enabled: owner && !!uuid,
    queryFn: async () => (await api.get<Page>('/v1/taxpayer/payroll/documents', { taxpayer: uuid, query: { limit: 200 } })).items,
  });
  return { ...q, owner, items: q.data ?? [] };
}

/** Bordro PDF'i uygulama içi görüntüleyicide açılır (#780). */
export function payrollBelgesi(doc: PayrollDocument): AcilacakBelge {
  return {
    title: doc.title,
    path: `/v1/taxpayer/payroll/documents/${encodeURIComponent(doc.uuid)}/file`,
    fileName: doc.file_name,
    contentType: doc.content_type,
    size: doc.size_bytes,
    id: doc.uuid,
    prefix: 'bordro',
  };
}
