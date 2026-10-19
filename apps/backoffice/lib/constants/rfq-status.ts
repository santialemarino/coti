import type { RfqStatus } from '@/lib/api/rfqs';

/*
 * The status vocabulary lives outside any component so both the client badge and server pages can
 * read it. It carries no 'use client', so importing it from a Server Component stays a value import
 * rather than a client reference.
 */

// The order the domain documents them in, used by the tabs and by sorting.
export const STATUS_ORDER: readonly RfqStatus[] = [
  'RECEIVED',
  'FAILED',
  'GENERATED',
  'QUOTED',
  'SENT',
  'CHANGE_REQUESTED',
  'ACCEPTED',
  'REJECTED',
];

/*
 * The single mapping from a domain state to its colours, written out verbatim so Tailwind emits each
 * utility. Each state names a wash, a hairline and a label, exactly the way the badge's own
 * success/warning/danger tones are built — so a status pill is visibly one of Coti's pills that
 * happens to be purple, rather than a foreign object that happens to be on a Coti screen. RECEIVED
 * has no pill colours: it never renders a badge, it always shows the ingestion spinner.
 */
export const STATUS_COLOUR: Record<RfqStatus, string> = {
  RECEIVED: '',
  FAILED: 'bg-status-failed-subtle border-status-failed-border text-status-failed-foreground',
  GENERATED:
    'bg-status-generated-subtle border-status-generated-border text-status-generated-foreground',
  QUOTED: 'bg-status-quoted-subtle border-status-quoted-border text-status-quoted-foreground',
  SENT: 'bg-status-sent-subtle border-status-sent-border text-status-sent-foreground',
  CHANGE_REQUESTED:
    'bg-status-change-requested-subtle border-status-change-requested-border text-status-change-requested-foreground',
  ACCEPTED:
    'bg-status-accepted-subtle border-status-accepted-border text-status-accepted-foreground',
  REJECTED:
    'bg-status-rejected-subtle border-status-rejected-border text-status-rejected-foreground',
};

/*
 * The dot carries the full-strength hex, which is what keeps each state recognisable at a glance.
 * RECEIVED's neutral dot only appears beside a label, such as a queue group's heading.
 */
export const STATUS_DOT: Record<RfqStatus, string> = {
  RECEIVED: 'bg-foreground-subtle',
  FAILED: 'bg-status-failed',
  GENERATED: 'bg-status-generated',
  QUOTED: 'bg-status-quoted',
  SENT: 'bg-status-sent',
  CHANGE_REQUESTED: 'bg-status-change-requested',
  ACCEPTED: 'bg-status-accepted',
  REJECTED: 'bg-status-rejected',
};

/*
 * Only statuses whose quote exists and is final enough to carry an amount show a total in the list;
 * the rest render "-". CHANGE_REQUESTED is included in the no-total set on purpose: until the
 * seller approves a number, the amount is not a promise the table should state.
 */
const QUOTE_TOTAL_STATUSES: ReadonlySet<RfqStatus> = new Set([
  'QUOTED',
  'SENT',
  'ACCEPTED',
  'REJECTED',
]);

export function hasQuoteTotal(status: RfqStatus): boolean {
  return QUOTE_TOTAL_STATUSES.has(status);
}
