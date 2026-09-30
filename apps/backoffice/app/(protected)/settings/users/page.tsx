import { getTranslations } from 'next-intl/server';

import { PageHeader } from '@/app/(protected)/_components/page-header';
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
      <PageHeader title={t('title')} />
      <UserTable users={users} branches={branches} currentUserId={session.userId} />
    </main>
  );
}
