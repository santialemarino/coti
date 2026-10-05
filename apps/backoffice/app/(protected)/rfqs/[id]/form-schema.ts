import { z } from 'zod';

import { IVA_CONDITIONS } from '@/lib/api/invoicing';
import { optionalText, rawText, type SchemaText } from '@/lib/forms/validators';

export const TAX_ID_MAX_LENGTH = 32;

// A CUIT is eleven digits and a DNI seven or eight; separators are the API's to strip.
function isTaxIdShape(raw: string): boolean {
  const digits = raw.replace(/\D/g, '');
  return digits.length === 11 || digits.length === 7 || digits.length === 8;
}

// The client's fiscal identity, edited where an invoice needs it. Every field may stay empty.
export function clientFiscalSchema(t: SchemaText = rawText) {
  return z.object({
    legalName: optionalText(t),
    taxId: optionalText(t, TAX_ID_MAX_LENGTH).refine(
      (raw) => raw === '' || isTaxIdShape(raw),
      t.field('taxId.invalid'),
    ),
    ivaCondition: z.union([z.enum(IVA_CONDITIONS), z.literal('')]),
  });
}

export type ClientFiscalValues = z.infer<ReturnType<typeof clientFiscalSchema>>;
