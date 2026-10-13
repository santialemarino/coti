import { describe, expect, it } from 'vitest';

import { arcaStepStatuses } from '@/app/(protected)/settings/invoicing/arca/step-statuses';
import type { InvoicingBranch } from '@/lib/api/invoicing';

const MORON: InvoicingBranch = { branchId: 'm', name: 'Morón', isActive: true, pointOfSale: 1 };
const BOSCH: InvoicingBranch = {
  branchId: 'b',
  name: 'Villa Bosch',
  isActive: true,
  pointOfSale: null,
};
const CLOSED: InvoicingBranch = {
  branchId: 'c',
  name: 'Cerrada',
  isActive: false,
  pointOfSale: null,
};

describe('arcaStepStatuses', () => {
  it('starts on enabling WSASS and generating the request', () => {
    expect(arcaStepStatuses({ csr: '', hasCertificate: false }, [MORON])).toEqual({
      wsass: 'current',
      generate: 'current',
      copy: 'pending',
      certificate: 'pending',
      authorize: 'pending',
      upload: 'pending',
      pointsOfSale: 'pending',
    });
  });

  it('moves to the steps done in ARCA and the upload once the request exists', () => {
    expect(arcaStepStatuses({ csr: 'request', hasCertificate: false }, [MORON])).toEqual({
      wsass: 'done',
      generate: 'done',
      copy: 'current',
      certificate: 'current',
      authorize: 'current',
      upload: 'current',
      pointsOfSale: 'pending',
    });
  });

  it('leaves the points of sale while an open branch has none', () => {
    const statuses = arcaStepStatuses({ csr: 'request', hasCertificate: true }, [MORON, BOSCH]);
    expect(statuses.upload).toBe('done');
    expect(statuses.pointsOfSale).toBe('current');
  });

  it('finishes once every open branch has a point of sale, closed ones aside', () => {
    const statuses = arcaStepStatuses({ csr: 'request', hasCertificate: true }, [MORON, CLOSED]);
    expect(Object.values(statuses).every((status) => status === 'done')).toBe(true);
  });
});
