'use client';

import { useState } from 'react';
import { useRouter } from 'next/navigation';
import { PlusIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';

import { Button } from '@repo/ui/components';
import { CreateRfqDialog } from '@/app/(protected)/rfqs/_components/create-rfq-dialog';
import { useRfqList } from '@/app/(protected)/rfqs/_components/rfq-list-context';

interface RfqCreateButtonProps {
  size?: 'default' | 'sm';
  className?: string;
}

// "Crear pedido" with the dialog it opens, for every screen the queue offers it on.
export function RfqCreateButton({ size = 'default', className }: RfqCreateButtonProps) {
  const router = useRouter();
  const t = useTranslations('rfqs');
  const { activeBranchId } = useRfqList();
  const [open, setOpen] = useState(false);

  return (
    <>
      <Button size={size} onClick={() => setOpen(true)} className={className}>
        <PlusIcon aria-hidden="true" />
        {t('list.create')}
      </Button>
      <CreateRfqDialog
        open={open}
        onOpenChange={setOpen}
        onCreated={() => router.refresh()}
        activeBranchId={activeBranchId}
      />
    </>
  );
}
