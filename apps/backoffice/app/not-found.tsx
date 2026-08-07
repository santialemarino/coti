import Link from 'next/link';
import { SearchXIcon } from 'lucide-react';
import { getTranslations } from 'next-intl/server';

import { Button, Card, StatusScreen } from '@repo/ui/components';
import { BrandedScreen } from '@/components/branded-screen';
import { ROUTES } from '@/config/routes';
import { getAccessToken } from '@/lib/auth/session';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('notFound');

/*
 * Rendered by the root layout alone — a route group's layout is exactly what an unmatched URL failed
 * to reach — so it brings its own frame.
 *
 * The gate never lets a signed-out caller reach an unknown path directly: it is not a public route,
 * so they are sent to log in and arrive here afterwards through `next`. Which way out to offer is
 * still worth deciding, because Next renders this for a `notFound()` raised inside a page too, and
 * sending someone to a login screen they are already past is a dead end.
 *
 * The token is read from the cookie rather than validated against the API: this is a guess about
 * where to send someone, not an authorization decision, and `getSession` throws when the API is
 * unreachable — which would replace "this page does not exist" with an error screen over an outage
 * that has nothing to do with it. A stale token costs one bounce off the gate, which handles it.
 */
export default async function NotFound() {
  const t = await getTranslations('notFound');
  const signedIn = (await getAccessToken()) !== undefined;

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
            <Link href={signedIn ? ROUTES.home : ROUTES.login}>
              {signedIn ? t('goHome') : t('goToLogin')}
            </Link>
          </Button>
        </StatusScreen>
      </Card>
    </BrandedScreen>
  );
}
