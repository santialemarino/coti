import { z } from 'zod';

import { MONEY_MAX } from '@/lib/constants/forms';
import { optionalText, rawText, requiredText, type SchemaText } from '@/lib/forms/validators';

export function productSchema(t: SchemaText = rawText) {
  return z
    .object({
      code: optionalText(t),
      name: requiredText(t, 'name.required'),
      description: optionalText(t, 512),
      unit: requiredText(t, 'unit.required', 64),
      familyId: z.string().min(1, t.field('family.required')),
      subgroupId: z.string(),
      isActive: z.boolean(),
      // Canonical decimal strings from AmountInput; both optional, and only sent on create.
      price: z.string(),
      minPrice: z.string(),
    })
    .superRefine((values, ctx) => {
      const amountTooHigh = t.shared('amountTooHigh');
      if (values.price && Number(values.price) > MONEY_MAX) {
        ctx.addIssue({ code: 'custom', path: ['price'], message: amountTooHigh });
      }
      if (!values.minPrice) return;
      if (Number(values.minPrice) > MONEY_MAX) {
        ctx.addIssue({ code: 'custom', path: ['minPrice'], message: amountTooHigh });
      } else if (!values.price) {
        ctx.addIssue({
          code: 'custom',
          path: ['price'],
          message: t.field('price.requiredWithMin'),
        });
      } else if (Number(values.minPrice) > Number(values.price)) {
        ctx.addIssue({
          code: 'custom',
          path: ['minPrice'],
          message: t.field('minPrice.abovePrice'),
        });
      }
    });
}

export type ProductValues = z.infer<ReturnType<typeof productSchema>>;
