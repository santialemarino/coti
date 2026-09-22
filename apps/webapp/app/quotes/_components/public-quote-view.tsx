import { ClockAlertIcon } from 'lucide-react';
import { getTranslations } from 'next-intl/server';

import { Card, StatusScreen } from '@repo/ui/components';
import { QuoteActions } from '@/app/quotes/[token]/_components/quote-actions';
import { QuoteHeader } from '@/app/quotes/[token]/_components/quote-header';
import { QuoteInfo } from '@/app/quotes/[token]/_components/quote-info';
import { QuoteItems } from '@/app/quotes/[token]/_components/quote-items';
import { Brand } from '@/components/brand';
import type { PublicQuoteSend } from '@/lib/api/public-quotes';
import { getFormatters } from '@/lib/i18n/formatters-server';

interface PublicQuoteViewProps {
  send: PublicQuoteSend;
  /* The token the answer posts to; the dev-only preview route leaves it undefined. */
  token?: string;
}

/*
 * The customer's whole view of a published quote, whatever read produced it: the token route feeds
 * it the live getPublicQuoteByToken result, the dev-only /quotes/preview route a frozen fixture.
 * Either way it renders the frozen representation the send published, never live commercial data.
 */
export async function PublicQuoteView({ send, token }: PublicQuoteViewProps) {
  const fmt = await getFormatters();
  const t = await getTranslations('quote');
  const tCommon = await getTranslations('common');

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
      <div className="flex flex-col w-full max-w-6xl gap-y-6 animate-rise-in">
        <QuoteHeader
          supplier={quote.supplier}
          branch={quote.branch}
          reference={quote.reference}
          versionNumber={quote.versionNumber}
          approvedAt={quote.approvedAt}
        />

        <div className="flex flex-col gap-y-1">
          <h1 className="text-heading-4 text-foreground">
            {quote.customerName
              ? t('greeting', { name: quote.customerName })
              : t('greetingAnonymous')}
          </h1>
          <p className="text-paragraph text-foreground-muted">{t('intro')}</p>
        </div>

        <QuoteItems
          items={quote.items}
          discounts={quote.discounts}
          total={quote.total}
          currency={quote.currency}
        />

        <QuoteInfo
          expiresAt={send.expiresAt}
          validityNote={quote.validityNote}
          pdfUrl={send.pdfUrl}
        />

        <QuoteActions token={token} customerStatus={send.customerStatus} />

        <footer className="flex flex-col gap-y-2 pt-2 sm:flex-row sm:items-center sm:justify-between">
          <Brand variant="wordmark" size="sm" label={tCommon('appName')} />
          <p className="text-paragraph-sm text-foreground-muted">{t('questions')}</p>
        </footer>
      </div>
    </main>
  );
}
