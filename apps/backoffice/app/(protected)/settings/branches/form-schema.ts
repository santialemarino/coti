import { z } from 'zod';

import { EXPIRY_MAX_DAYS, EXPIRY_MIN_DAYS } from '@/lib/constants/branch';
import { optionalText, rawText, requiredText, type SchemaText } from '@/lib/forms/validators';

/*
 * The expiry stays a string all the way to the action, which is the one place it becomes a number.
 * Kept as typed rather than coerced in the schema so the form never holds a `NaN` it would have to
 * render, and the range is checked on the value the caller actually sees.
 */
const expiryDays = (t: SchemaText) =>
  z
    .string()
    .trim()
    .min(1, t.field('defaultExpiryDays.required'))
    .pipe(
      z.string().refine(
        (raw) => {
          const days = Number(raw);
          return /^\d+$/.test(raw) && days >= EXPIRY_MIN_DAYS && days <= EXPIRY_MAX_DAYS;
        },
        t.field('defaultExpiryDays.outOfRange', { min: EXPIRY_MIN_DAYS, max: EXPIRY_MAX_DAYS }),
      ),
    );

export function branchSchema(t: SchemaText = rawText) {
  return z.object({
    name: requiredText(t, 'name.required'),
    address: optionalText(t),
    defaultExpiryDays: expiryDays(t),
  });
}

export type BranchValues = z.infer<ReturnType<typeof branchSchema>>;
