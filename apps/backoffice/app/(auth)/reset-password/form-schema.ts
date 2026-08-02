import { z } from 'zod';

import { PASSWORD_MIN_LENGTH } from '@/lib/constants/auth';

export const resetPasswordSchema = z
  .object({
    token: z.string().min(1),
    newPassword: z.string().min(PASSWORD_MIN_LENGTH),
    confirmPassword: z.string(),
  })
  .refine((values) => values.newPassword === values.confirmPassword, {
    path: ['confirmPassword'],
  });
