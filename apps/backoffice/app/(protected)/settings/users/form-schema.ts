import { z } from 'zod';

import { USER_ROLES } from '@/lib/constants/auth';
import {
  emailAddress,
  newPassword,
  rawText,
  requiredText,
  type SchemaText,
} from '@/lib/forms/validators';

export type UserFormMode = 'create' | 'edit';

/* How a new user gets in: a mailed link to choose their own password, or one the admin sets. */
export const USER_ACCESS = ['INVITE', 'PASSWORD'] as const;

export type UserAccess = (typeof USER_ACCESS)[number];

/*
 * The mode and the access decide one field: a password is set once, by the admin, when the user is
 * created without an invite. Otherwise the field is unvalidated and unread rather than absent,
 * which keeps every case on one type. The API's update body carries no password at all.
 */
export function userSchema(mode: UserFormMode, t: SchemaText = rawText) {
  const password = newPassword(t, 'password.required');

  return z
    .object({
      name: requiredText(t, 'name.required'),
      email: emailAddress(t, 'email.required'),
      role: z.enum(USER_ROLES, t.field('role.required')),
      // Every id here comes from the account's own branch list, and the API refuses one that is
      // not an active branch of the account, so a shape check would add nothing.
      branchIds: z.array(z.string()),
      access: z.enum(USER_ACCESS),
      password: z.string(),
    })
    .superRefine((values, ctx) => {
      if (mode !== 'create' || values.access !== 'PASSWORD') return;
      password.safeParse(values.password).error?.issues.forEach((issue) => {
        ctx.addIssue({ code: 'custom', path: ['password'], message: issue.message });
      });
    });
}

export type UserValues = z.infer<ReturnType<typeof userSchema>>;
