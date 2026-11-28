import Link from 'next/link';
import { ArrowUpRightIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';

import { Badge, Card, CardContent, CardHeader, CardTitle, InlineLink } from '@repo/ui/components';
import { ClientNotesEditor } from '@/components/client-notes-editor';
import { ROUTES } from '@/config/routes';
import { clientDisplayName, type ClientProfile } from '@/lib/api/client-profiles';
import { useFormatters } from '@/lib/i18n/formatters';

export function RfqClientCard({ profile }: { profile: ClientProfile }) {
  const t = useTranslations('clients.context');
  const tClients = useTranslations('clients');
  const fmt = useFormatters();
  const { client, tags, activity } = profile;
  const name = clientDisplayName(client, tClients('unnamed'));

  return (
    <Card>
      <CardHeader className="flex-row items-start justify-between gap-x-4">
        <div className="flex min-w-0 flex-col gap-y-1">
          <span className="text-paragraph-xs text-foreground-muted">{t('title')}</span>
          <CardTitle className="wrap-anywhere">{name}</CardTitle>
        </div>
        <InlineLink asChild tone="muted" className="shrink-0">
          <Link href={ROUTES.clientDetail(client.id)}>
            {t('viewProfile')}
            <ArrowUpRightIcon aria-hidden="true" className="size-4" />
          </Link>
        </InlineLink>
      </CardHeader>
      <CardContent className="flex flex-col gap-y-4">
        {tags.length > 0 ? (
          <ul className="flex flex-wrap gap-2">
            {tags.map((tag) => (
              <li key={tag.id}>
                <Badge tone="outline" size="sm">
                  {tag.name}
                </Badge>
              </li>
            ))}
          </ul>
        ) : null}
        <dl className="grid grid-cols-2 gap-x-6 gap-y-3 sm:grid-cols-4">
          <div>
            <dt className="text-paragraph-xs text-foreground-muted">{t('lastPurchase')}</dt>
            <dd className="text-paragraph-sm-semibold tabular-nums">
              {activity.lastAcceptedAt
                ? fmt.dateNumeric(activity.lastAcceptedAt)
                : t('noPurchases')}
            </dd>
          </div>
          <div>
            <dt className="text-paragraph-xs text-foreground-muted">{t('openQuotes')}</dt>
            <dd className="text-paragraph-sm-semibold tabular-nums">
              {fmt.value(activity.openQuotes)}
            </dd>
          </div>
          <div>
            <dt className="text-paragraph-xs text-foreground-muted">{t('averageTicket')}</dt>
            <dd className="text-paragraph-sm-semibold tabular-nums">
              {activity.averageTicket === null
                ? t('unavailable')
                : fmt.currency(activity.averageTicket)}
            </dd>
          </div>
        </dl>
        <ClientNotesEditor clientId={client.id} name={name} notes={client.notes} />
      </CardContent>
    </Card>
  );
}
