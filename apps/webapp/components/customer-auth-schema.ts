import { isValidPhoneNumber } from 'libphonenumber-js/min';
import { z } from 'zod';

import { isValidCuit } from '@repo/ui/lib';

export type CustomerAuthMode = 'welcome' | 'login' | 'signup' | 'forgot' | 'google' | 'linkGoogle';

/* The catalog key of what a chosen password lacks, or null; the API enforces the same policy. */
export function passwordIssue(
  password: string,
  minimum: number,
): { key: string; params?: Record<string, number> } | null {
  if ([...password].length < minimum) return { key: 'passwordLength', params: { min: minimum } };
  if (new TextEncoder().encode(password).length > 72) return { key: 'passwordMaximum' };
  if (
    !/\p{Lu}/u.test(password) ||
    !/\p{Ll}/u.test(password) ||
    !/\p{Nd}/u.test(password) ||
    !/[^\p{L}\p{Nd}]/u.test(password)
  )
    return { key: 'passwordPolicy' };
  return null;
}

export function customerAuthSchema(
  mode: CustomerAuthMode,
  message: (key: string, values?: Record<string, number>) => string,
  minimum: number,
) {
  return z
    .object({
      taxId: z.string(),
      password: z.string(),
      confirmation: z.string(),
      email: z.string(),
      name: z.string().optional(),
      phone: z.string().optional(),
    })
    .superRefine((values, ctx) => {
      const add = (path: keyof typeof values, key: string, params?: Record<string, number>) =>
        ctx.addIssue({ code: 'custom', path: [path], message: message(key, params) });
      if (['signup', 'google'].includes(mode) && !values.name?.trim()) add('name', 'nameRequired');
      if (mode === 'signup' && !values.email.trim() && !values.phone?.trim())
        add('email', 'contactRequired');
      if (mode === 'signup' && values.phone && !isValidPhoneNumber(values.phone))
        add('phone', 'phoneInvalid');
      if (
        ['login', 'signup', 'google', 'linkGoogle'].includes(mode) &&
        (!isValidCuit(values.taxId) || values.taxId === '00000000000')
      )
        add('taxId', 'taxInvalid');
      if (['login', 'signup'].includes(mode) && !values.password)
        add('password', 'passwordRequired');
      if (mode === 'signup' && values.password) {
        const issue = passwordIssue(values.password, minimum);
        if (issue) add('password', issue.key, issue.params);
        if (values.confirmation !== values.password) add('confirmation', 'passwordMismatch');
      }
      if (
        mode === 'signup' &&
        values.email.trim() !== '' &&
        !z.email().max(255).safeParse(values.email.trim()).success
      )
        add('email', 'emailInvalid');
    });
}
