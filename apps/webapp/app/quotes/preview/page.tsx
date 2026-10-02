import type { Metadata } from 'next';
import { getTranslations } from 'next-intl/server';

import { PublicQuoteView } from '@/app/quotes/_components/public-quote-view';
import { makeSentPreviewSend } from '@/lib/api/public-quotes.fixtures';

export async function generateMetadata(): Promise<Metadata> {
  const t = await getTranslations('quote');
  return { title: `${t('title')} — Coti`, robots: { index: false, follow: false } };
}

/*
 * Dev-only visual check of the customer quote: it reuses the exact PublicQuoteView of the token
 * route, fed a fresh SENT fixture instead of an API read. Nobody can reach it through a message and
 * it writes nothing — the only way in is a developer typing the URL.
 */
export default function QuotePreviewPage() {
  return <PublicQuoteView send={makeSentPreviewSend()} />;
}
