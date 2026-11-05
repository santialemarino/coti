import { getTranslations } from 'next-intl/server';

import { Card, CardContent, CardHeader, CardTitle } from '@repo/ui/components';
import { ContactEmailConfirmation } from '@/app/verify-contact/_components/contact-email-confirmation';
import { ROUTES } from '@/config/routes';
import { requireCustomer } from '@/lib/auth/customer-flow';

export default async function Page({
  searchParams,
}: {
  searchParams: Promise<{ challenge?: string; token?: string }>;
}) {
  const query = await searchParams;
  const destination =
    ROUTES.verifyContact +
    '?' +
    new URLSearchParams({ challenge: query.challenge || '', token: query.token || '' });
  await requireCustomer(destination);
  const t = await getTranslations('customerProfile');
  return (
    <main className="flex min-h-dvh items-center justify-center px-4 py-10">
      <Card className="max-w-sm">
        <CardHeader>
          <CardTitle>
            <h1>{t('confirmContact')}</h1>
          </CardTitle>
        </CardHeader>
        <CardContent>
          <ContactEmailConfirmation challenge={query.challenge || ''} token={query.token || ''} />
        </CardContent>
      </Card>
    </main>
  );
}
