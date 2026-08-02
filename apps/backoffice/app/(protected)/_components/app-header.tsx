import Link from 'next/link';
import { getTranslations } from 'next-intl/server';

import { Button } from '@repo/ui/components';
import { signOut } from '@/app/(protected)/actions';
import { ROUTES } from '@/config/routes';
import type { SessionUser } from '@/lib/auth/session';

interface AppHeaderProps {
  session: SessionUser;
}

export async function AppHeader({ session }: AppHeaderProps) {
  const t = await getTranslations('common');

  return (
    <header className="flex items-center justify-between px-6 py-4 border-b">
      <Link href={ROUTES.home} className="font-bold">
        {t('appName')}
      </Link>
      <div className="flex items-center gap-x-4">
        <span className="text-sm text-muted-foreground">
          {session.name} · {t(`roles.${session.role}`)}
        </span>
        <Link href={ROUTES.changePassword} className="text-sm underline">
          {t('nav.changePassword')}
        </Link>
        <form action={signOut}>
          <Button type="submit" variant="outline" size="sm">
            {t('nav.signOut')}
          </Button>
        </form>
      </div>
    </header>
  );
}
