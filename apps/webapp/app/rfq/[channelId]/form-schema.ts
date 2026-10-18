import { isValidPhoneNumber } from 'libphonenumber-js/min';
import { z } from 'zod';

import { isValidCuit } from '@repo/ui/lib';
import {
  emailAddress,
  optionalText,
  rawText,
  requiredText,
  type SchemaText,
} from '@/lib/forms/validators';

export function contactSchema(t: SchemaText = rawText) {
  return z.object({
    taxId: requiredText(t, 'taxId.required', 32).refine(
      (value) => isValidCuit(value) && value !== '00000000000',
      t.field('taxId.invalid'),
    ),
    name: requiredText(t, 'name.required'),
    email: emailAddress(t, 'email.required'),
    phone: optionalText(t, 64).refine(
      (value) => value === '' || isValidPhoneNumber(value),
      t.field('phone.invalid'),
    ),
  });
}

export function contactStepSchema(t: SchemaText = rawText) {
  return contactSchema(t).extend({
    mode: z.enum(['text', 'file']),
    text: z.string(),
    file: z.custom<File | undefined>(),
  });
}

export function orderSchema(
  maxTextCharacters: number,
  maxFileSize: number,
  acceptedTypes: string[],
  t: SchemaText = rawText,
) {
  return contactStepSchema(t).superRefine((values, ctx) => {
    if (values.mode === 'text') {
      const parsed = requiredText(t, 'text.required', maxTextCharacters).safeParse(values.text);
      if (!parsed.success)
        for (const issue of parsed.error.issues)
          ctx.addIssue({ code: 'custom', path: ['text'], message: issue.message });
    } else if (!values.file) {
      ctx.addIssue({ code: 'custom', path: ['file'], message: t.field('file.required') });
    } else if (values.file.size === 0 || values.file.size > maxFileSize) {
      ctx.addIssue({ code: 'custom', path: ['file'], message: t.field('file.size') });
    } else if (!acceptedTypes.includes(values.file.type)) {
      ctx.addIssue({ code: 'custom', path: ['file'], message: t.field('file.type') });
    }
  });
}

export type PublicRfqValues = z.infer<ReturnType<typeof orderSchema>>;
export const EMPTY_PUBLIC_RFQ: PublicRfqValues = {
  taxId: '',
  name: '',
  email: '',
  phone: '',
  mode: 'text',
  text: '',
  file: undefined,
};
