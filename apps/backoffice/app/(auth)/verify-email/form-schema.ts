import { z } from 'zod';

import { emailAddress, rawText, type SchemaText } from '@/lib/forms/validators';

export function resendVerificationSchema(t: SchemaText = rawText) {
  return z.object({
    email: emailAddress(t, 'email.required'),
  });
}

export type ResendVerificationValues = z.infer<ReturnType<typeof resendVerificationSchema>>;
