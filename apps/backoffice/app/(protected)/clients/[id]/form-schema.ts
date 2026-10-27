import { z } from 'zod';

import { IVA_CONDITIONS } from '@/lib/api/invoicing';
import { optionalTaxId, optionalText, rawText, type SchemaText } from '@/lib/forms/validators';

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
