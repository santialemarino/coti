import Link from 'next/link';
import { getTranslations } from 'next-intl/server';

import { Button } from '@repo/ui/components';
import { CustomerProfileForm } from '@/app/profile/_components/customer-profile-form';
import { ROUTES } from '@/config/routes';
import { getCustomerProfile } from '@/lib/api/customer-profile';
import { requireCustomer } from '@/lib/auth/customer-flow';

export default async function CustomerProfilePage() {
  await requireCustomer(ROUTES.profile);
  const [profile, t] = await Promise.all([
    getCustomerProfile(),
    getTranslations('customerProfile'),
  ]);
  return (
    <main className="flex min-h-dvh flex-col items-center px-4 py-8 sm:px-6 sm:py-12">
      <div className="flex w-full max-w-3xl flex-col gap-y-6">
        <header className="flex items-center justify-between gap-4">
          <h1 className="text-heading-4">{t('title')}</h1>
          <Button variant="outline" asChild>
            <Link href={ROUTES.orders}>{t('orders')}</Link>
          </Button>
        </header>
        <CustomerProfileForm profile={profile} />
      </div>
    </main>
  );
}
