import { z } from 'zod';

import { POINT_OF_SALE_MAX, POINT_OF_SALE_MIN, type InvoicingBranch } from '@/lib/api/invoicing';
import type { SchemaText } from '@/lib/forms/validators';

export const POINT_OF_SALE_MAX_LENGTH = String(POINT_OF_SALE_MAX).length;

// One branch's row: a QuantityInput's digit string, `''` for "none yet".
export interface PointOfSaleValue {
  branchId: string;
  pointOfSale: string;
}

/*
 * Every branch's point of sale, with the range ARCA accepts. Two branches sharing one would hand
 * ARCA two invoice sequences under one number, so the second is refused on its own row. Messages
 * resolve in the invoicing settings namespace wherever the table is edited.
 */
export function pointsOfSaleSchema(t: SchemaText) {
  return z
    .array(
      z.object({
        branchId: z.string().min(1),
        pointOfSale: z
          .string()
          .refine(
            (raw) =>
              raw === '' ||
              (/^\d+$/.test(raw) &&
                Number(raw) >= POINT_OF_SALE_MIN &&
                Number(raw) <= POINT_OF_SALE_MAX),
            t.field('pointOfSale.range', { min: POINT_OF_SALE_MIN, max: POINT_OF_SALE_MAX }),
          ),
      }),
    )
    .superRefine((branches, ctx) => {
      const seen = new Set<number>();
      branches.forEach((branch, index) => {
        if (branch.pointOfSale === '') return;
        const value = Number(branch.pointOfSale);
        if (seen.has(value)) {
          ctx.addIssue({
            code: 'custom',
            path: [index, 'pointOfSale'],
            message: t.field('pointOfSale.duplicate'),
          });
        }
        seen.add(value);
      });
    });
}

export function pointsOfSaleValues(branches: readonly InvoicingBranch[]): PointOfSaleValue[] {
  return branches.map((branch) => ({
    branchId: branch.branchId,
    pointOfSale: branch.pointOfSale === null ? '' : String(branch.pointOfSale),
  }));
}

// The wire shape: every branch is sent, so a cleared number arrives as null.
export function pointsOfSaleBody(values: readonly PointOfSaleValue[]) {
  return values.map((branch) => ({
    branch_id: branch.branchId,
    point_of_sale: branch.pointOfSale === '' ? null : Number(branch.pointOfSale),
  }));
}
