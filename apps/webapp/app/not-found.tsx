import type { Metadata } from 'next';
import Link from 'next/link';
import { SearchXIcon } from 'lucide-react';
import { getTranslations } from 'next-intl/server';

import { Button, Card, StatusScreen } from '@repo/ui/components';
import { Brand } from '@/components/brand';

export async function generateMetadata(): Promise<Metadata> {
  const t = await getTranslations('notFound');
  return { title: `${t('title')} — Coti` };
}

/*
 * The public side has no session and no gate, so an unknown address lands straight here — including
 * a quote link whose token no longer resolves, which is the common case and the reason the copy
 * points at asking for the link again rather than at anything the visitor can fix themselves.
 */
export default async function NotFound() {
  const t = await getTranslations('notFound');
  const tCommon = await getTranslations('common');

  return (
    <main className="flex flex-col min-h-screen items-center justify-center px-4 py-10">
      <div className="flex flex-col w-full max-w-auth-card items-center gap-y-8 animate-rise-in">
        <Brand variant="lockup" size="xl" label={tCommon('appName')} />
        <Card>
          <StatusScreen
            icon={SearchXIcon}
            tone="warning"
            title={t('title')}
            description={t('description')}
          >
            <Button asChild size="lg">
              <Link href="/">{t('goHome')}</Link>
            </Button>
          </StatusScreen>
        </Card>
      </div>
    </main>
  );
}
