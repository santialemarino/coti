import { getTranslations } from 'next-intl/server';

import { CustomerProfileForm } from '@/app/profile/_components/customer-profile-form';
import { MainContactCard } from '@/app/profile/_components/main-contact-card';
import { CustomerHeader } from '@/components/customer-header';
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
    <>
      <CustomerHeader section="profile" />
      <main className="flex min-h-[calc(100dvh-4rem)] flex-col items-center px-4 py-8 sm:px-6 sm:py-12">
        <div className="flex w-full max-w-3xl flex-col gap-y-6">
          <header className="flex items-center justify-between gap-4">
            <h1 className="text-heading-3">{t('title')}</h1>
          </header>
          <CustomerProfileForm profile={profile} />
          {/* Remounts when a verification or a save changes what the card starts from. */}
          <MainContactCard
            key={profile.contacts
              .filter((contact) => contact.verified)
              .map((contact) => `${contact.kind}:${contact.value}:${contact.main}`)
              .join('|')}
            contacts={profile.contacts}
          />
        </div>
      </main>
    </>
  );
}
