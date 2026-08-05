import { getTranslations } from 'next-intl/server';

import { UserTable } from '@/app/(protected)/settings/users/_components/user-table';
import { getBranches } from '@/lib/api/branches';
import { getUsers } from '@/lib/api/users';
import { requireAdmin } from '@/lib/auth/session';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('userSettings');

export default async function UserSettingsPage() {
  const session = await requireAdmin();
  const t = await getTranslations('users');
  // The reach list rather than the administration one: the API only assigns a user to an active
  // branch, so offering a closed one would be a checkbox that can only fail.
  const [users, branches] = await Promise.all([getUsers(), getBranches()]);

  return (
    <main className="flex flex-col gap-y-8">
      <header className="flex flex-col gap-y-2">
        <h1 className="text-heading-2">{t('title')}</h1>
        <p className="text-paragraph text-foreground-muted">{t('description')}</p>
      </header>
      <UserTable users={users} branches={branches} currentUserId={session.userId} />
    </main>
  );
}
