import type { Metadata } from 'next';
import { notFound } from 'next/navigation';
import { getTranslations } from 'next-intl/server';

import { PublicRfqForm } from '@/app/rfq/[channelId]/_components/public-rfq-form';
import { getPublicRfqDestination } from '@/lib/api/public-rfqs';

export async function generateMetadata(): Promise<Metadata> {
  const t = await getTranslations('rfq');
  return { title: `${t('title')} — Coti`, robots: { index: false, follow: false } };
}

export default async function PublicRfqPage({
  params,
}: {
  params: Promise<{ channelId: string }>;
}) {
  const { channelId } = await params;
  const destination = await getPublicRfqDestination(channelId);
  if (!destination) notFound();
  return <PublicRfqForm channelId={channelId} destination={destination} />;
}
