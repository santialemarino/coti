import { describe, expect, it } from 'vitest';

import {
  changedPointsOfSale,
  pointsOfSaleBody,
  pointsOfSaleValues,
} from '@/lib/forms/points-of-sale';

describe('points of sale at the boundary', () => {
  it('reads an unset number as an empty field', () => {
    expect(
      pointsOfSaleValues([
        { branchId: 'm', name: 'Morón', isActive: true, pointOfSale: 3 },
        { branchId: 'b', name: 'Villa Bosch', isActive: false, pointOfSale: null },
      ]),
    ).toEqual([
      { branchId: 'm', pointOfSale: '3' },
      { branchId: 'b', pointOfSale: '' },
    ]);
  });

  it('sends every branch, a cleared one as null', () => {
    expect(
      pointsOfSaleBody([
        { branchId: 'm', pointOfSale: '03' },
        { branchId: 'b', pointOfSale: '' },
      ]),
    ).toEqual([
      { branch_id: 'm', point_of_sale: 3 },
      { branch_id: 'b', point_of_sale: null },
    ]);
  });
});

describe('changedPointsOfSale', () => {
  it('keeps only the rows whose number moved, a cleared one included', () => {
    const initial = [
      { branchId: 'm', pointOfSale: '1' },
      { branchId: 'b', pointOfSale: '2' },
      { branchId: 'c', pointOfSale: '' },
    ];
    expect(
      changedPointsOfSale(
        [
          { branchId: 'm', pointOfSale: '1' },
          { branchId: 'b', pointOfSale: '' },
          { branchId: 'c', pointOfSale: '4' },
        ],
        initial,
      ),
    ).toEqual([
      { branchId: 'b', pointOfSale: '' },
      { branchId: 'c', pointOfSale: '4' },
    ]);
  });
});
