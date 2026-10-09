import { describe, expect, it } from 'vitest';

import { GLOBAL_ERROR_COPY } from '@/lib/i18n/global-error-copy';
import messages from '@/translations/es.json';

describe('GLOBAL_ERROR_COPY', () => {
  // The catalog stays the source of the wording; this copy only exists to keep it off every page.
  it('matches the catalog it was copied from', () => {
    expect(GLOBAL_ERROR_COPY).toEqual({
      appName: messages.common.appName,
      title: messages.errors.INTERNAL,
      retry: messages.common.retry,
    });
  });
});
