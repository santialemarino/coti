import { z } from 'zod';

import {
  PASSWORD_MAX_LENGTH,
  PASSWORD_MIN_LENGTH,
  rawKey,
  type MessageFor,
} from '@/lib/constants/auth';
import { TEXT_FIELD_MAX_LENGTH } from '@/lib/constants/forms';

// Trimmed before the checks run, so a name of nothing but spaces fails `min(1)` rather than
// reaching the API as a blank the database happily stores.
const text = (t: MessageFor) => z.string().trim().max(TEXT_FIELD_MAX_LENGTH, t('tooLong'));

export function signupSchema(t: MessageFor = rawKey) {
  return z
    .object({
      accountName: text(t).min(1, t('accountName.required')),
      legalName: text(t),
      taxId: text(t),
      branchName: text(t).min(1, t('branchName.required')),
      branchAddress: text(t),
      adminName: text(t).min(1, t('adminName.required')),
      adminEmail: z.email(t('adminEmail.invalid')).max(TEXT_FIELD_MAX_LENGTH, t('tooLong')),
      adminPassword: z
        .string()
        .min(PASSWORD_MIN_LENGTH, t('adminPassword.tooShort'))
        .max(PASSWORD_MAX_LENGTH, t('adminPassword.tooLong')),
      confirmPassword: z.string(),
    })
    .refine((values) => values.adminPassword === values.confirmPassword, {
      path: ['confirmPassword'],
      message: t('confirmPassword.mismatch'),
    });
}

export type SignupValues = z.infer<ReturnType<typeof signupSchema>>;
