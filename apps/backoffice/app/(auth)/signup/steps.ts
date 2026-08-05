import { type SignupValues } from '@/app/(auth)/signup/form-schema';

export type StepKey = 'account' | 'branch' | 'admin';

interface SignupStep {
  /* What advancing gates on, and what decides where a rejection from the API is shown. */
  fields: readonly (keyof SignupValues)[];
  /* Absent at the ends of the wizard, which is how its buttons know what they do. */
  previous?: StepKey;
  next?: StepKey;
}

export const STEPS: Record<StepKey, SignupStep> = {
  account: {
    fields: ['accountName', 'legalName', 'taxId'],
    next: 'branch',
  },
  branch: {
    fields: ['branchName', 'branchAddress'],
    previous: 'account',
    next: 'admin',
  },
  admin: {
    fields: ['adminName', 'adminEmail', 'adminPassword', 'confirmPassword'],
    previous: 'branch',
  },
};

export const STEP_ORDER: readonly StepKey[] = ['account', 'branch', 'admin'];

// Which step shows a given field, so a rejection the API attaches to one can be put on screen.
export function stepOwning(field: keyof SignupValues): StepKey | undefined {
  return STEP_ORDER.find((key) => STEPS[key].fields.includes(field));
}
