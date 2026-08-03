import { z } from 'zod';

import { rawKey, type MessageFor } from '@/lib/constants/auth';

export function forgotPasswordSchema(t: MessageFor = rawKey) {
  return z.object({
    email: z.email(t('email.invalid')),
  });
}

export type ForgotPasswordValues = z.infer<ReturnType<typeof forgotPasswordSchema>>;
