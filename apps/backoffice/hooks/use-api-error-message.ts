'use client';

import { useCallback } from 'react';
import { useTranslations } from 'next-intl';

import type { ApiErrorCode } from '@/lib/api/errors';
import { apiErrorMessage } from '@/lib/i18n/api-error';

/*
 * The client half of the error catalog: bind a flow's namespace once and every refusal it
 * receives resolves through it. A server component reads the same catalog with
 * `apiErrorMessage(await getTranslations(), …)`.
 */
export function useApiErrorMessage(namespace?: string): (code?: ApiErrorCode) => string {
  const t = useTranslations();

  return useCallback(
    // Missing means the failure never carried a code, which is the unexpected case itself.
    (code?: ApiErrorCode) => apiErrorMessage(t, namespace, code),
    [t, namespace],
  );
}
