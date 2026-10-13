import type { StepGuideStatus } from '@repo/ui/components';
import type { ARCASetup } from '@/lib/api/arca-setup';
import type { InvoicingBranch } from '@/lib/api/invoicing';

export const ARCA_STEPS = [
  'wsass',
  'generate',
  'copy',
  'certificate',
  'authorize',
  'upload',
  'pointsOfSale',
] as const;
export type ARCAStep = (typeof ARCA_STEPS)[number];

// The steps done on ARCA's side, which Coti cannot observe: current until the certificate arrives.
const ARCA_SIDE: readonly ARCAStep[] = ['copy', 'certificate', 'authorize', 'upload'];

// Whether every open branch can invoice, which is what finishes the connection.
export function everyActiveBranchHasPointOfSale(branches: readonly InvoicingBranch[]): boolean {
  const active = branches.filter((branch) => branch.isActive);
  return active.length > 0 && active.every((branch) => branch.pointOfSale !== null);
}

/*
 * Each step's state, from what the account holds: no request yet → enabling WSASS and generating are
 * the steps to take; a request without a certificate → everything done in ARCA plus the upload; a
 * certificate → the points of sale, done once every open branch has one.
 */
export function arcaStepStatuses(
  setup: Pick<ARCASetup, 'csr' | 'hasCertificate'>,
  branches: readonly InvoicingBranch[],
): Record<ARCAStep, StepGuideStatus> {
  const status = (step: ARCAStep): StepGuideStatus => {
    if (!setup.csr) return step === 'wsass' || step === 'generate' ? 'current' : 'pending';
    if (!setup.hasCertificate) {
      if (step === 'wsass' || step === 'generate') return 'done';
      return ARCA_SIDE.includes(step) ? 'current' : 'pending';
    }
    if (step !== 'pointsOfSale') return 'done';
    return everyActiveBranchHasPointOfSale(branches) ? 'done' : 'current';
  };
  return Object.fromEntries(ARCA_STEPS.map((step) => [step, status(step)])) as Record<
    ARCAStep,
    StepGuideStatus
  >;
}
