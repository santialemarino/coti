import { z } from 'zod';

import { HEX_COLOR_DIGITS } from '@/lib/constants/brand';
import { LOWER_PRICE_STRATEGIES } from '@/lib/constants/conversation';
import { URL_FIELD_MAX_LENGTH } from '@/lib/constants/forms';
import {
  optionalTaxId,
  optionalText,
  rawText,
  requiredText,
  type SchemaText,
} from '@/lib/forms/validators';

/*
 * The optional fields hold `''` for "not set", because a text input cannot hold null — the action
 * omits them from the body rather than sending the empty string. Which is why each format check
 * passes `''` explicitly instead of relying on the field being absent.
 */
export function accountSchema(t: SchemaText = rawText) {
  return z.object({
    name: requiredText(t, 'name.required'),
    legalName: optionalText(t),
    taxId: optionalTaxId(t, 'cuit'),
    brandLogoUrl: optionalText(t, URL_FIELD_MAX_LENGTH).refine(
      (raw) => raw === '' || URL.canParse(raw),
      t.field('brandLogoUrl.invalid'),
    ),
    brandColor: z
      .string()
      .trim()
      .refine((raw) => raw === '' || HEX_COLOR_DIGITS.test(raw), t.field('brandColor.invalid')),
    lowerPriceStrategy: z.enum(LOWER_PRICE_STRATEGIES),
  });
}

export type AccountValues = z.infer<ReturnType<typeof accountSchema>>;

/*
 * What the form validates: every field but the logo URL, which has no input to show an error on.
 * The form sends the stored URL as it is, and the action's own check answers for it.
 */
export function accountFormSchema(t: SchemaText = rawText) {
  return accountSchema(t).omit({ brandLogoUrl: true });
}

export type AccountFormValues = z.infer<ReturnType<typeof accountFormSchema>>;
