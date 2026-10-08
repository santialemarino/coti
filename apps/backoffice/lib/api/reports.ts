import 'server-only';

import { apiRequest } from '@/lib/api/client';

// --- Raw types (API JSON shape, snake_case) ---

interface SellerReportRaw {
  orders_received: number;
  quotes_sent: number;
  quotes_accepted: number;
  average_quote_time_seconds: number | null;
  statuses: SellerReportStatusRaw[];
  top_materials: SellerReportMaterialRaw[];
  top_clients: SellerReportClientRaw[];
}

interface SellerReportStatusRaw {
  status: string;
  count: number;
}

interface SellerReportMaterialRaw {
  name: string;
  order_count: number;
}

interface SellerReportClientRaw {
  name: string;
  order_count: number;
}

// --- Frontend types (camelCase) ---

export interface SellerReport {
  ordersReceived: number;
  quotesSent: number;
  quotesAccepted: number;
  averageQuoteTimeSeconds: number | null;
  statuses: SellerReportStatus[];
  topMaterials: SellerReportMaterial[];
  topClients: SellerReportClient[];
}

export interface SellerReportStatus {
  status: string;
  count: number;
}

export interface SellerReportMaterial {
  name: string;
  orderCount: number;
}

export interface SellerReportClient {
  name: string;
  orderCount: number;
}

export interface SellerReportFilter {
  dateFrom?: string;
  dateTo?: string;
}

// --- Mappers ---

function mapSellerReport(raw: SellerReportRaw): SellerReport {
  return {
    ordersReceived: raw.orders_received,
    quotesSent: raw.quotes_sent,
    quotesAccepted: raw.quotes_accepted,
    averageQuoteTimeSeconds: raw.average_quote_time_seconds,
    statuses: raw.statuses.map((item) => ({ status: item.status, count: item.count })),
    topMaterials: raw.top_materials.map((item) => ({
      name: item.name,
      orderCount: item.order_count,
    })),
    topClients: raw.top_clients.map((item) => ({
      name: item.name,
      orderCount: item.order_count,
    })),
  };
}

// --- API functions ---

export async function getSellerReport(filter: SellerReportFilter): Promise<SellerReport> {
  const raw = await apiRequest<SellerReportRaw>({
    path: '/v1/reports',
    query: {
      date_from: filter.dateFrom,
      date_to: filter.dateTo,
    },
  });
  return mapSellerReport(raw);
}
