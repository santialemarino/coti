import { z } from 'zod';

import type { ARCAEnvironment } from '@/lib/api/invoicing';
import { CERTIFICATE_MAX_BYTES, CERTIFICATE_MAX_KB } from '@/lib/constants/arca';
import { rawText, type SchemaText } from '@/lib/forms/validators';

// One PEM certificate block: the shape is checked here, the certificate itself by the API.
const PEM_CERTIFICATE =
  /^-----BEGIN CERTIFICATE-----\n[A-Za-z0-9+/=\n]+\n-----END CERTIFICATE-----$/;
// The request Coti generated, which is easy to bring back by mistake.
const PEM_REQUEST = '-----BEGIN CERTIFICATE REQUEST-----';

export type CertificateContent = 'certificate' | 'request' | 'other';

// What a paste is compared and sent as: trimmed, with Windows line endings turned into plain ones.
export function normalizeCertificate(raw: string): string {
  return raw.replace(/\r\n?/g, '\n').trim();
}

// What a text holds, whether it was pasted or read from a file.
export function certificateContent(raw: string): CertificateContent {
  const text = normalizeCertificate(raw);
  if (text.includes(PEM_REQUEST)) return 'request';
  return PEM_CERTIFICATE.test(text) ? 'certificate' : 'other';
}

// Production hands the certificate over as a file, so its empty-field message leads with the file.
export function arcaCertificateSchema(
  t: SchemaText = rawText,
  environment: ARCAEnvironment = 'HOMOLOGATION',
) {
  return z.object({
    certificate: z
      .string()
      .transform(normalizeCertificate)
      .pipe(
        z
          .string()
          .min(1, t.field(`steps.upload.${environment}.required`))
          .max(CERTIFICATE_MAX_BYTES, t.field('steps.upload.tooLarge', { max: CERTIFICATE_MAX_KB }))
          .superRefine((text, ctx) => {
            const content = certificateContent(text);
            if (content === 'certificate') return;
            ctx.addIssue({
              code: 'custom',
              message: t.field(
                content === 'request' ? 'steps.upload.pastedRequest' : 'steps.upload.invalid',
              ),
            });
          }),
      ),
  });
}

export type ARCACertificateValues = z.input<ReturnType<typeof arcaCertificateSchema>>;
