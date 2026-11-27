import { InboxIcon } from 'lucide-react';
import { getTranslations } from 'next-intl/server';

import { Button, Card, StatusScreen } from '@repo/ui/components';
import { CustomerAuthShell } from '@/components/customer-auth-shell';
import { PortalLink as Link } from '@/components/portal-link';
import { ROUTES } from '@/config/routes';
import { getCustomerAuthOptions } from '@/lib/api/customer-auth-options';

/*
 * A quote sent to a contact this account proved only afterwards, which a seller still has to match.
 * It names nothing about the quote: a forwarded link must not tell its reader what it holds.
 */
export async function QuoteNotInAccount() {
  const [portal, t] = await Promise.all([
    getCustomerAuthOptions(),
    getTranslations('customerVerification'),
  ]);
  return (
    <CustomerAuthShell supplierName={portal.supplierName} logoUrl={portal.logoUrl}>
      <h1 className="sr-only">{t('notInAccount.title')}</h1>
      <Card>
        <StatusScreen
          icon={InboxIcon}
          tone="info"
          title={t('notInAccount.title')}
          description={t('notInAccount.description', { supplier: portal.supplierName })}
        >
          <Button asChild size="lg">
            <Link href={ROUTES.orders}>{t('notInAccount.orders')}</Link>
          </Button>
        </StatusScreen>
      </Card>
    </CustomerAuthShell>
  );
}
