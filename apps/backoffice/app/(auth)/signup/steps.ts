import { signupObject, signupSchema, type SignupValues } from '@/app/(auth)/signup/form-schema';
import { rawText, type SchemaText } from '@/lib/forms/validators';

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

/*
 * What the form validates while the caller is on a given step. Advancing is a submit of that step,
 * so the schema covers its fields and nothing else — validating the whole object would mark fields
 * nobody has reached. The last step validates everything, because a field two steps back can still
 * have been emptied on the way through.
 */
export function stepSchema(step: StepKey, t: SchemaText = rawText) {
  if (step === 'account')
    return signupObject(t).pick({ accountName: true, legalName: true, taxId: true });
  if (step === 'branch') return signupObject(t).pick({ branchName: true, branchAddress: true });
  return signupSchema(t);
}

// Which step shows a given field, so a rejection the API attaches to one can be put on screen.
export function stepOwning(field: keyof SignupValues): StepKey | undefined {
  return STEP_ORDER.find((key) => STEPS[key].fields.includes(field));
}
