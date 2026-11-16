// İstemci ortak pakette (@hakan/mobile-core, #295); ofis kapsamı X-Organization.
import { createApiClient, type RequestOptions as CoreRequestOptions } from '@hakan/mobile-core/api/client';

import { session } from '../auth/session';
import { env } from '../config/env';

export type { Query } from '@hakan/mobile-core/api/client';

/** `organization`: ofis kapsamı, `/v1/tenant/*` isteklerinin `X-Organization` başlığı (slug ya da uuid). */
export type RequestOptions = CoreRequestOptions<'organization'>;

export const { request, api } = createApiClient({
  baseUrl: () => env.apiUrl,
  timeoutMs: env.requestTimeoutMs,
  session,
  scope: { option: 'organization', header: 'X-Organization' },
});
