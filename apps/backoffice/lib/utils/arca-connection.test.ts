import { describe, expect, it } from 'vitest';

import type { InvoicingBranch } from '@/lib/api/invoicing';
import { arcaConnectionState, everyActiveBranchHasPointOfSale } from '@/lib/utils/arca-connection';

const NOW = '2026-10-13T12:00:00.000Z';
const RENEWAL_DAYS = 30;
const READY: InvoicingBranch[] = [{ branchId: 'm', name: 'Morón', isActive: true, pointOfSale: 1 }];
const MISSING: InvoicingBranch[] = [
  ...READY,
  { branchId: 'b', name: 'Villa Bosch', isActive: true, pointOfSale: null },
];
const NONE = { enabled: true, csr: '', hasCertificate: false, certificateExpiresAt: null };
const INSTALLED = {
  ...NONE,
  csr: 'request',
  hasCertificate: true,
  certificateExpiresAt: '2028-10-13T00:00:00Z',
};

describe('arcaConnectionState', () => {
  it.each([
    ['disabled', { ...NONE, enabled: false }, READY],
    ['notConnected', NONE, READY],
    ['inProgress', { ...NONE, csr: 'request' }, READY],
    ['expired', { ...INSTALLED, certificateExpiresAt: '2026-10-13T11:00:00Z' }, MISSING],
    ['pointsMissing', INSTALLED, MISSING],
    ['pointsMissing', { ...INSTALLED, certificateExpiresAt: '2026-10-20T00:00:00Z' }, MISSING],
    ['expiringSoon', { ...INSTALLED, certificateExpiresAt: '2026-11-12T00:00:00Z' }, READY],
    ['connected', { ...INSTALLED, certificateExpiresAt: '2026-11-13T00:00:00Z' }, READY],
    ['connected', INSTALLED, READY],
  ] as const)('reads %s', (state, setup, branches) => {
    expect(arcaConnectionState(setup, branches, NOW, RENEWAL_DAYS)).toBe(state);
  });
});

describe('everyActiveBranchHasPointOfSale', () => {
  it('ignores closed branches but needs at least one open one', () => {
    const closed: InvoicingBranch = {
      branchId: 'c',
      name: 'Cerrada',
      isActive: false,
      pointOfSale: null,
    };
    expect(everyActiveBranchHasPointOfSale([...READY, closed])).toBe(true);
    expect(everyActiveBranchHasPointOfSale([closed])).toBe(false);
    expect(everyActiveBranchHasPointOfSale(MISSING)).toBe(false);
  });
});
