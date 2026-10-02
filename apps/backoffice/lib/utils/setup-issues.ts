import { ROUTES } from '@/config/routes';

/*
 * A piece of configuration whose absence changes what the product does. Each one is reported
 * inline, where the missing piece bites, and never papered over with a fallback the reader
 * cannot see.
 */
export const SETUP_ISSUES = ['BRANCH_EMAIL', 'BRANCH_NO_SELLERS', 'NO_INTAKE_CHANNELS'] as const;

export type SetupIssue = (typeof SETUP_ISSUES)[number];

// Where an administrator fixes each one. An issue with no screen of its own has no entry.
export const SETUP_FIX_ROUTE: Partial<Record<SetupIssue, string>> = {
  BRANCH_EMAIL: ROUTES.branchSettings,
  BRANCH_NO_SELLERS: ROUTES.userSettings,
};

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
