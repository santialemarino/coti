import { z } from 'zod';

import {
  emailAddress,
  newPassword,
  optionalText,
  passwordConfirmation,
  rawText,
  requiredText,
  type SchemaText,
} from '@/lib/forms/validators';

/* Unrefined, so a step can pick the fields it owns — a refinement spans the whole object. */
export function signupObject(t: SchemaText = rawText) {
  return z.object({
    accountName: requiredText(t, 'accountName.required'),
    legalName: optionalText(t),
    taxId: optionalText(t),
    branchName: requiredText(t, 'branchName.required'),
    branchAddress: optionalText(t),
    adminName: requiredText(t, 'adminName.required'),
    adminEmail: emailAddress(t, 'adminEmail.required'),
    adminPassword: newPassword(t, 'adminPassword.required'),
    confirmPassword: passwordConfirmation(t, 'confirmPassword.required'),
  });
}

export function signupSchema(t: SchemaText = rawText) {
  return signupObject(t).refine(
    (values) => !values.confirmPassword || values.adminPassword === values.confirmPassword,
    {
      path: ['confirmPassword'],
      message: t.shared('passwordMismatch'),
    },
  );
}

export type SignupValues = z.infer<ReturnType<typeof signupObject>>;
