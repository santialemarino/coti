import type { ARCASetup } from '@/lib/api/arca-setup';
import type { InvoicingBranch } from '@/lib/api/invoicing';

const DAY_MS = 24 * 60 * 60 * 1000;

/*
 * Where the account's ARCA connection stands. The status card and the guide both read it, so the
 * two can never disagree. Ordered by what blocks invoicing first: an expired certificate before a
 * missing point of sale, and either before a certificate that is only about to expire.
 */
export type ARCAConnectionState =
  | 'disabled'
  | 'notConnected'
  | 'reconnect'
  | 'inProgress'
  | 'expired'
  | 'pointsMissing'
  | 'expiringSoon'
  | 'connected';

// Whether every open branch can invoice, which is what finishes the connection.
export function everyActiveBranchHasPointOfSale(branches: readonly InvoicingBranch[]): boolean {
  const active = branches.filter((branch) => branch.isActive);
  return active.length > 0 && active.every((branch) => branch.pointOfSale !== null);
}

export function arcaConnectionState(
  setup: Pick<
    ARCASetup,
    'enabled' | 'csr' | 'hasCertificate' | 'certificateExpiresAt' | 'reconnectRequired'
  >,
  branches: readonly InvoicingBranch[],
  now: string,
  renewalDays: number,
): ARCAConnectionState {
  if (!setup.enabled) return 'disabled';
  if (!setup.csr) return 'notConnected';
  if (setup.reconnectRequired) return 'reconnect';
  if (!setup.hasCertificate) return 'inProgress';
  const expires = setup.certificateExpiresAt ? Date.parse(setup.certificateExpiresAt) : null;
  const today = Date.parse(now);
  if (expires !== null && expires <= today) return 'expired';
  if (!everyActiveBranchHasPointOfSale(branches)) return 'pointsMissing';
  if (expires !== null && expires - today <= renewalDays * DAY_MS) return 'expiringSoon';
  return 'connected';
}
