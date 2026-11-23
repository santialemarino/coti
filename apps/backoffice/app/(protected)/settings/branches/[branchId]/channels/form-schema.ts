import { isPossiblePhoneNumber, isValidPhoneNumber } from 'libphonenumber-js/min';
import { z } from 'zod';

import { rawText, requiredText, type SchemaText } from '@/lib/forms/validators';

export const CHANNEL_SECRET_MAX_LENGTH = 4096;
const META_TEST_NUMBER_PREFIX = '+1555';

export function whatsappChannelSchema(
  t: SchemaText = rawText,
  configured = false,
  originalIdentifier?: string | null,
) {
  const setting = (schema: z.ZodString) =>
    configured
      ? z
          .string()
          .trim()
          .pipe(z.union([z.literal(''), schema]))
      : schema;
  const secret = (requiredKey: string) =>
    setting(requiredText(t, requiredKey, CHANNEL_SECRET_MAX_LENGTH)).refine(
      (value) => new TextEncoder().encode(value).byteLength <= CHANNEL_SECRET_MAX_LENGTH,
      t.field('secretTooLong'),
    );

  return z
    .object({
      identifier: requiredText(t, 'number.required').refine(
        // Meta test numbers use reserved NANP codes that real-number validation rejects.
        (value) =>
          isValidPhoneNumber(value) ||
          (value.startsWith(META_TEST_NUMBER_PREFIX) && isPossiblePhoneNumber(value)),
        t.field('number.invalid'),
      ),
      phoneNumberId: setting(
        requiredText(t, 'phoneNumberId.required').regex(/^\d+$/, t.field('metaId.invalid')),
      ),
      businessAccountId: setting(
        requiredText(t, 'businessAccountId.required').regex(/^\d+$/, t.field('metaId.invalid')),
      ),
      accessToken: secret('accessToken.required'),
      webhookVerifyToken: secret('webhookVerifyToken.required'),
      isActive: z.boolean(),
    })
    .superRefine((values, ctx) => {
      if (!configured || originalIdentifier == null || values.identifier === originalIdentifier)
        return;
      for (const field of ['phoneNumberId', 'businessAccountId', 'accessToken'] as const) {
        if (!values[field]) {
          ctx.addIssue({ code: 'custom', path: [field], message: t.field('numberChangeRequired') });
        }
      }
    });
}

export type WhatsAppChannelValues = z.infer<ReturnType<typeof whatsappChannelSchema>>;
