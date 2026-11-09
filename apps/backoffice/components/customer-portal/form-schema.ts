import { z } from 'zod';

export function portalSchema(message = 'invalid') {
  return z.object({
    subdomain: z
      .string()
      .trim()
      .toLowerCase()
      .regex(/^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/, message),
  });
}

export function portalPreview(baseUrl: string, subdomain: string): string {
  if (!baseUrl) return '';
  const base = new URL(baseUrl);
  return `${base.origin}/c/${subdomain.trim().toLowerCase() || '…'}`;
}
