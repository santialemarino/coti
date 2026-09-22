import type { Metadata } from 'next';
import { notFound } from 'next/navigation';
import { getTranslations } from 'next-intl/server';

import { PublicQuoteView } from '@/app/quotes/_components/public-quote-view';
import { getPublicQuoteByToken } from '@/lib/api/public-quotes';

export async function generateMetadata(): Promise<Metadata> {
  const t = await getTranslations('quote');
  return { title: `${t('title')} — Coti`, robots: { index: false, follow: false } };
}

/*
 * The customer's whole view of a quote, opened from the link the send put in their message. The
 * token is the whole access control — there is no session here — so an unknown one asks the route
 * for its 404 and an expired one says so. The rendering lives in PublicQuoteView, shared with the
 * dev-only /quotes/preview route; this page only owns where the send comes from.
 */
export default async function PublicQuotePage({ params }: { params: Promise<{ token: string }> }) {
  const { token } = await params;

  const send = await getPublicQuoteByToken(token);
  if (!send) notFound();

  return <PublicQuoteView send={send} token={token} />;
}
