import type { ARCAEnvironment } from '@/lib/api/invoicing';

export function showsTestEnvironment(environment: ARCAEnvironment | undefined): boolean {
  return process.env.NODE_ENV !== 'production' && environment === 'HOMOLOGATION';
}
