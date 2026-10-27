import { isValidPhoneNumber } from 'libphonenumber-js/min';
import { z } from 'zod';

export function customerProfileSchema(t: (key: string) => string) {
  return z
    .object({
      name: z.string().trim().min(1, t('nameRequired')).max(255, t('nameTooLong')),
      legalName: z.string().trim().max(255, t('nameTooLong')).optional(),
      address: z.string().trim().max(255, t('nameTooLong')).optional(),
      ivaCondition: z
        .enum(['', 'REGISTERED', 'MONOTRIBUTO', 'EXEMPT', 'FINAL_CONSUMER'])
        .optional(),
      contacts: z.array(
        z.object({
          kind: z.enum(['WHATSAPP', 'EMAIL']),
          value: z.string().trim(),
          primary: z.boolean(),
        }),
      ),
    })
    .superRefine((data, ctx) => {
      if (!data.contacts.some((contact) => contact.value !== '')) {
        ctx.addIssue({ code: 'custom', path: ['root'], message: t('contactRequired') });
      }
      const seen = new Set<string>();
      for (const kind of ['WHATSAPP', 'EMAIL'] as const) {
        const filled = data.contacts
          .map((contact, index) => ({ ...contact, index }))
          .filter((contact) => contact.kind === kind && contact.value !== '');
        for (const contact of filled) {
          const value = kind === 'EMAIL' ? contact.value.toLowerCase() : contact.value;
          const valid =
            kind === 'WHATSAPP'
              ? isValidPhoneNumber(value)
              : z.email().max(255).safeParse(value).success;
          if (!valid)
            ctx.addIssue({
              code: 'custom',
              path: ['contacts', contact.index, 'value'],
              message: t(kind === 'EMAIL' ? 'invalidEmail' : 'invalidPhone'),
            });
          const key = kind + ':' + value;
          if (seen.has(key))
            ctx.addIssue({
              code: 'custom',
              path: ['contacts', contact.index, 'value'],
              message: t('duplicate'),
            });
          seen.add(key);
        }
        if (filled.length > 0 && filled.filter((contact) => contact.primary).length !== 1) {
          ctx.addIssue({
            code: 'custom',
            path: ['contacts', filled[0]!.index, 'value'],
            message: t('choosePrimary'),
          });
        }
      }
    });
}
export type CustomerProfileValues = z.infer<ReturnType<typeof customerProfileSchema>>;
