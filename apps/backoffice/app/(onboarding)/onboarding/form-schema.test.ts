import { describe, expect, it } from 'vitest';

import { onboardingBrandSchema } from '@/app/(onboarding)/onboarding/form-schema';

describe('onboardingBrandSchema', () => {
  it.each(['', 'FFF', 'ffff', 'C2410C', 'C2410C80'])(
    'accepts editable hexadecimal digits %s',
    (brandColor) => {
      expect(onboardingBrandSchema().safeParse({ brandColor }).success).toBe(true);
    },
  );

  it.each(['#C2410C', 'orange', '12', '12345', '1234567'])('rejects %s', (brandColor) => {
    expect(onboardingBrandSchema().safeParse({ brandColor }).success).toBe(false);
  });
});
