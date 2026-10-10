import { ROUTES } from '@/config/routes';

/*
 * A piece of configuration whose absence changes what the product does. Each one is reported
 * inline, where the missing piece bites, and never papered over with a fallback the reader
 * cannot see.
 */
export const SETUP_ISSUES = [
  'BRANCH_EMAIL',
  'BRANCH_NO_SELLERS',
  'NO_INTAKE_CHANNELS',
  'INVOICING_DISABLED',
  'ACCOUNT_IVA_CONDITION',
  'ISSUER_PROFILE_REQUIRED',
  'ACCOUNT_TAX_ID',
  'BRANCH_POINT_OF_SALE',
  'ARCA_CREDENTIALS',
  'ARCA_CREDENTIALS_EXPIRED',
  'ARCA_CREDENTIALS_CUIT',
] as const;

export type SetupIssue = (typeof SETUP_ISSUES)[number];

// Where an administrator fixes each one. An issue with no screen of its own has no entry.
export const SETUP_FIX_ROUTE: Partial<Record<SetupIssue, string>> = {
  BRANCH_EMAIL: ROUTES.branchSettings,
  BRANCH_NO_SELLERS: ROUTES.userSettings,
  ACCOUNT_IVA_CONDITION: ROUTES.invoicingSettings,
  ISSUER_PROFILE_REQUIRED: ROUTES.invoicingSettings,
  ACCOUNT_TAX_ID: ROUTES.accountSettings,
  BRANCH_POINT_OF_SALE: ROUTES.arcaSettings,
  ARCA_CREDENTIALS: ROUTES.arcaSettings,
  ARCA_CREDENTIALS_EXPIRED: ROUTES.arcaSettings,
  ARCA_CREDENTIALS_CUIT: ROUTES.arcaSettings,
};

// Whether a gap an invoice preview reports is configuration, which SetupNotice words and routes.
export function isSetupIssue(value: string): value is SetupIssue {
  return SETUP_ISSUES.some((issue) => issue === value);
}

interface BranchMailbox {
  email: string | null;
  isActive: boolean;
}

// A branch without a mailbox sends its quotes with nowhere for the customer's reply to go.
export function missesBranchEmail(branch: Pick<BranchMailbox, 'email'>): boolean {
  return !branch.email?.trim();
}

// Whether any open branch is missing its mailbox, which is what puts a dot on the way to fix it.
export function anyBranchMissesEmail(branches: readonly BranchMailbox[]): boolean {
  return branches.some((branch) => branch.isActive && missesBranchEmail(branch));
}
