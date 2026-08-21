// Native statuses the domain defines; labels are frontend i18n (see docs/internal/domain/estados.md).
export type RfqStatus =
  | 'RECEIVED'
  | 'GENERATED'
  | 'DRAFT'
  | 'QUOTED'
  | 'SENT'
  | 'CHANGE_REQUESTED'
  | 'ACCEPTED'
  | 'REJECTED';

// The channel a request arrived through; an icon and an i18n label hang off each value.
export type RfqChannel = 'whatsapp' | 'email' | 'webapp' | 'manual_entry';

export type RfqPriority = 'high' | 'normal' | 'low';

export interface RfqRecord {
  id: string;
  client: string;
  createdAt: string;
  channel: RfqChannel;
  seller: string;
  branch: string;
  itemCount: number;
  /*
   * Decimal string, the wire format money travels as (NUMERIC(14,2)); currency is per-account.
   * Absent until the quote exists — an uncotized request has no amount to show.
   */
  total?: string;
  priority: RfqPriority;
  status: RfqStatus;
  processing?: boolean;
  archived?: boolean;
}

// Raw shape returned by GET /v1/rfqs — the mapper lives on the server side.
export interface RfqListItem {
  id: string;
  client: string | null;
  created_at: string;
  channel: string;
  seller: string;
  branch: string;
  item_count: number;
  total: string | null;
  status: string;
  archived_at: string | null;
}

/*
 * The stand-in duration of an AI quote generation until the backend drives it; the dashboard shows
 * the processing spinner for this long when the seller marks a pedido as QUOTED.
 */
export const QUOTE_GENERATION_MS = 1200;
