'use client';

import { usePathname, useRouter } from 'next/navigation';
import { useTranslations } from 'next-intl';

import { ToggleGroup, ToggleGroupItem } from '@repo/ui/components';
import { ADMINISTRATION_VIEWS, type AdministrationView } from '@/lib/constants/administration';

interface AdministrationViewSwitchProps {
  view: AdministrationView;
}

// Switches between the invoice history and the sales awaiting an invoice; each starts on page one.
export function AdministrationViewSwitch({ view }: AdministrationViewSwitchProps) {
  const router = useRouter();
  const pathname = usePathname();
  const t = useTranslations('invoicing.views');

  return (
    <ToggleGroup
      type="single"
      value={view}
      onValueChange={(next) => {
        if (!next || next === view) return;
        router.push(next === 'history' ? pathname : `${pathname}?view=${next}`);
      }}
      aria-label={t('label')}
    >
      {ADMINISTRATION_VIEWS.map((option) => (
        <ToggleGroupItem key={option} value={option}>
          {t(option)}
        </ToggleGroupItem>
      ))}
    </ToggleGroup>
  );
}
