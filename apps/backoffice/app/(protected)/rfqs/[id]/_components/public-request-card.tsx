import { useTranslations } from 'next-intl';

import { Card, CardContent, CardHeader, CardTitle } from '@repo/ui/components';
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
    <Card>
      <CardHeader>
        <CardTitle>{t('title')}</CardTitle>
      </CardHeader>
      <CardContent>
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
      </CardContent>
    </Card>
  );
}
