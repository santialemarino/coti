import { z } from 'zod';

import { PASSWORD_MIN_LENGTH, rawKey, type MessageFor } from '@/lib/constants/auth';

export function loginSchema(t: MessageFor = rawKey) {
  return z.object({
    email: z.email(t('email.invalid')),
    password: z.string().min(PASSWORD_MIN_LENGTH, t('password.tooShort')),
    rememberMe: z.boolean(),
  });
}

export type LoginValues = z.infer<ReturnType<typeof loginSchema>>;
