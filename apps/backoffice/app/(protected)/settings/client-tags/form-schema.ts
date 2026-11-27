import { z } from 'zod';

import { rawKey, type MessageFor } from '@/lib/forms/validators';

const MAX_INT32 = 2_147_483_647;

export function clientTagRuleSchema(t: MessageFor = rawKey) {
  return z.object({
    kind: z.enum(['RECURRING', 'ABANDONER']),
    enabled: z.boolean(),
    minimum: z
      .number(t('invalidMinimum'))
      .int(t('invalidMinimum'))
      .min(1, t('invalidMinimum'))
      .max(MAX_INT32, t('invalidMinimum')),
    windowDays: z
      .number(t('invalidWindow'))
      .int(t('invalidWindow'))
      .min(0, t('invalidWindow'))
      .max(MAX_INT32, t('invalidWindow')),
  });
}
