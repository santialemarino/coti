import { z } from 'zod';

import { passwordIssue } from '@/components/customer-auth-schema';

const RECOVERY_CODE = /^\d{6}$/;

export function recoveryCodeSchema(
  message: (key: string, values?: Record<string, number>) => string,
  minimum: number,
) {
  return z
    .object({ code: z.string(), password: z.string(), confirmation: z.string() })
    .superRefine((values, ctx) => {
      const add = (
        path: 'code' | 'password' | 'confirmation',
        key: string,
        params?: Record<string, number>,
      ) => ctx.addIssue({ code: 'custom', path: [path], message: message(key, params) });
      if (!RECOVERY_CODE.test(values.code.trim())) add('code', 'codeInvalid');
      if (!values.password) add('password', 'passwordRequired');
      else {
        const issue = passwordIssue(values.password, minimum);
        if (issue) add('password', issue.key, issue.params);
        else if (values.confirmation !== values.password) add('confirmation', 'passwordMismatch');
      }
    });
}
