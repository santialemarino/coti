import { z } from 'zod';

import { currentSecret, emailAddress, rawText, type SchemaText } from '@/lib/forms/validators';

export function loginSchema(t: SchemaText = rawText) {
  return z.object({
    email: emailAddress(t, 'email.required'),
    password: currentSecret(t, 'password.required'),
    rememberMe: z.boolean(),
  });
}

export type LoginValues = z.infer<ReturnType<typeof loginSchema>>;
