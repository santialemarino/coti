import 'server-only';

import { apiRequest } from '@/lib/api/client';

// --- Raw types (API JSON shape, snake_case) ---

interface SellerReportRaw {
  range: { from: string; to: string };
  scope: { multi_branch: boolean; branch_id: string | null };
  kpis: {
    rfqs_received: number;
    quotes_sent: number;
    quotes_accepted: number;
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
  timeline: { date: string; sent: number; accepted: number }[];
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

export interface SellerReport {
  range: { from: string; to: string };
  scope: { multiBranch: boolean; branchId: string | null };
  kpis: {
    rfqsReceived: number;
    quotesSent: number;
    quotesAccepted: number;
    // Ratio read by the formatters, or null when nothing was sent.
    conversionRate: number | null;
    acceptedAmount: string;
    responseTime: { averageSeconds: number | null; medianSeconds: number | null; sample: number };
  };
  branches: { branchId: string; branchName: string; rfqs: number }[];
  quotesByStatus: { status: string; count: number }[];
  timeline: { date: string; sent: number; accepted: number }[];
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
    scope: { multiBranch: raw.scope.multi_branch, branchId: raw.scope.branch_id },
    kpis: {
      rfqsReceived: raw.kpis.rfqs_received,
      quotesSent: raw.kpis.quotes_sent,
      quotesAccepted: raw.kpis.quotes_accepted,
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
      sent: item.sent,
      accepted: item.accepted,
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
export async function getSellerReport(from?: string, to?: string): Promise<SellerReport> {
  const raw = await apiRequest<SellerReportRaw>({
    path: '/v1/reports',
    query: { from, to },
  });
  return mapSellerReport(raw);
}
