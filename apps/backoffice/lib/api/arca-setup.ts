import 'server-only';

import { apiRequest } from '@/lib/api/client';

interface ARCASetupRaw {
  enabled: boolean;
  tax_id: string;
  csr: string;
  has_certificate: boolean;
  certificate: string;
  certificate_expires_at: string | null;
}

// The connection's public material. The private key never leaves the API.
export interface ARCASetup {
  enabled: boolean;
  taxId: string;
  csr: string;
  hasCertificate: boolean;
  certificate: string;
  certificateExpiresAt: string | null;
}

// The account's connection, the same whichever branch is active.
export async function getARCASetup(): Promise<ARCASetup> {
  const raw = await apiRequest<ARCASetupRaw>({ path: '/v1/arca/setup', branchScoped: false });
  return {
    enabled: raw.enabled,
    taxId: raw.tax_id,
    csr: raw.csr,
    hasCertificate: raw.has_certificate,
    certificate: raw.certificate,
    certificateExpiresAt: raw.certificate_expires_at,
  };
}
