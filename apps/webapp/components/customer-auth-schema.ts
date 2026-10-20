import { z } from 'zod';

import { isValidCuit } from '@repo/ui/lib';

export type CustomerAuthMode =
  | 'welcome'
  | 'login'
  | 'signup'
  | 'forgot'
  | 'reset'
  | 'verify'
  | 'google'
  | 'linkGoogle';

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
    })
    .superRefine((values, ctx) => {
      const add = (path: keyof typeof values, key: string, params?: Record<string, number>) =>
        ctx.addIssue({ code: 'custom', path: [path], message: message(key, params) });
      if (
        ['login', 'signup', 'forgot', 'google', 'linkGoogle'].includes(mode) &&
        (!isValidCuit(values.taxId) || values.taxId === '00000000000')
      )
        add('taxId', 'taxInvalid');
      if (['login', 'signup', 'reset'].includes(mode) && !values.password)
        add('password', 'passwordRequired');
      if (['signup', 'reset'].includes(mode) && values.password) {
        if ([...values.password].length < minimum)
          add('password', 'passwordLength', { min: minimum });
        else if (new TextEncoder().encode(values.password).length > 72)
          add('password', 'passwordMaximum');
        else if (
          !/\p{Lu}/u.test(values.password) ||
          !/\p{Ll}/u.test(values.password) ||
          !/\p{Nd}/u.test(values.password) ||
          !/[^\p{L}\p{Nd}]/u.test(values.password)
        )
          add('password', 'passwordPolicy');
        if (values.confirmation !== values.password) add('confirmation', 'passwordMismatch');
      }
      if (
        (mode === 'forgot' || (mode === 'signup' && values.email.trim() !== '')) &&
        !z.email().max(255).safeParse(values.email.trim()).success
      )
        add('email', 'emailInvalid');
    });
}
