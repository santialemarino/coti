import { getTranslations } from 'next-intl/server';

import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
  DropdownChevron,
} from '@repo/ui/components';
import { LandingSection } from '@/app/(public)/welcome/_components/landing-section';

const QUESTIONS = [
  'catalog',
  'noMatch',
  'autoReply',
  'changes',
  'invoicing',
  'branches',
  'customerAccount',
] as const;

export async function LandingFaq() {
  const t = await getTranslations('landing.faq');

  return (
    <LandingSection id="faq" eyebrow={t('eyebrow')} title={t('title')}>
      <ul className="flex flex-col max-w-3xl border-y border-border divide-y divide-border">
        {QUESTIONS.map((question) => (
          <li key={question}>
            <Collapsible>
              <CollapsibleTrigger className="group/faq flex w-full items-center justify-between py-4 gap-x-4 rounded-md outline-none text-left text-heading-6 text-foreground transition-colors duration-200 ease-out-soft hover:text-primary focus-visible:text-primary">
                {t(`items.${question}.question`)}
                {/* The trigger owns the open state, so the rotation is driven off its data-state. */}
                <DropdownChevron className="group-data-[state=open]/faq:rotate-180" />
              </CollapsibleTrigger>
              <CollapsibleContent>
                <p className="pb-4 text-paragraph-sm text-foreground-muted">
                  {t(`items.${question}.answer`)}
                </p>
              </CollapsibleContent>
            </Collapsible>
          </li>
        ))}
      </ul>
    </LandingSection>
  );
}
