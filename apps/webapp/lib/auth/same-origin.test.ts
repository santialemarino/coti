import { NextRequest } from 'next/server';
import { expect, it } from 'vitest';

import { isSameOrigin } from '@/lib/auth/same-origin';

it('accepts only mutations from the current supplier origin', () => {
  expect(
    isSameOrigin(
      new NextRequest('https://supplier.test/api/action', {
        headers: { origin: 'https://supplier.test' },
      }),
    ),
  ).toBe(true);
  for (const origin of [
    '',
    'null',
    'https://other-supplier.test',
    'https://supplier.test.attacker.test',
  ])
    expect(
      isSameOrigin(
        new NextRequest('https://supplier.test/api/action', { headers: origin ? { origin } : {} }),
      ),
    ).toBe(false);
  expect(
    isSameOrigin(
      new NextRequest('https://supplier.test/api/action', {
        headers: { origin: 'https://supplier.test', 'sec-fetch-site': 'cross-site' },
      }),
    ),
  ).toBe(false);
});
