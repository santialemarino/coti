import { describe, expect, it } from 'vitest';

import { contactSchema, EMPTY_PUBLIC_RFQ, orderSchema } from '@/app/rfq/[channelId]/form-schema';

const contact = {
  taxId: '20123456786',
  name: 'Obra Norte',
  email: 'contacto@example.com',
  phone: '',
};

describe('public RFQ validation', () => {
  it('requires customer declarations every time and permits an absent phone', () => {
    expect(contactSchema().safeParse(contact).success).toBe(true);
    expect(contactSchema().safeParse(EMPTY_PUBLIC_RFQ).success).toBe(false);
    expect(contactSchema().safeParse({ ...contact, taxId: '20123456780' }).success).toBe(false);
  });

  it('validates only the selected input mode and enforces the API limits', () => {
    const schema = orderSchema(20, 10, ['text/plain']);
    expect(
      schema.safeParse({ ...contact, mode: 'text', text: '10 bolsas', file: undefined }).success,
    ).toBe(true);
    expect(schema.safeParse({ ...contact, mode: 'text', text: ' ', file: undefined }).success).toBe(
      false,
    );
    expect(
      schema.safeParse({ ...contact, mode: 'text', text: 'a'.repeat(21), file: undefined }).success,
    ).toBe(false);
    expect(
      schema.safeParse({
        ...contact,
        mode: 'file',
        text: '',
        file: new File(['cemento'], 'pedido.txt', { type: 'text/plain' }),
      }).success,
    ).toBe(true);
    expect(
      schema.safeParse({
        ...contact,
        mode: 'file',
        text: '',
        file: new File(['a'.repeat(11)], 'pedido.txt', { type: 'text/plain' }),
      }).success,
    ).toBe(false);
  });
});
