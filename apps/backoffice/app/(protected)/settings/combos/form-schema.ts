import { z } from 'zod';

import { optionalText, rawText, requiredText, type SchemaText } from '@/lib/forms/validators';

// The API's own caps.
export const COMBO_NAME_MAX_LENGTH = 255;
export const COMBO_DESCRIPTION_MAX_LENGTH = 512;
export const COMBO_MIN_ITEMS = 2;

const isPositive = (raw: string) => raw !== '' && Number(raw) > 0;

export function comboSchema(t: SchemaText = rawText) {
  const item = z.object({
    productId: z.string(),
    // The product's name, for display only; the API takes the id.
    label: z.string(),
    quantity: z.string(),
  });
  return z
    .object({
      name: requiredText(t, 'name.required', COMBO_NAME_MAX_LENGTH),
      description: optionalText(t, COMBO_DESCRIPTION_MAX_LENGTH),
      isActive: z.boolean(),
      items: z.array(item),
      branchIds: z.array(z.string()),
    })
    .superRefine((values, ctx) => {
      const issue = (path: (string | number)[], key: string, params?: Record<string, number>) =>
        ctx.addIssue({ code: 'custom', path, message: t.field(key, params) });
      if (values.items.length < COMBO_MIN_ITEMS) {
        issue(['items'], 'items.tooFew', { count: COMBO_MIN_ITEMS });
      }
      const seen = new Set<string>();
      values.items.forEach((entry, index) => {
        if (!entry.productId) issue(['items', index, 'productId'], 'items.productRequired');
        else if (seen.has(entry.productId)) issue(['items', index, 'productId'], 'items.repeated');
        seen.add(entry.productId);
        if (!isPositive(entry.quantity))
          issue(['items', index, 'quantity'], 'items.quantityRequired');
      });
      if (values.branchIds.length === 0) issue(['branchIds'], 'branches.required');
    });
}

export type ComboValues = z.infer<ReturnType<typeof comboSchema>>;
