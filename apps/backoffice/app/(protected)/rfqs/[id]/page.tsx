import Link from 'next/link';
import { ArrowLeftIcon, PackageSearchIcon } from 'lucide-react';
import { getTranslations } from 'next-intl/server';

import { Button, Card, StatusScreen } from '@repo/ui/components';
import { ROUTES } from '@/config/routes';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('rfqsDetail');

/*
 * Placeholder for the pedido detail screen. The real quote review lives behind this route, so the
 * list has somewhere real to link.
 */
export default async function RfqDetailPage({ params }: { params: Promise<{ id: string }> }) {
  const t = await getTranslations('rfqs');
  const { id } = await params;

  return (
    <Card className="gap-y-0 overflow-hidden py-0">
      <StatusScreen
        icon={PackageSearchIcon}
        tone="info"
        title={t('detail.placeholderTitle', { id })}
        description={t('detail.placeholderDescription')}
      >
        <Button asChild variant="outline">
          <Link href={ROUTES.rfqs}>
            <ArrowLeftIcon aria-hidden="true" />
            {t('detail.back')}
          </Link>
        </Button>
      </StatusScreen>
    </Card>
  );
}
