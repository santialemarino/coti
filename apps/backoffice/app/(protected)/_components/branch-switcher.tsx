'use client';

import { useTransition } from 'react';
import { Building2Icon, StoreIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';

import { Combobox } from '@repo/ui/components';
import { selectBranch } from '@/app/(protected)/actions';
import type { Branch } from '@/lib/api/branches';
import { ALL_BRANCHES } from '@/lib/constants/branch';

// From here a list stops being scannable and the search box earns the click it costs.
const SEARCHABLE_FROM = 8;

interface BranchSwitcherProps {
  branches: Branch[];
  activeBranchId: string | null;
}

export function BranchSwitcher({ branches, activeBranchId }: BranchSwitcherProps) {
  const t = useTranslations('common.branch');
  const [pending, startTransition] = useTransition();

  const options = [
    { value: ALL_BRANCHES, label: t('all'), icon: <Building2Icon aria-hidden="true" /> },
    ...branches.map((branch) => ({
      value: branch.id,
      label: branch.name,
      icon: <StoreIcon aria-hidden="true" />,
    })),
  ];

  function onValueChange(value: string) {
    startTransition(async () => {
      await selectBranch(value);
    });
  }

  return (
    <Combobox
      options={options}
      value={activeBranchId ?? ALL_BRANCHES}
      onValueChange={onValueChange}
      placeholder={t('placeholder')}
      searchable={branches.length >= SEARCHABLE_FROM}
      searchPlaceholder={t('search')}
      emptyLabel={t('empty')}
      disabled={pending}
      aria-label={t('label')}
      className="w-44 sm:w-56"
    />
  );
}
