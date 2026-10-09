import { z } from 'zod';

import { rawText, requiredText, type SchemaText } from '@/lib/forms/validators';

export function arcaIdentitySchema(t: SchemaText = rawText) {
  return z.object({
    taxId: requiredText(t, 'taxId.required', 13).regex(
      /^\d{2}-?\d{8}-?\d$/,
      t.field('taxId.invalid'),
    ),
  });
}

export function arcaPointOfSaleSchema(t: SchemaText = rawText) {
  return z.object({
    pointOfSale: requiredText(t, 'pointOfSale.required', 5)
      .regex(/^\d+$/, t.field('pointOfSale.invalid'))
      .refine(
        (value) => Number(value) >= 1 && Number(value) <= 99998,
        t.field('pointOfSale.invalid'),
      ),
  });
}

export function arcaCertificateSchema(t: SchemaText = rawText) {
  return z.object({ certificate: requiredText(t, 'certificate.required', 255) });
}

export type ARCACertificateValues = z.infer<ReturnType<typeof arcaCertificateSchema>>;
export type ARCAIdentityValues = z.infer<ReturnType<typeof arcaIdentitySchema>>;
export type ARCAPointOfSaleValues = z.infer<ReturnType<typeof arcaPointOfSaleSchema>>;
