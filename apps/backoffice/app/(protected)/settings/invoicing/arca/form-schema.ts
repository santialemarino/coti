import { z } from 'zod';

import { CERTIFICATE_MAX_BYTES, CERTIFICATE_MAX_KB } from '@/lib/constants/arca';
import { rawText, type SchemaText } from '@/lib/forms/validators';

// One PEM certificate block: the shape is checked here, the certificate itself by the API.
const PEM_CERTIFICATE =
  /^-----BEGIN CERTIFICATE-----\n[A-Za-z0-9+/=\n]+\n-----END CERTIFICATE-----$/;

// What a paste is compared and sent as: trimmed, with Windows line endings turned into plain ones.
export function normalizeCertificate(raw: string): string {
  return raw.replace(/\r\n?/g, '\n').trim();
}

export function arcaCertificateSchema(t: SchemaText = rawText) {
  return z.object({
    certificate: z
      .string()
      .transform(normalizeCertificate)
      .pipe(
        z
          .string()
          .min(1, t.field('steps.upload.required'))
          .pipe(
            z
              .string()
              .max(
                CERTIFICATE_MAX_BYTES,
                t.field('steps.upload.tooLarge', { max: CERTIFICATE_MAX_KB }),
              )
              .regex(PEM_CERTIFICATE, t.field('steps.upload.invalid')),
          ),
      ),
  });
}

export type ARCACertificateValues = z.input<ReturnType<typeof arcaCertificateSchema>>;
