import { useQuery } from '@tanstack/react-query';
import { File, Paths } from 'expo-file-system';
import { Linking, Platform, Share } from 'react-native';

import { api } from '@/core/api/client';
import type { Schemas } from '@/core/api/envelope';
import { useAuth } from '@/core/auth/auth-provider';
import { session } from '@/core/auth/session';
import { env } from '@/core/config/env';

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

/**
 * PDF'i önbelleğe indirir ve paylaşım / görüntüleme sayfasını açar. Liste
 * isteği belirteci tazelediği için burada geçerli bir erişim belirteci var.
 */
export async function openDocument(taxpayer: string, doc: PayrollDocument): Promise<void> {
  const token = session.get()?.accessToken;
  const dest = new File(Paths.cache, `bordro-${doc.uuid}.pdf`);
  if (dest.exists) dest.delete();
  const file = await File.downloadFileAsync(
    `${env.apiUrl.replace(/\/$/, '')}/v1/taxpayer/payroll/documents/${encodeURIComponent(doc.uuid)}/file`,
    dest,
    { headers: { 'X-Taxpayer': taxpayer, ...(token ? { Authorization: `Bearer ${token}` } : {}) } },
  );
  if (Platform.OS === 'ios') {
    await Share.share({ url: file.uri, title: doc.title });
    return;
  }
  await Linking.openURL(file.contentUri ?? file.uri);
}
