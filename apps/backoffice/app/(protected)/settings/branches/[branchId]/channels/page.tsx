import { notFound } from 'next/navigation';
import { getTranslations } from 'next-intl/server';

import { Callout } from '@repo/ui/components';
import { PageHeader } from '@/app/(protected)/_components/page-header';
import { BranchEmailSettings } from '@/app/(protected)/settings/branches/[branchId]/channels/_components/branch-email-settings';
import { WhatsAppChannelSettings } from '@/app/(protected)/settings/branches/[branchId]/channels/_components/whatsapp-channel-settings';
import { getBranchChannels, getWhatsAppSignupSettings } from '@/lib/api/branch-channels';
import { getBranches } from '@/lib/api/branches';
import { requireAdmin } from '@/lib/auth/session';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('branchChannels');

interface BranchChannelsPageProps {
  params: Promise<{ branchId: string }>;
}

export default async function BranchChannelsPage({ params }: BranchChannelsPageProps) {
  await requireAdmin();
  const t = await getTranslations('branchChannels');
  const { branchId } = await params;
  const branches = await getBranches();
  const branch = branches.find((item) => item.id === branchId);
  if (!branch) notFound();
  const channels = await getBranchChannels(branch.id);
  const whatsappChannels = channels.filter((item) => item.type === 'WHATSAPP');

  const signups = await Promise.all(
    whatsappChannels.map((channel) =>
      getWhatsAppSignupSettings(branch.id, channel.id).catch(() => undefined),
    ),
  );

  return (
    <main className="flex flex-col gap-y-8">
      <PageHeader title={t('title', { name: branch.name })} description={t('description')} />
      <div className="grid max-w-4xl items-start gap-6 xl:grid-cols-2">
        <BranchEmailSettings branch={branch} />
        {whatsappChannels.map((channel, index) => (
          <WhatsAppChannelSettings
            key={channel.id}
            branchName={branch.name}
            channel={channel}
            signup={signups[index]}
          />
        ))}
        {whatsappChannels.length === 0 ? (
          <Callout tone="warning">{t('noWhatsAppChannel')}</Callout>
        ) : null}
      </div>
    </main>
  );
}
