import type { Metadata } from 'next';
import { notFound } from 'next/navigation';
import { ClockAlertIcon } from 'lucide-react';
import { getTranslations } from 'next-intl/server';

import { Card, StatusScreen } from '@repo/ui/components';
import { QuoteHeader } from '@/app/quotes/[token]/_components/quote-header';
import { QuoteItems } from '@/app/quotes/[token]/_components/quote-items';
import { QuoteSummary } from '@/app/quotes/[token]/_components/quote-summary';
import { Brand } from '@/components/brand';
import { getPublicQuoteByToken } from '@/lib/api/public-quotes';
import { getFormatters } from '@/lib/i18n/formatters-server';

export async function generateMetadata(): Promise<Metadata> {
  const t = await getTranslations('quote');
  return { title: `${t('title')} — Coti`, robots: { index: false, follow: false } };
}

/*
 * The customer's whole view of a quote, opened from the link the send put in their message. The
 * token is the access control — there is no session here — so an unknown one is a 404 and an
 * expired one says so rather than showing a quote nobody should still be reading.
 *
 * Everything rendered comes from the frozen representation the send published, never from live
 * commercial data: the prices a customer sees are the prices that were sent.
 */
export default async function PublicQuotePage({ params }: { params: Promise<{ token: string }> }) {
  const { token } = await params;
  const fmt = await getFormatters();
  const t = await getTranslations('quote');
  const tCommon = await getTranslations('common');

  const send = await getPublicQuoteByToken(token);
  if (!send) notFound();

  if (send.status !== 'ACTIVE' || !send.quote) {
    return (
      <main className="flex flex-col min-h-screen items-center justify-center px-4 py-10">
        <div className="flex flex-col w-full max-w-auth-card items-center gap-y-8 animate-rise-in">
          <Brand variant="lockup" size="xl" label={tCommon('appName')} />
          <Card>
            <StatusScreen
              icon={ClockAlertIcon}
              tone="warning"
              title={t('expiredTitle')}
              description={t('expiredDescription', { date: fmt.date(send.expiresAt) })}
            />
          </Card>
        </div>
      </main>
    );
  }

  const { quote } = send;

  return (
    <main className="flex flex-col min-h-screen items-center px-4 py-8 sm:py-12">
      <div className="flex flex-col w-full max-w-3xl gap-y-6 animate-rise-in">
        <QuoteHeader
          supplier={quote.supplier}
          branch={quote.branch}
          reference={quote.reference}
          versionNumber={quote.versionNumber}
          approvedAt={quote.approvedAt}
        />

        <div className="flex flex-col gap-y-1">
          <h1 className="text-heading-3 text-foreground">
            {quote.customerName
              ? t('greeting', { name: quote.customerName })
              : t('greetingAnonymous')}
          </h1>
          <p className="text-paragraph text-foreground-muted">{t('intro')}</p>
        </div>

        <QuoteItems items={quote.items} currency={quote.currency} />

        <QuoteSummary
          discounts={quote.discounts}
          total={quote.total}
          currency={quote.currency}
          expiresAt={send.expiresAt}
          validityNote={quote.validityNote}
          pdfUrl={send.pdfUrl}
        />

        <p className="text-paragraph-sm text-foreground-muted">{t('questions')}</p>

        <footer className="flex justify-center pt-2">
          <Brand variant="wordmark" size="sm" label={tCommon('appName')} />
        </footer>
      </div>
    </main>
  );
}
