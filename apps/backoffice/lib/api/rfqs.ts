/*
 * The statuses a seller sees and reasons about. DRAFT is deliberately absent: it is an internal
 * quote_state while the RFQ is still GENERATED, never a visible business state (see
 * docs/internal/domain/estados.md). The wire may still carry it, so every binding point shoves it
 * through normalizeRfqStatus.
 */
export type RfqStatus =
  | 'RECEIVED'
  | 'GENERATED'
  | 'QUOTED'
  | 'SENT'
  | 'CHANGE_REQUESTED'
  | 'ACCEPTED'
  | 'REJECTED';

/*
 * The backend merges quote.current_status into the RFQ status it hands back, so a raw response can
 * say DRAFT while the seller sees GENERATED. Normalize at the display boundary: DRAFT collapses onto
 * GENERATED, everything else passes through untouched.
 */
export function normalizeRfqStatus(status: string): RfqStatus {
  const upper = status.toUpperCase();
  if (upper === 'DRAFT' || upper === 'GENERATED') return 'GENERATED';
  return upper as RfqStatus;
}

// The channel a request arrived through; an icon and an i18n label hang off each value.
export type RfqChannel = 'whatsapp' | 'email' | 'webapp' | 'manual_entry';

export type RfqPriority = 'high' | 'normal' | 'low';

export interface RfqRecord {
  id: string;
  client: string;
  createdAt: string;
  channel: RfqChannel;
  seller: string;
  // The resolved seller user id; null when the quote has none assigned yet.
  sellerId: string | null;
  branch: string;
  branchId: string;
  itemCount: number;
  /*
   * Decimal string, the wire format money travels as (NUMERIC(14,2)); currency is per-account.
   * Absent until the quote exists — an uncotized request has no amount to show.
   */
  total?: string;
  priority: RfqPriority;
  status: RfqStatus;
  // Backend-set flag: the seller must chase this quote; it surfaces first and is highlighted.
  needsFollowup: boolean;
  processing?: boolean;
  archived?: boolean;
}

// Raw shape returned by GET /v1/rfqs — the mapper lives on the server side.
export interface RfqListItem {
  id: string;
  client: string | null;
  created_at: string;
  channel: string;
  seller_id: string | null;
  seller: string;
  branch: string;
  branch_id: string;
  item_count: number;
  total: string | null;
  status: string;
  needs_followup: boolean;
  archived_at: string | null;
}

/*
 * The stand-in duration of an AI quote generation until the backend drives it; the dashboard shows
 * the processing spinner for this long when the seller marks a pedido as QUOTED.
 */
export const QUOTE_GENERATION_MS = 1200;

// Raw shape returned by GET /v1/rfqs/:rfqId — the detail view projection.
export interface RfqDetailResponse {
  rfq: RfqListItem;
  quote: QuoteResponse | null;
  version: QuoteVersionResponse | null;
  items: QuoteItemResponse[];
  alternatives: Record<string, QuoteItemAlternativeResponse[]>;
  discounts?: QuoteDiscountResponse[];
  changes_requested?: ChangeRequestDiff;
}

export interface QuoteResponse {
  id: string;
  branch_id: string;
  client_id: string | null;
  rfq_id: string;
  seller_id: string | null;
  current_version_id: string | null;
  current_status: string;
  expires_at: string | null;
  archived_at: string | null;
  needs_followup: boolean;
  followup_flagged_at: string | null;
  created_at: string;
  updated_at: string;
}

export interface QuoteVersionResponse {
  id: string;
  quote_id: string;
  author_id: string | null;
  version_number: number;
  total: string;
  is_immutable: boolean;
  comment: string | null;
  created_at: string;
}

export interface QuoteItemResponse {
  id: string;
  version_id: string;
  product_id: string | null;
  product_code: string | null;
  product_name: string | null;
  product_unit: string | null;
  requested_description: string;
  quantity: string;
  unit: string | null;
  unit_price_snapshot: string | null;
  min_price_snapshot: string | null;
  subtotal: string | null;
  confidence_score: string | null;
  match_status: string;
  alternatives: QuoteItemAlternativeResponse[];
  pricing_unavailable: boolean | null;
  quantity_rationale: string | null;
  created_at: string;
}

export interface QuoteItemAlternativeResponse {
  id: string;
  product_id: string | null;
  combo_id: string | null;
  type: string;
  origin: string;
  rank: number;
  confidence_score: string | null;
  price_snapshot: string | null;
  approved_by_seller: boolean;
  chosen_by_client: boolean;
  code: string | null;
  canonical_name: string | null;
  unit: string | null;
}

export type DiscountScope = 'ITEM' | 'ITEM_SET' | 'TOTAL';
export type DiscountOrigin = 'AUTOMATIC' | 'AI_ADAPTATION' | 'MANUAL_SELLER';

export interface QuoteDiscountResponse {
  id: string;
  quote_version_id: string;
  promotion_id: string | null;
  promotion_name: string | null;
  condition_type: string | null;
  scope: DiscountScope;
  origin: DiscountOrigin;
  amount: string;
  suppressed_by_seller: boolean;
  created_at: string;
}

export interface DiffLineItem {
  description: string;
  quantity: string;
  unit: string | null;
  unit_price: string | null;
  changed: boolean;
  change_type?: 'modified' | 'added' | 'removed';
}

export interface DiffDiscountLine {
  name: string;
  amount: string;
  changed: boolean;
}

export interface ChangeRequestDiff {
  reason: string | null;
  original: {
    items: DiffLineItem[];
    discounts: DiffDiscountLine[];
    total: string;
  };
  requested: {
    items: DiffLineItem[];
    discounts: DiffDiscountLine[];
    total: string;
  };
}
