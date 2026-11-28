import { z } from 'zod';

import { IVA_CONDITIONS } from '@/lib/api/invoicing';
import {
  optionalTaxId,
  optionalText,
  rawText,
  requiredText,
  type SchemaText,
} from '@/lib/forms/validators';

// The seller corrects only a provisional profile's name; a registered one's is the customer's.
export function clientNameSchema(t: SchemaText = rawText) {
  return z.object({ name: requiredText(t, 'required') });
}

export type ClientNameValues = z.infer<ReturnType<typeof clientNameSchema>>;

export const CLIENT_NOTES_MAX_LENGTH = 512;

export function clientNotesSchema(t: SchemaText = rawText) {
  return z.object({
    notes: z
      .string()
      .refine(
        (notes) => Array.from(notes).length <= CLIENT_NOTES_MAX_LENGTH,
        t.shared('tooLong', { max: CLIENT_NOTES_MAX_LENGTH }),
      )
      .refine(
        (notes) =>
          !Array.from(notes).some((character) => character !== '\n' && /\p{Cc}/u.test(character)),
        t.field('invalidCharacters'),
      )
      .transform((notes) => notes.trim()),
  });
}

export type ClientNotesValues = z.infer<ReturnType<typeof clientNotesSchema>>;

// The client's fiscal identity is editable independently of an order; its CUIT is not part of it.
export function clientFiscalSchema(t: SchemaText = rawText) {
  return z.object({
    address: optionalText(t).optional(),
    legalName: optionalText(t),
    dni: optionalTaxId(t, 'dni', 'dni'),
    ivaCondition: z.union([z.enum(IVA_CONDITIONS), z.literal('')]),
  });
}

export type ClientFiscalValues = z.infer<ReturnType<typeof clientFiscalSchema>>;
