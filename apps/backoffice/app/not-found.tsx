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
 * to reach — so it brings its own frame. The gate lets a signed-out caller reach it directly, so the
 * way out depends on who is looking: the queue for a seller, the public site for anyone else.
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
            <Link href={ROUTES.home}>{signedIn ? t('goToOrders') : t('backHome')}</Link>
          </Button>
          {signedIn ? (
            <Button asChild variant="outline" size="lg">
              <Link href={ROUTES.landing}>{t('goToSite')}</Link>
            </Button>
          ) : null}
        </StatusScreen>
      </Card>
    </BrandedScreen>
  );
}
