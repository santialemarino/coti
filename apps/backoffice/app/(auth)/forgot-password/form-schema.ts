import { z } from 'zod';

import { emailAddress, rawText, type SchemaText } from '@/lib/forms/validators';

export function forgotPasswordSchema(t: SchemaText = rawText) {
  return z.object({
    email: emailAddress(t, 'email.required'),
  });
}

export type ForgotPasswordValues = z.infer<ReturnType<typeof forgotPasswordSchema>>;
