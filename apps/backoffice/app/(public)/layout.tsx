import { PublicFooter } from '@/app/(public)/_components/public-footer';
import { PublicHeader } from '@/app/(public)/_components/public-header';
import { isAuthenticated } from '@/lib/auth/session';

// The public site's frame. A seller can open these pages too, so the header reads the session cookies
// to offer the way back to the queue instead of a login they are already past.
export default async function PublicLayout({ children }: { children: React.ReactNode }) {
  const signedIn = await isAuthenticated();

  return (
    <div className="flex flex-col min-h-screen">
      <PublicHeader signedIn={signedIn} />
      <main className="flex flex-col flex-1">{children}</main>
      <PublicFooter />
    </div>
  );
}
