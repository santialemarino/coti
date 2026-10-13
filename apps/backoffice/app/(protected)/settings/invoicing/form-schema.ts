import { z } from 'zod';

import { POINT_OF_SALE_MAX, POINT_OF_SALE_MIN } from '@/lib/api/invoicing';
import { TEXT_FIELD_MAX_LENGTH } from '@/lib/constants/forms';
import { rawText, requiredText, type SchemaText } from '@/lib/forms/validators';

// The caps the inputs stop typing at, shared with the rules below so the two cannot drift.
export const ADDRESS_MAX_LENGTH = TEXT_FIELD_MAX_LENGTH;
export const GROSS_INCOME_NUMBER_MAX_LENGTH = 20;
export const POINT_OF_SALE_MAX_LENGTH = String(POINT_OF_SALE_MAX).length;

// Mirrors the API: province registrations differ in shape, so only bounds every one fits apply.
const GROSS_INCOME_DIGITS_MIN = 5;
const GROSS_INCOME_DIGITS_MAX = 15;
const GROSS_INCOME_NUMBER = /^\d+(-\d+)*$/;

// What the API stores for an issuer exempt from gross income tax.
export const GROSS_INCOME_EXEMPT = 'EXENTO';

export const GROSS_INCOME_KINDS = ['number', 'exempt'] as const;
export type GrossIncomeKind = (typeof GROSS_INCOME_KINDS)[number];

// The issuer conditions a corralón can hold; a final consumer never issues invoices.
export const ISSUER_IVA_CONDITIONS = ['REGISTERED', 'MONOTRIBUTO', 'EXEMPT'] as const;

// A registration number as it is stored: spaces and dots dropped, digit groups and hyphens kept.
export function normalizeGrossIncomeNumber(raw: string): string {
  return raw.replace(/[\s.]/g, '');
}

function isGrossIncomeNumber(value: string): boolean {
  const digits = value.replace(/-/g, '').length;
  return (
    GROSS_INCOME_NUMBER.test(value) &&
    digits >= GROSS_INCOME_DIGITS_MIN &&
    digits <= GROSS_INCOME_DIGITS_MAX
  );
}

/*
 * A point of sale is a QuantityInput's digit string, `''` for "none yet". Two branches sharing one
 * would hand ARCA two invoice sequences under one number, so the second is refused on its own row.
 */
export function invoicingSettingsSchema(t: SchemaText = rawText) {
  return z
    .object({
      ivaCondition: z.union([z.enum(ISSUER_IVA_CONDITIONS), z.literal('')]),
      pricesIncludeVat: z.boolean(),
      address: requiredText(t, 'profile.addressRequired', ADDRESS_MAX_LENGTH),
      grossIncomeKind: z.enum(GROSS_INCOME_KINDS),
      grossIncomeNumber: z.string().transform(normalizeGrossIncomeNumber),
      activityStartedOn: requiredText(t, 'profile.activityStartedOnRequired', 10).pipe(
        z.string().regex(/^\d{4}-\d{2}-\d{2}$/, t.field('profile.activityStartedOnInvalid')),
      ),
      branches: z
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
        }),
    })
    .superRefine((values, ctx) => {
      if (values.ivaCondition === '') {
        ctx.addIssue({
          code: 'custom',
          path: ['ivaCondition'],
          message: t.field('ivaCondition.required'),
        });
      }
      if (values.grossIncomeKind === 'exempt') return;
      if (values.grossIncomeNumber === '') {
        ctx.addIssue({
          code: 'custom',
          path: ['grossIncomeNumber'],
          message: t.field('profile.grossIncomeNumberRequired'),
        });
      } else if (!isGrossIncomeNumber(values.grossIncomeNumber)) {
        ctx.addIssue({
          code: 'custom',
          path: ['grossIncomeNumber'],
          message: t.field('profile.grossIncomeNumberInvalid', {
            min: GROSS_INCOME_DIGITS_MIN,
            max: GROSS_INCOME_DIGITS_MAX,
          }),
        });
      }
    });
}

export type InvoicingSettingsValues = z.input<ReturnType<typeof invoicingSettingsSchema>>;
export type InvoicingSettingsOutput = z.output<ReturnType<typeof invoicingSettingsSchema>>;

// What the API stores for the registration the form describes.
export function grossIncomeRegistrationOf(
  values: Pick<InvoicingSettingsOutput, 'grossIncomeKind' | 'grossIncomeNumber'>,
): string {
  return values.grossIncomeKind === 'exempt' ? GROSS_INCOME_EXEMPT : values.grossIncomeNumber;
}
