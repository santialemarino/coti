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
  isAdmin: boolean;
}

export function BranchSwitcher({ branches, activeBranchId, isAdmin }: BranchSwitcherProps) {
  const t = useTranslations('common.branch');
  const [pending, startTransition] = useTransition();

  // "Todas" is account-wide, an admin's reach alone; a seller never sees an option the API
  // reads as something wider than their assignments.
  const options = [
    ...(isAdmin
      ? [{ value: ALL_BRANCHES, label: t('all'), icon: <Building2Icon aria-hidden="true" /> }]
      : []),
    ...branches.map((branch) => ({
      value: branch.id,
      label: branch.name,
      icon: <StoreIcon aria-hidden="true" />,
    })),
  ];

  // A seller on a single branch has nowhere to switch to, so the control reads as context
  // rather than a menu: locked, but naming the branch they are working in.
  const locked = !isAdmin && branches.length <= 1;

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
      disabled={locked || pending}
      aria-label={t('label')}
      className="w-44 sm:w-56"
    />
  );
}
