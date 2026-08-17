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

/*
 * The mode decides one field: a password is set once, when the user is created. The API's update
 * body carries none at all — an admin who has to change one mails a recovery link — so in edit
 * mode the field is unvalidated and unread rather than absent, which keeps both modes on one type.
 */
export function userSchema(mode: UserFormMode, t: SchemaText = rawText) {
  return z.object({
    name: requiredText(t, 'name.required'),
    email: emailAddress(t, 'email.required'),
    role: z.enum(USER_ROLES, t.field('role.required')),
    // Every id here comes from the account's own branch list, and the API refuses one that is not
    // an active branch of the account, so a shape check would add nothing.
    branchIds: z.array(z.string()),
    password: mode === 'create' ? newPassword(t, 'password.required') : z.string(),
  });
}

export type UserValues = z.infer<ReturnType<typeof userSchema>>;
