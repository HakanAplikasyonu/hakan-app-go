import { useQuery } from '@tanstack/react-query';

import { api } from '@/core/api/client';
import type { Schemas } from '@/core/api/envelope';
import { useOrganization } from '@/core/auth/auth-provider';
import type { RemoteFile } from '@/features/files/open';

export type PayrollRun = Schemas['PayrollRun'];
export type PayrollDocument = Schemas['PayrollDocument'];

const base = (engagement: string) => `/v1/tenant/engagements/${encodeURIComponent(engagement)}/payroll`;

/** Bordro dönemleri (`GET .../payroll/runs`, #382): yeni dönem önce. */
export function usePayrollRuns(engagement: string, enabled = true) {
  const org = useOrganization();
  return useQuery({
    queryKey: ['engagement', org, engagement, 'payroll-runs'],
    queryFn: async () => (await api.get<Schemas['PayrollRunPage']>(`${base(engagement)}/runs`, { organization: org, query: { limit: 24 } })).items,
    enabled,
  });
}

/** Bordro PDF'leri (`GET .../payroll/documents`): dönem verilirse yalnızca o dönem. */
export function usePayrollDocuments(engagement: string, period: string | undefined, enabled = true) {
  const org = useOrganization();
  return useQuery({
    queryKey: ['engagement', org, engagement, 'payroll-documents', period ?? ''],
    queryFn: async () =>
      (await api.get<Schemas['PayrollDocumentPage']>(`${base(engagement)}/documents`, { organization: org, query: { period, limit: 200 } })).items,
    enabled,
  });
}

export function payrollFile(engagement: string, d: PayrollDocument): RemoteFile {
  return {
    path: `${base(engagement)}/documents/${encodeURIComponent(d.uuid)}/file`,
    uuid: d.uuid,
    prefix: 'bordro',
    title: d.title,
    fileName: d.file_name,
    contentType: d.content_type,
  };
}
