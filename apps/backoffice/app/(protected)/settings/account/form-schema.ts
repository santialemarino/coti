import { z } from 'zod';

import { rawKey, type MessageFor } from '@/lib/constants/auth';
import { HEX_COLOR } from '@/lib/constants/brand';
import { TEXT_FIELD_MAX_LENGTH, URL_FIELD_MAX_LENGTH } from '@/lib/constants/forms';

// Trimmed before the checks, so a value of nothing but spaces is refused rather than stored as a
// blank no screen can render.
const optional = (t: MessageFor) => z.string().trim().max(TEXT_FIELD_MAX_LENGTH, t('tooLong'));

/*
 * The optional fields hold `''` for "not set", because a text input cannot hold null — the action
 * omits them from the body rather than sending the empty string. Which is why each format check
 * passes `''` explicitly instead of relying on the field being absent.
 */
export function accountSchema(t: MessageFor = rawKey) {
  return z.object({
    name: z.string().trim().min(1, t('name.required')).max(TEXT_FIELD_MAX_LENGTH, t('tooLong')),
    legalName: optional(t),
    taxId: optional(t),
    brandLogoUrl: z
      .string()
      .trim()
      .max(URL_FIELD_MAX_LENGTH, t('brandLogoUrl.tooLong'))
      .refine((raw) => raw === '' || URL.canParse(raw), t('brandLogoUrl.invalid')),
    brandColor: z
      .string()
      .trim()
      .refine((raw) => raw === '' || HEX_COLOR.test(raw), t('brandColor.invalid')),
  });
}

export type AccountValues = z.infer<ReturnType<typeof accountSchema>>;
