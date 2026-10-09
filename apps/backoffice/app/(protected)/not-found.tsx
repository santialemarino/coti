import Link from 'next/link';
import { SearchXIcon } from 'lucide-react';
import { getTranslations } from 'next-intl/server';

import { Button, Card, StatusScreen } from '@repo/ui/components';
import { ROUTES } from '@/config/routes';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('notFound');

// A `notFound()` raised by a screen inside the app: the shell stays, and the way out is the queue.
export default async function ProtectedNotFound() {
  const t = await getTranslations('notFound');

  return (
    <div className="flex flex-1 items-center justify-center px-4 py-16">
      <div className="flex flex-col w-full max-w-auth-card">
        <Card>
          <StatusScreen
            icon={SearchXIcon}
            tone="warning"
            title={t('title')}
            description={t('description')}
          >
            <Button asChild size="lg">
              <Link href={ROUTES.home}>{t('goToOrders')}</Link>
            </Button>
          </StatusScreen>
        </Card>
      </div>
    </div>
  );
}
