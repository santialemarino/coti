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

export interface ClientSaleRaw {
  quote_id: string;
  rfq_id: string;
  quote_number: number;
  branch_id: string;
  branch_name: string;
  total: string;
  accepted_at: string;
}

export interface ClientProfileRaw {
  client: ClientRaw;
  tags: ClientTagRaw[];
  sales: ClientSaleRaw[];
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

export interface ClientSale {
  quoteId: string;
  rfqId: string;
  quoteNumber: number;
  branchId: string;
  branchName: string;
  total: string;
  acceptedAt: string;
}

export interface ClientProfile {
  client: Client;
  tags: ClientTag[];
  sales: ClientSale[];
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
    sales: raw.sales.map((sale) => ({
      quoteId: sale.quote_id,
      rfqId: sale.rfq_id,
      quoteNumber: sale.quote_number,
      branchId: sale.branch_id,
      branchName: sale.branch_name,
      total: sale.total,
      acceptedAt: sale.accepted_at,
    })),
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
