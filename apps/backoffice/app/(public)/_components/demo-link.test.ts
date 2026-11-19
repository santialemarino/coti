import { describe, expect, it } from 'vitest';

import { demoContactHref } from '@/app/(public)/_components/demo-link';

const COPY = { message: 'Hola, quiero una demo', subject: 'Demo de Coti' };

describe('demoContactHref', () => {
  it('opens a WhatsApp chat with the message when a number is configured', () => {
    expect(demoContactHref(COPY, '+54 9 11 5550-0199', 'ventas@coti.test')).toBe(
      'https://wa.me/5491155500199?text=Hola%2C%20quiero%20una%20demo',
    );
  });

  it('falls back to an email with the subject', () => {
    expect(demoContactHref(COPY, undefined, 'ventas@coti.test')).toBe(
      'mailto:ventas@coti.test?subject=Demo%20de%20Coti',
    );
  });

  it('offers nothing while no contact is configured', () => {
    expect(demoContactHref(COPY, undefined, undefined)).toBeNull();
  });
});
