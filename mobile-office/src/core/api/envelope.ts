/**
 * Response envelope of the Go API; the parsing is shared with the mükellef
 * app (@hakan/mobile-core, #295). Pure (no React Native imports) so
 * `node --test` can run it.
 */
import { createErrorMessage } from '@hakan/mobile-core/api/envelope';

import type { components } from './schema';

export { ApiError, isApiError, unwrap, type ErrorDetail } from '@hakan/mobile-core/api/envelope';

export type Schemas = components['schemas'];

/** Turkish, user-facing message for an error (codes the screens can hit). */
export const errorMessage = createErrorMessage({
  NO_TENANT_MEMBERSHIP: 'Bu ofise erişiminiz yok.',
  ORGANIZATION_SUSPENDED: 'Bu ofisin hesabı askıya alınmış.',
  ORGANIZATION_ACCESS_EXPIRED: 'Bu ofisin erişim süresi dolmuş.',
  PERMISSION_DENIED: 'Bu işlem için yetkiniz yok.',
  FORBIDDEN: 'Bu işlem için yetkiniz yok.',
});
