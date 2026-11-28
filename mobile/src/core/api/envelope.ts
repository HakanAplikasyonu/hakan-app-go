/**
 * Response envelope of the Go API; the parsing is shared with the office app
 * (@hakan/mobile-core, #295). Pure (no React Native imports) so `node --test`
 * can run it.
 */
import { createErrorMessage } from '@hakan/mobile-core/api/envelope';

import type { components } from './schema';

export { ApiError, isApiError, unwrap, type ErrorDetail } from '@hakan/mobile-core/api/envelope';

export type Schemas = components['schemas'];

/** Turkish, user-facing message for an error (codes the screens can hit). */
export const errorMessage = createErrorMessage({
  NO_TAXPAYER_MEMBERSHIP: 'Bu şirkete erişiminiz yok.',
  Z_REPORT_PARTS_OFF: 'Ofisiniz parçalı Z raporunu kapattı; fotoğrafları ayrı ayrı gönderin.',
  UPLOAD_TIMEOUT: 'Bağlantı yavaş olduğu için fotoğraf yarıda kaldı; kendiliğinden yeniden gönderilecek.',
  UPLOAD_TOO_LARGE: 'Fotoğraf çok büyük (en çok 10 MB). Yeniden çekin.',
});
