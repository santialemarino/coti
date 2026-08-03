import { z } from 'zod';

import { PASSWORD_MIN_LENGTH, rawKey, type MessageFor } from '@/lib/constants/auth';

export function resetPasswordSchema(t: MessageFor = rawKey) {
  return z
    .object({
      token: z.string().min(1),
      newPassword: z.string().min(PASSWORD_MIN_LENGTH, t('newPassword.tooShort')),
      confirmPassword: z.string(),
    })
    .refine((values) => values.newPassword === values.confirmPassword, {
      path: ['confirmPassword'],
      message: t('confirmPassword.mismatch'),
    });
}

export type ResetPasswordValues = z.infer<ReturnType<typeof resetPasswordSchema>>;
