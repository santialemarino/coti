import 'server-only';

import { apiRequest } from '@/lib/api/client';

// --- Raw types (API JSON shape, snake_case) ---

export type ReportGranularity = 'day' | 'week' | 'month';

interface SellerReportRaw {
  range: { from: string; to: string };
  granularity: ReportGranularity;
  scope: { multi_branch: boolean; branch_id: string | null };
  filters: {
    statuses: string[];
    channel: string | null;
    seller_id: string | null;
    seller_name: string;
    client_id: string | null;
    client_name: string;
    branches: { id: string; name: string }[];
    tags: { id: string; name: string }[];
  };
  kpis: {
    rfqs_received: number;
    quotes_sent: number;
    quotes_accepted: number;
    sent_accepted: number;
    conversion_rate: string | null;
    accepted_amount: string;
    response_time: {
      average_seconds: number | null;
      median_seconds: number | null;
      sample: number;
    };
  };
  branches?: {
    branch_id: string;
    branch_name: string;
    rfqs: number;
  }[];
  quotes_by_status: { status: string; count: number }[];
  timeline: {
    date: string;
    rfqs: number;
    sent: number;
    accepted: number;
    sent_accepted: number;
    conversion_rate: string | null;
  }[];
  top_products: { product_id: string | null; name: string; requests: number }[];
  top_revenue_products: { product_id: string; name: string; revenue: string }[];
  top_clients: { client_id: string | null; name: string; accepted_amount: string }[];
  ai_match: {
    matched: number;
    total: number;
    rate: string | null;
    confidence: { low: number; medium: number; high: number };
  };
}

// --- Frontend types (camelCase) ---

export interface SellerReportFilters {
  statuses: string[];
  channel: string | null;
  sellerId: string | null;
  sellerName: string;
  clientId: string | null;
  clientName: string;
  branches: { id: string; name: string }[];
  tags: { id: string; name: string }[];
}

export interface SellerReportFilterSelection {
  from?: string;
  to?: string;
  statuses?: string[];
  channel?: string;
  sellerId?: string;
  clientId?: string;
  branchIds?: string[];
  tagIds?: string[];
  granularity?: string;
}

export interface SellerReportTrendPoint {
  // The bucket's first day inside the period: a day, a week from Monday or a month.
  date: string;
  rfqs: number;
  sent: number;
  accepted: number;
  sentAccepted: number;
  conversionRate: number | null;
}

export interface SellerReport {
  range: { from: string; to: string };
  granularity: ReportGranularity;
  scope: { multiBranch: boolean; branchId: string | null };
  filters: SellerReportFilters;
  kpis: {
    rfqsReceived: number;
    quotesSent: number;
    quotesAccepted: number;
    // Quotes first sent in the period that were accepted since: the conversion cohort.
    sentAccepted: number;
    // sentAccepted over quotesSent, read by the formatters; null when nothing was sent.
    conversionRate: number | null;
    acceptedAmount: string;
    responseTime: { averageSeconds: number | null; medianSeconds: number | null; sample: number };
  };
  branches: { branchId: string; branchName: string; rfqs: number }[];
  quotesByStatus: { status: string; count: number }[];
  timeline: SellerReportTrendPoint[];
  topProducts: { productId: string | null; name: string; requests: number }[];
  topRevenueProducts: { productId: string; name: string; revenue: string }[];
  topClients: { clientId: string | null; name: string; acceptedAmount: string }[];
  aiMatch: {
    matched: number;
    total: number;
    rate: number | null;
    confidence: { low: number; medium: number; high: number };
  };
}

// --- Mappers ---

function rate(raw: string | null): number | null {
  return raw === null ? null : Number(raw);
}

function mapSellerReport(raw: SellerReportRaw): SellerReport {
  return {
    range: { from: raw.range.from, to: raw.range.to },
    granularity: raw.granularity,
    scope: { multiBranch: raw.scope.multi_branch, branchId: raw.scope.branch_id },
    filters: {
      statuses: raw.filters.statuses,
      channel: raw.filters.channel,
      sellerId: raw.filters.seller_id,
      sellerName: raw.filters.seller_name,
      clientId: raw.filters.client_id,
      clientName: raw.filters.client_name,
      branches: raw.filters.branches ?? [],
      tags: raw.filters.tags ?? [],
    },
    kpis: {
      rfqsReceived: raw.kpis.rfqs_received,
      quotesSent: raw.kpis.quotes_sent,
      quotesAccepted: raw.kpis.quotes_accepted,
      sentAccepted: raw.kpis.sent_accepted,
      conversionRate: rate(raw.kpis.conversion_rate),
      acceptedAmount: raw.kpis.accepted_amount,
      responseTime: {
        averageSeconds: raw.kpis.response_time.average_seconds,
        medianSeconds: raw.kpis.response_time.median_seconds,
        sample: raw.kpis.response_time.sample,
      },
    },
    branches: (raw.branches ?? []).map((branch) => ({
      branchId: branch.branch_id,
      branchName: branch.branch_name,
      rfqs: branch.rfqs,
    })),
    quotesByStatus: raw.quotes_by_status.map((item) => ({
      status: item.status,
      count: item.count,
    })),
    timeline: raw.timeline.map((item) => ({
      date: item.date,
      rfqs: item.rfqs,
      sent: item.sent,
      accepted: item.accepted,
      sentAccepted: item.sent_accepted,
      conversionRate: rate(item.conversion_rate),
    })),
    topProducts: raw.top_products.map((item) => ({
      productId: item.product_id,
      name: item.name,
      requests: item.requests,
    })),
    topRevenueProducts: raw.top_revenue_products.map((item) => ({
      productId: item.product_id,
      name: item.name,
      revenue: item.revenue,
    })),
    topClients: raw.top_clients.map((item) => ({
      clientId: item.client_id,
      name: item.name,
      acceptedAmount: item.accepted_amount,
    })),
    aiMatch: {
      matched: raw.ai_match.matched,
      total: raw.ai_match.total,
      rate: rate(raw.ai_match.rate),
      confidence: raw.ai_match.confidence,
    },
  };
}

// --- API functions ---

// The seller dashboard for one inclusive period; omitting both dates asks for the last 30 days.
export async function getSellerReport(
  filters: SellerReportFilterSelection = {},
): Promise<SellerReport> {
  const raw = await apiRequest<SellerReportRaw>({
    path: '/v1/reports',
    query: {
      from: filters.from,
      to: filters.to,
      status: filters.statuses?.length ? filters.statuses.join(',') : undefined,
      channel: filters.channel,
      seller_id: filters.sellerId,
      client_id: filters.clientId,
      branch_id: filters.branchIds?.length ? filters.branchIds.join(',') : undefined,
      tag_id: filters.tagIds?.length ? filters.tagIds.join(',') : undefined,
      granularity: filters.granularity,
    },
  });
  return mapSellerReport(raw);
}
