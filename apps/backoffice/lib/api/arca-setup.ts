import 'server-only';

import { apiRequest } from '@/lib/api/client';
import { arcaEnvironmentOf, type ARCAEnvironment } from '@/lib/api/invoicing';

interface ARCASetupRaw {
  enabled: boolean;
  environment: string;
  setup_environment: string | null;
  reconnect_required: boolean;
  tax_id: string;
  csr: string;
  has_certificate: boolean;
  certificate: string;
  certificate_expires_at: string | null;
}

// The connection's public material. The private key never leaves the API.
export interface ARCASetup {
  enabled: boolean;
  // Where this server authorizes invoices; HOMOLOGATION is test mode.
  environment: ARCAEnvironment;
  // The environment the connection was made for, null without one.
  setupEnvironment: ARCAEnvironment | null;
  // The connection belongs to the other environment and has to be started over.
  reconnectRequired: boolean;
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
    environment: arcaEnvironmentOf(raw.environment),
    setupEnvironment: raw.setup_environment ? arcaEnvironmentOf(raw.setup_environment) : null,
    reconnectRequired: raw.reconnect_required,
    taxId: raw.tax_id,
    csr: raw.csr,
    hasCertificate: raw.has_certificate,
    certificate: raw.certificate,
    certificateExpiresAt: raw.certificate_expires_at,
  };
}
