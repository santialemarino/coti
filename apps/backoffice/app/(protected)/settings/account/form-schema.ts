import { z } from 'zod';

import { HEX_COLOR } from '@/lib/constants/brand';
import { URL_FIELD_MAX_LENGTH } from '@/lib/constants/forms';
import { optionalText, rawText, requiredText, type SchemaText } from '@/lib/forms/validators';

/*
 * The optional fields hold `''` for "not set", because a text input cannot hold null — the action
 * omits them from the body rather than sending the empty string. Which is why each format check
 * passes `''` explicitly instead of relying on the field being absent.
 */
export function accountSchema(t: SchemaText = rawText) {
  return z.object({
    name: requiredText(t, 'name.required'),
    legalName: optionalText(t),
    taxId: optionalText(t),
    brandLogoUrl: optionalText(t, URL_FIELD_MAX_LENGTH).refine(
      (raw) => raw === '' || URL.canParse(raw),
      t.field('brandLogoUrl.invalid'),
    ),
    brandColor: z
      .string()
      .trim()
      .refine((raw) => raw === '' || HEX_COLOR.test(raw), t.field('brandColor.invalid')),
  });
}

export type AccountValues = z.infer<ReturnType<typeof accountSchema>>;
