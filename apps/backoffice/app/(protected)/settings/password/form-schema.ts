import { z } from 'zod';

import { PASSWORD_MIN_LENGTH, rawKey, type MessageFor } from '@/lib/constants/auth';

export function changePasswordSchema(t: MessageFor = rawKey) {
  return z
    .object({
      currentPassword: z.string().min(1, t('currentPassword.required')),
      newPassword: z.string().min(PASSWORD_MIN_LENGTH, t('newPassword.tooShort')),
      confirmPassword: z.string(),
    })
    .refine((values) => values.newPassword === values.confirmPassword, {
      path: ['confirmPassword'],
      message: t('confirmPassword.mismatch'),
    });
}

export type ChangePasswordValues = z.infer<ReturnType<typeof changePasswordSchema>>;
