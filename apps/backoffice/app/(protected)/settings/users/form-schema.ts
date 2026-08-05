import { z } from 'zod';

import {
  PASSWORD_MAX_LENGTH,
  PASSWORD_MIN_LENGTH,
  rawKey,
  USER_ROLES,
  type MessageFor,
} from '@/lib/constants/auth';
import { TEXT_FIELD_MAX_LENGTH } from '@/lib/constants/forms';

export type UserFormMode = 'create' | 'edit';

const initialPassword = (t: MessageFor) =>
  z
    .string()
    .min(PASSWORD_MIN_LENGTH, t('password.tooShort'))
    .max(PASSWORD_MAX_LENGTH, t('password.tooLong'));

/*
 * The mode decides one field: a password is set once, when the user is created. The API's update
 * body carries none at all — an admin who has to change one mails a recovery link — so in edit
 * mode the field is unvalidated and unread rather than absent, which keeps both modes on one type.
 */
export function userSchema(mode: UserFormMode, t: MessageFor = rawKey) {
  return z.object({
    // Trimmed before the check, so a name of nothing but spaces is refused rather than stored as a
    // blank no screen can render.
    name: z.string().trim().min(1, t('name.required')).max(TEXT_FIELD_MAX_LENGTH, t('tooLong')),
    email: z.email(t('email.invalid')).max(TEXT_FIELD_MAX_LENGTH, t('tooLong')),
    role: z.enum(USER_ROLES, t('role.required')),
    // Every id here comes from the account's own branch list, and the API refuses one that is not
    // an active branch of the account, so a shape check would add nothing.
    branchIds: z.array(z.string()),
    password: mode === 'create' ? initialPassword(t) : z.string(),
  });
}

export type UserValues = z.infer<ReturnType<typeof userSchema>>;
