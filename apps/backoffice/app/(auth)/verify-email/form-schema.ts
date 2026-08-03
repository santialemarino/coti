import { z } from 'zod';

import { rawKey, type MessageFor } from '@/lib/constants/auth';

export function resendVerificationSchema(t: MessageFor = rawKey) {
  return z.object({
    email: z.email(t('email.invalid')),
  });
}

export type ResendVerificationValues = z.infer<ReturnType<typeof resendVerificationSchema>>;
