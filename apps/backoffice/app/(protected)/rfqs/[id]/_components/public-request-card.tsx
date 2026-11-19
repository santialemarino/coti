import { useTranslations } from 'next-intl';

import type { PublicRfqRequest } from '@/lib/api/rfqs';

interface PublicRequestCardProps {
  request: PublicRfqRequest;
}

export function PublicRequestCard({ request }: PublicRequestCardProps) {
  const t = useTranslations('rfqs.detail.publicRequest');
  const fields = [
    { key: 'taxId', value: request.taxId },
    { key: 'name', value: request.name },
    { key: 'email', value: request.email },
    { key: 'phone', value: request.phone },
  ];

  return (
    <section className="flex flex-col p-5 gap-y-5 border border-border bg-card rounded-1.5xl shadow-e2">
      <h2 className="text-heading-6">{t('title')}</h2>
      <dl className="grid gap-x-6 gap-y-4 sm:grid-cols-2">
        {fields
          .filter((field) => field.value)
          .map((field) => (
            <div key={field.key} className="flex min-w-0 flex-col gap-y-1">
              <dt className="text-paragraph-xs text-foreground-muted">{t(field.key)}</dt>
              <dd className="break-words text-paragraph-sm">{field.value}</dd>
            </div>
          ))}
      </dl>
    </section>
  );
}
