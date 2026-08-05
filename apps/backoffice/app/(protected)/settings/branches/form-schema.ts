import { z } from 'zod';

import { rawKey, type MessageFor } from '@/lib/constants/auth';
import { EXPIRY_MAX_DAYS, EXPIRY_MIN_DAYS } from '@/lib/constants/branch';
import { TEXT_FIELD_MAX_LENGTH } from '@/lib/constants/forms';

/*
 * The expiry stays a string all the way to the action, which is the one place it becomes a number.
 * Kept as typed rather than coerced in the schema so the form never holds a `NaN` it would have to
 * render, and the range is checked on the value the caller actually sees.
 */
const expiryDays = (t: MessageFor) =>
  z
    .string()
    .trim()
    .refine((raw) => {
      const days = Number(raw);
      return /^\d+$/.test(raw) && days >= EXPIRY_MIN_DAYS && days <= EXPIRY_MAX_DAYS;
    }, t('defaultExpiryDays.outOfRange'));

export function branchSchema(t: MessageFor = rawKey) {
  return z.object({
    // Trimmed before the check, so a name of nothing but spaces is refused rather than stored as a
    // blank no screen can render.
    name: z.string().trim().min(1, t('name.required')).max(TEXT_FIELD_MAX_LENGTH, t('tooLong')),
    address: z.string().trim().max(TEXT_FIELD_MAX_LENGTH, t('tooLong')),
    defaultExpiryDays: expiryDays(t),
  });
}

export type BranchValues = z.infer<ReturnType<typeof branchSchema>>;
