import { z } from 'zod';

import { optionalText, rawText, requiredText, type SchemaText } from '@/lib/forms/validators';

export function productSchema(t: SchemaText = rawText) {
  return z.object({
    code: optionalText(t),
    name: requiredText(t, 'name.required'),
    description: optionalText(t, 512),
    unit: requiredText(t, 'unit.required', 64),
    familyId: z.string().min(1, t.field('family.required')),
    subgroupId: z.string(),
    isActive: z.boolean(),
  });
}

export type ProductValues = z.infer<ReturnType<typeof productSchema>>;
