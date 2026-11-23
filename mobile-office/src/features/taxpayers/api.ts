import { useMutation, useQueryClient } from '@tanstack/react-query';
import { Platform } from 'react-native';

import { api } from '@/core/api/client';
import { ApiError, unwrap, type Schemas } from '@/core/api/envelope';
import { useOrganization } from '@/core/auth/auth-provider';
import { session } from '@/core/auth/session';
import { env } from '@/core/config/env';
import { isComingSoon } from '@/features/doctasks/channels';

import { requestBody, type CertificateReading, type NewTaxpayerForm } from './certificate';

export type PickedImage = { uri: string; name: string; mimeType: string };

/**
 * Vergi levhası okuma ucu (yapay zekâ). K1 ekibinden bekleniyor:
 * `POST /v1/tenant/tax-certificates/read`, çok parçalı `file` alanı;
 * yanıt `{ tax_id, title, tax_office, address, activity_code }` (okunamayan
 * alan null). Uç geldiğinde yalnızca bu dosya değişir.
 */
const READ_PATH = '/v1/tenant/tax-certificates/read';

async function post(org: string, file: PickedImage, token: string | null) {
  const form = new FormData();
  if (Platform.OS === 'web') {
    form.append('file', await (await fetch(file.uri)).blob(), file.name);
  } else {
    // React Native FormData dosya tanımı nesnesi alır.
    form.append('file', { uri: file.uri, name: file.name, type: file.mimeType } as unknown as Blob);
  }
  const headers: Record<string, string> = { Accept: 'application/json', 'X-Organization': org };
  if (token) headers.Authorization = `Bearer ${token}`;
  try {
    const res = await fetch(`${env.apiUrl}${READ_PATH}`, { method: 'POST', headers, body: form });
    const payload = await res.json().catch(() => undefined);
    return { status: res.status, payload, requestId: res.headers.get('X-Request-ID') ?? undefined };
  } catch {
    throw new ApiError({ status: 0, code: 'NETWORK_ERROR', message: 'Network request failed' });
  }
}

/** Levhayı okutur; uç henüz yoksa `'soon'` döner ("Bu özellik yakında"). */
export async function readTaxCertificate(org: string, file: PickedImage): Promise<CertificateReading | 'soon'> {
  let res = await post(org, file, session.get()?.accessToken ?? null);
  if (res.status === 401) {
    // Oturumu bir kez tazeleyip yeniden dene.
    await api.get('/v1/auth/me').catch(() => {});
    res = await post(org, file, session.get()?.accessToken ?? null);
  }
  if (isComingSoon(res)) return 'soon';
  return unwrap<CertificateReading>(res.status, res.payload, res.requestId);
}

export function useReadCertificate() {
  const org = useOrganization();
  return useMutation({ mutationFn: (file: PickedImage) => readTaxCertificate(org, file) });
}

export type TaxpayerRequestAccepted = Schemas['TaxpayerRequestAccepted'];

/** Mükellef ekle (`POST /v1/tenant/taxpayer-requests`): mükellefe onay e-postası gider. */
export function useCreateTaxpayerRequest() {
  const org = useOrganization();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (f: NewTaxpayerForm) => api.post<TaxpayerRequestAccepted>('/v1/tenant/taxpayer-requests', requestBody(f), { organization: org }),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ['clients', org] }),
  });
}
