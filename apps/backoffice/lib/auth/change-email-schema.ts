import { z } from 'zod';

import { currentSecret, emailAddress, rawText, type SchemaText } from '@/lib/forms/validators';

/*
 * Shared rather than colonised by one page: the correction is offered both on the confirmation
 * screen, where an unconfirmed caller lands, and in settings, where a confirmed one goes looking.
 */
export function changeEmailSchema(t: SchemaText = rawText) {
  return z.object({
    newEmail: emailAddress(t, 'newEmail.required'),
    currentPassword: currentSecret(t, 'currentPassword.required'),
  });
}

export type ChangeEmailValues = z.infer<ReturnType<typeof changeEmailSchema>>;
