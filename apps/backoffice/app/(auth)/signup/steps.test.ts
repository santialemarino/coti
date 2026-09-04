import { describe, expect, it } from 'vitest';

import { signupSchema, type SignupValues } from '@/app/(auth)/signup/form-schema';
import { STEP_ORDER, stepOwning, STEPS } from '@/app/(auth)/signup/steps';

const SCHEMA_FIELDS = Object.keys(signupSchema().shape) as (keyof SignupValues)[];

describe('the wizard steps', () => {
  /*
   * The step lists and the schema are separate declarations, and nothing in the type system ties
   * them together. A field missing from every step is never validated before the caller reaches
   * the end, and a rejection the API attaches to it has no step to be shown on.
   */
  it('gives every field of the schema exactly one step', () => {
    const owners = SCHEMA_FIELDS.map((field) => [field, stepOwning(field)]);

    expect(Object.fromEntries(owners)).toEqual({
      accountName: 'account',
      legalName: 'account',
      taxId: 'account',
      branchName: 'branch',
      branchAddress: 'branch',
      adminName: 'admin',
      adminEmail: 'admin',
      adminPassword: 'admin',
      confirmPassword: 'admin',
    });
  });

  it('names no field the schema does not have', () => {
    const stepped = STEP_ORDER.flatMap((key) => STEPS[key].fields);

    expect(stepped.filter((field) => !SCHEMA_FIELDS.includes(field))).toEqual([]);
  });

  // The links are what the buttons read: a wrong one either strands the caller or skips a step.
  it('links the steps in the order the rail shows them', () => {
    expect(STEP_ORDER.map((key) => [STEPS[key].previous, STEPS[key].next])).toEqual([
      [undefined, 'branch'],
      ['account', 'admin'],
      ['branch', undefined],
    ]);
  });
});
