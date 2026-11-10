import { z } from 'zod';

import { MONEY_MAX } from '@/lib/constants/forms';
import { rawText, type SchemaText } from '@/lib/forms/validators';

export function productPriceSchema(t: SchemaText = rawText) {
  const amount = z
    .string()
    .regex(/^\d+(?:\.\d{1,2})?$/, t.field('priceEditor.invalidAmount'))
    .refine((value) => Number(value) <= MONEY_MAX, t.shared('amountTooHigh'));
  return z
    .object({
      branchId: z.string().uuid(t.field('priceEditor.branchRequired')),
      price: z.string().min(1, t.field('priceEditor.priceRequired')).pipe(amount),
      minPrice: z.union([z.literal(''), amount]),
      currency: z.string().regex(/^[A-Z]{3}$/),
    })
    .superRefine((values, ctx) => {
      if (values.minPrice && Number(values.minPrice) > Number(values.price)) {
        ctx.addIssue({
          code: 'custom',
          path: ['minPrice'],
          message: t.field('minPrice.abovePrice'),
        });
      }
    });
}

export type ProductPriceValues = z.infer<ReturnType<typeof productPriceSchema>>;
