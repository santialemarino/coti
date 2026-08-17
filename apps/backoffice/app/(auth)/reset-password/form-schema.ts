import { z } from 'zod';

import {
  newPassword,
  passwordConfirmation,
  rawText,
  type SchemaText,
} from '@/lib/forms/validators';

export function resetPasswordSchema(t: SchemaText = rawText) {
  return z
    .object({
      token: z.string().min(1),
      newPassword: newPassword(t, 'newPassword.required'),
      confirmPassword: passwordConfirmation(t, 'confirmPassword.required'),
    })
    .refine((values) => !values.confirmPassword || values.newPassword === values.confirmPassword, {
      path: ['confirmPassword'],
      message: t.shared('passwordMismatch'),
    });
}

export type ResetPasswordValues = z.infer<ReturnType<typeof resetPasswordSchema>>;
