import { expect, it } from 'vitest';

import { mapCustomerProfile } from '@/lib/api/customer-profile-model';

it('maps the complete commercial profile boundary independently of credentials', () => {
  expect(
    mapCustomerProfile({
      client_id: 'client',
      tax_id: '20123456786',
      name: 'Customer',
      ownership_unproved: true,
      contacts: [{ kind: 'EMAIL', value: 'one@example.test', primary: true }],
    }),
  ).toEqual({
    clientId: 'client',
    legalName: null,
    address: null,
    ivaCondition: null,
    taxId: '20123456786',
    name: 'Customer',
    ownershipUnproved: true,
    contacts: [{ kind: 'EMAIL', value: 'one@example.test', primary: true }],
  });
});
