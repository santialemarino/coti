import { SettingsBackLink } from '@/app/(protected)/settings/_components/settings-back-link';
import { getSession } from '@/lib/auth/session';
import { ADMIN_ROLE } from '@/lib/constants/auth';

/*
 * The frame every settings page shares. The sections are listed in the context column from lg up;
 * below it they are the index's own page, which each section leads back to.
 */
export default async function SettingsLayout({ children }: { children: React.ReactNode }) {
  const session = await getSession();
  // Only an administrator is offered the sections, so only they have a list to go back to.
  const hasSections = session?.role === ADMIN_ROLE;

  return (
    <div className="flex flex-1 flex-col min-w-0 px-6 py-10 gap-y-8 lg:px-10">
      {hasSections ? <SettingsBackLink /> : null}
      {children}
    </div>
  );
}
