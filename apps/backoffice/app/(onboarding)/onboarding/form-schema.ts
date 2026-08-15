import { z } from 'zod';

import { HEX_COLOR_DIGITS } from '@/lib/constants/brand';
import { rawText, type SchemaText } from '@/lib/forms/validators';

export function onboardingBrandSchema(t: SchemaText = rawText) {
  return z.object({
    brandColor: z
      .string()
      .trim()
      .refine((raw) => raw === '' || HEX_COLOR_DIGITS.test(raw), t.field('brandColor.invalid')),
  });
}

export type OnboardingBrandValues = z.infer<ReturnType<typeof onboardingBrandSchema>>;
