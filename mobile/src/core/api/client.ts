// İstemci ortak pakette (@hakan/mobile-core, #295); mükellef kapsamı X-Taxpayer.
import { createApiClient, type RequestOptions as CoreRequestOptions } from '@hakan/mobile-core/api/client';

import { session } from '../auth/session';
import { env } from '../config/env';

export type { Query } from '@hakan/mobile-core/api/client';

/** `taxpayer`: mükellef scope of `/v1/taxpayer/*` requests (`X-Taxpayer`). */
export type RequestOptions = CoreRequestOptions<'taxpayer'>;

export const { request, api } = createApiClient({
  baseUrl: () => env.apiUrl,
  timeoutMs: env.requestTimeoutMs,
  session,
  scope: { option: 'taxpayer', header: 'X-Taxpayer' },
});
