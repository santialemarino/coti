import {
  BellRingIcon,
  HistoryIcon,
  InboxIcon,
  LayersIcon,
  MessageSquareReplyIcon,
  PercentIcon,
  ReceiptTextIcon,
  ScanSearchIcon,
  StoreIcon,
} from 'lucide-react';
import { getTranslations } from 'next-intl/server';

import { LandingSection } from '@/app/(public)/_components/landing-section';
import { revealItem } from '@/app/(public)/_components/reveal-item';

// The detail for the buyer who reads before deciding: everything a corralón gets, past the three
// headline benefits.
const FEATURES = [
  { key: 'intake', icon: InboxIcon },
  { key: 'matching', icon: ScanSearchIcon },
  { key: 'alternatives', icon: LayersIcon },
  { key: 'discounts', icon: PercentIcon },
  { key: 'versions', icon: HistoryIcon },
  { key: 'followUp', icon: BellRingIcon },
  { key: 'changes', icon: MessageSquareReplyIcon },
  { key: 'invoicing', icon: ReceiptTextIcon },
  { key: 'branches', icon: StoreIcon },
] as const;

export async function LandingFeatures() {
  const t = await getTranslations('landing.features');

  return (
    <LandingSection
      id="features"
      eyebrow={t('eyebrow')}
      title={t('title')}
      description={t('description')}
    >
      <ul className="grid gap-x-8 gap-y-8 sm:grid-cols-2 lg:grid-cols-3">
        {FEATURES.map(({ key, icon: Icon }, index) => (
          <li key={key} {...revealItem(index + 1, 'flex items-start gap-x-4')}>
            <span className="grid size-10 shrink-0 place-items-center bg-accent rounded-lg">
              <Icon aria-hidden="true" className="size-5 text-primary" />
            </span>
            <div className="flex flex-col gap-y-1">
              <h3 className="text-heading-6 text-foreground">{t(`items.${key}.title`)}</h3>
              <p className="text-paragraph-sm text-foreground-muted">
                {t(`items.${key}.description`)}
              </p>
            </div>
          </li>
        ))}
      </ul>
    </LandingSection>
  );
}
