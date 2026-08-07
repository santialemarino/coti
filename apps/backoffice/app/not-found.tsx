import Link from 'next/link';
import { SearchXIcon } from 'lucide-react';
import { getTranslations } from 'next-intl/server';

import { Button, Card, StatusScreen } from '@repo/ui/components';
import { BrandedScreen } from '@/components/branded-screen';
import { ROUTES } from '@/config/routes';
import { getSession } from '@/lib/auth/session';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('notFound');

/*
 * Rendered by the root layout alone — a route group's layout is exactly what an unmatched URL failed
 * to reach — so it brings its own frame.
 *
 * The gate never lets a signed-out caller reach an unknown path directly: it is not a public route,
 * so they are sent to log in and arrive here afterwards through `next`. The session is read anyway,
 * because Next renders this for a `notFound()` raised inside a page too, and because sending someone
 * to a login screen they are already past is a dead end.
 */
export default async function NotFound() {
  const t = await getTranslations('notFound');
  const session = await getSession();

  return (
    <BrandedScreen>
      <Card>
        <StatusScreen
          icon={SearchXIcon}
          tone="warning"
          title={t('title')}
          description={t('description')}
        >
          <Button asChild size="lg">
            <Link href={session ? ROUTES.home : ROUTES.login}>
              {session ? t('goHome') : t('goToLogin')}
            </Link>
          </Button>
        </StatusScreen>
      </Card>
    </BrandedScreen>
  );
}
