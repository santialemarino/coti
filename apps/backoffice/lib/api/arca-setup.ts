import 'server-only';

import { apiRequest } from '@/lib/api/client';

interface ARCASetupRaw {
  enabled: boolean;
  tax_id: string;
  csr: string;
  has_certificate: boolean;
  certificate_expires_at: string | null;
  point_of_sale: number;
  verified_at: string | null;
}

export interface ARCASetup {
  enabled: boolean;
  taxId: string;
  csr: string;
  hasCertificate: boolean;
  certificateExpiresAt: string | null;
  pointOfSale: number;
  verifiedAt: string | null;
}

export type ARCAFailure =
  | 'CERTIFICATE_INVALID'
  | 'WSAA_REJECTED'
  | 'TICKET_ACTIVE'
  | 'POINT_OF_SALE_REJECTED'
  | 'WSFE_REJECTED'
  | 'UNAVAILABLE';

export interface ARCAConnection {
  verified: boolean;
  failure: ARCAFailure | '';
  lastNumber: number;
}

export async function getARCASetup(): Promise<ARCASetup> {
  const raw = await apiRequest<ARCASetupRaw>({ path: '/v1/arca/setup' });
  return {
    enabled: raw.enabled,
    taxId: raw.tax_id,
    csr: raw.csr,
    hasCertificate: raw.has_certificate,
    certificateExpiresAt: raw.certificate_expires_at,
    pointOfSale: raw.point_of_sale,
    verifiedAt: raw.verified_at,
  };
}

export async function checkARCAConnection(
  pointOfSale: number,
  branchId: string,
): Promise<ARCAConnection> {
  const raw = await apiRequest<{
    verified: boolean;
    failure: ARCAFailure | '';
    last_number: number;
  }>({
    path: '/v1/arca/setup/verify',
    method: 'POST',
    body: { point_of_sale: pointOfSale },
    branchId,
  });
  return { verified: raw.verified, failure: raw.failure, lastNumber: raw.last_number };
}
