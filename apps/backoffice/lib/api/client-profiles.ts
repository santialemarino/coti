import { normalizeRfqStatus, type RfqStatus } from '@/lib/api/rfqs';

export interface ClientTagRaw {
  id: string;
  name: string;
  created_at: string;
}

export interface ClientRaw {
  id: string;
  name: string | null;
  phone: string | null;
  email: string | null;
  origin_channel: string | null;
  notes: string | null;
  created_at: string;
  updated_at: string;
}

export interface ClientSummaryRaw extends ClientRaw {
  tags: ClientTagRaw[];
  accepted_quote_count: number;
  last_accepted_at: string | null;
}

export interface ClientListRaw {
  items: ClientSummaryRaw[];
}

export interface ClientTagListRaw {
  items: ClientTagRaw[];
}

export interface ClientQuoteRaw {
  quote_id: string;
  rfq_id: string;
  quote_number: number;
  branch_id: string;
  branch_name: string;
  status: string;
  archived: boolean;
  total: string | null;
  seller_name: string | null;
  created_at: string;
  updated_at: string;
}

export interface ClientActivitySummaryRaw {
  total_quotes: number;
  accepted_quotes: number;
  rejected_quotes: number;
  pending_quotes: number;
  last_interaction_at: string | null;
}

export interface ClientProfileRaw {
  client: ClientRaw;
  tags: ClientTagRaw[];
  summary: ClientActivitySummaryRaw;
  quotes: ClientQuoteRaw[];
}

export interface ClientMatchRaw {
  client: ClientRaw;
  tags: ClientTagRaw[];
}

export interface QuoteClientAssociationRaw {
  current_client: ClientMatchRaw | null;
  suggestions: ClientMatchRaw[];
  contact_hints: { phone: string | null; email: string | null };
  available_tags: ClientTagRaw[];
}

export interface ClientTag {
  id: string;
  name: string;
  createdAt: string;
}

export interface Client {
  id: string;
  name: string | null;
  phone: string | null;
  email: string | null;
  originChannel: string | null;
  notes: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface ClientSummary extends Client {
  tags: ClientTag[];
  acceptedQuoteCount: number;
  lastAcceptedAt: string | null;
}

export interface ClientQuote {
  quoteId: string;
  rfqId: string;
  quoteNumber: number;
  branchId: string;
  branchName: string;
  status: RfqStatus;
  archived: boolean;
  total: string | null;
  sellerName: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface ClientActivitySummary {
  totalQuotes: number;
  acceptedQuotes: number;
  rejectedQuotes: number;
  pendingQuotes: number;
  lastInteractionAt: string | null;
}

export interface ClientProfile {
  client: Client;
  tags: ClientTag[];
  summary: ClientActivitySummary;
  quotes: ClientQuote[];
}

export interface ClientMatch {
  client: Client;
  tags: ClientTag[];
}

export interface QuoteClientAssociation {
  currentClient: ClientMatch | null;
  suggestions: ClientMatch[];
  contactHints: { phone: string | null; email: string | null };
  availableTags: ClientTag[];
}

export function mapClient(raw: ClientRaw): Client {
  return {
    id: raw.id,
    name: raw.name,
    phone: raw.phone,
    email: raw.email,
    originChannel: raw.origin_channel,
    notes: raw.notes,
    createdAt: raw.created_at,
    updatedAt: raw.updated_at,
  };
}

export function mapClientTag(raw: ClientTagRaw): ClientTag {
  return { id: raw.id, name: raw.name, createdAt: raw.created_at };
}

export function mapClientSummary(raw: ClientSummaryRaw): ClientSummary {
  return {
    ...mapClient(raw),
    tags: raw.tags.map(mapClientTag),
    acceptedQuoteCount: raw.accepted_quote_count,
    lastAcceptedAt: raw.last_accepted_at,
  };
}

export function clientMatchFromSummary(summary: ClientSummary): ClientMatch {
  return {
    client: {
      id: summary.id,
      name: summary.name,
      phone: summary.phone,
      email: summary.email,
      originChannel: summary.originChannel,
      notes: summary.notes,
      createdAt: summary.createdAt,
      updatedAt: summary.updatedAt,
    },
    tags: summary.tags,
  };
}

export function mapClientProfile(raw: ClientProfileRaw): ClientProfile {
  return {
    client: mapClient(raw.client),
    tags: raw.tags.map(mapClientTag),
    summary: {
      totalQuotes: raw.summary.total_quotes,
      acceptedQuotes: raw.summary.accepted_quotes,
      rejectedQuotes: raw.summary.rejected_quotes,
      pendingQuotes: raw.summary.pending_quotes,
      lastInteractionAt: raw.summary.last_interaction_at,
    },
    quotes: raw.quotes.map(mapClientQuote),
  };
}

export function mapClientQuote(raw: ClientQuoteRaw): ClientQuote {
  return {
    quoteId: raw.quote_id,
    rfqId: raw.rfq_id,
    quoteNumber: raw.quote_number,
    branchId: raw.branch_id,
    branchName: raw.branch_name,
    status: normalizeRfqStatus(raw.status),
    archived: raw.archived,
    total: raw.total,
    sellerName: raw.seller_name,
    createdAt: raw.created_at,
    updatedAt: raw.updated_at,
  };
}

export function mapClientMatch(raw: ClientMatchRaw): ClientMatch {
  return { client: mapClient(raw.client), tags: raw.tags.map(mapClientTag) };
}

export function mapQuoteClientAssociation(raw: QuoteClientAssociationRaw): QuoteClientAssociation {
  return {
    currentClient: raw.current_client ? mapClientMatch(raw.current_client) : null,
    suggestions: raw.suggestions.map(mapClientMatch),
    contactHints: {
      phone: raw.contact_hints.phone,
      email: raw.contact_hints.email,
    },
    availableTags: raw.available_tags.map(mapClientTag),
  };
}

export function clientDisplayName(client: Client, fallback: string): string {
  return client.name ?? client.email ?? client.phone ?? fallback;
}
