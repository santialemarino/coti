import { redirect } from 'next/navigation';

import { AppHeader } from '@/app/(protected)/_components/app-header';
import { ROUTES } from '@/config/routes';
import { getSession } from '@/lib/auth/session';

// Middleware answers whether a token exists; this answers whether the session
// behind it is still good, which only the API knows.
export default async function ProtectedLayout({ children }: { children: React.ReactNode }) {
  const session = await getSession();
  if (!session) redirect(ROUTES.sessionEnded);

  return (
    <div className="flex flex-col min-h-screen">
      <AppHeader session={session} />
      {children}
    </div>
  );
}
