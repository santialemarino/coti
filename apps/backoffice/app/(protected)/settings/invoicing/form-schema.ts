import { z } from 'zod';

import { IVA_CONDITIONS, POINT_OF_SALE_MAX, POINT_OF_SALE_MIN } from '@/lib/api/invoicing';
import { rawText, type SchemaText } from '@/lib/forms/validators';

/*
 * A point of sale is a QuantityInput's digit string, `''` for "none yet". Two branches sharing one
 * would hand ARCA two invoice sequences under one number, so the second is refused on its own row.
 */
export function invoicingSettingsSchema(t: SchemaText = rawText) {
  return z.object({
    ivaCondition: z.union([z.enum(IVA_CONDITIONS), z.literal('')]),
    pricesIncludeVat: z.boolean(),
    branches: z
      .array(
        z.object({
          branchId: z.string().min(1),
          pointOfSale: z
            .string()
            .refine(
              (raw) =>
                raw === '' ||
                (/^\d+$/.test(raw) &&
                  Number(raw) >= POINT_OF_SALE_MIN &&
                  Number(raw) <= POINT_OF_SALE_MAX),
              t.field('pointOfSale.range', { min: POINT_OF_SALE_MIN, max: POINT_OF_SALE_MAX }),
            ),
        }),
      )
      .superRefine((branches, ctx) => {
        const seen = new Set<number>();
        branches.forEach((branch, index) => {
          if (branch.pointOfSale === '') return;
          const value = Number(branch.pointOfSale);
          if (seen.has(value)) {
            ctx.addIssue({
              code: 'custom',
              path: [index, 'pointOfSale'],
              message: t.field('pointOfSale.duplicate'),
            });
          }
          seen.add(value);
        });
      }),
  });
}

export type InvoicingSettingsValues = z.infer<ReturnType<typeof invoicingSettingsSchema>>;
