import { z } from 'zod';

import {
  currentSecret,
  newPassword,
  passwordConfirmation,
  rawText,
  type SchemaText,
} from '@/lib/forms/validators';

export function changePasswordSchema(t: SchemaText = rawText) {
  return z
    .object({
      currentPassword: currentSecret(t, 'currentPassword.required'),
      newPassword: newPassword(t, 'newPassword.required'),
      confirmPassword: passwordConfirmation(t, 'confirmPassword.required'),
    })
    .refine((values) => !values.confirmPassword || values.newPassword === values.confirmPassword, {
      path: ['confirmPassword'],
      message: t.shared('passwordMismatch'),
    });
}

export type ChangePasswordValues = z.infer<ReturnType<typeof changePasswordSchema>>;
