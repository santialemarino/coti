'use client';

import { useEffect, useState } from 'react';
import { ClipboardListIcon, UploadIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@repo/ui/components';
import { cn } from '@repo/ui/lib';
import { RfqImportView } from '@/app/(protected)/rfqs/_components/rfq-import-view';
import { RfqManualView } from '@/app/(protected)/rfqs/_components/rfq-manual-view';

interface CreateRfqDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

type Step = 'choose' | 'import' | 'manual';

const OPTION_CLASSES =
  'flex flex-col items-start gap-y-3 rounded-1.5xl border border-border bg-card p-5 text-left shadow-e2 outline-none transition-[border-color,box-shadow,background-color] duration-200 ease-out-soft hover:border-border-strong hover:bg-muted focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/45';

/*
 * The "crear pedido" flow: first pick how the order is built (AI import or manual catalog pick),
 * then that step's form runs inside the same dialog. The RFQ domain has no backend endpoints yet,
 * so both steps simulate their latency the way the RFQ list mocks its data.
 */
export function CreateRfqDialog({ open, onOpenChange }: CreateRfqDialogProps) {
  const t = useTranslations('rfqs.create');
  const [step, setStep] = useState<Step>('choose');

  /* A fresh dialog always starts at the choice, whatever step closed it last. */
  useEffect(() => {
    if (open) setStep('choose');
  }, [open]);

  const copy = {
    choose: { title: t('title'), description: t('description') },
    import: { title: t('importOption.title'), description: t('importOption.description') },
    manual: { title: t('manualOption.title'), description: t('manualOption.description') },
  }[step];

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className={cn('sm:max-w-lg', step === 'manual' && 'sm:max-w-3xl')}
        closeOnClickOutside={step === 'choose'}
      >
        <DialogHeader>
          <DialogTitle>{copy.title}</DialogTitle>
          <DialogDescription>{copy.description}</DialogDescription>
        </DialogHeader>

        {step === 'choose' ? (
          <div className="grid gap-3 sm:grid-cols-2">
            <button type="button" className={OPTION_CLASSES} onClick={() => setStep('import')}>
              <span className="flex size-10 items-center justify-center rounded-lg bg-accent text-accent-foreground">
                <UploadIcon aria-hidden="true" className="size-5" />
              </span>
              <span className="text-heading-6 text-foreground">{t('importOption.title')}</span>
              <span className="text-paragraph-sm text-foreground-muted">
                {t('importOption.description')}
              </span>
            </button>
            <button type="button" className={OPTION_CLASSES} onClick={() => setStep('manual')}>
              <span className="flex size-10 items-center justify-center rounded-lg bg-accent text-accent-foreground">
                <ClipboardListIcon aria-hidden="true" className="size-5" />
              </span>
              <span className="text-heading-6 text-foreground">{t('manualOption.title')}</span>
              <span className="text-paragraph-sm text-foreground-muted">
                {t('manualOption.description')}
              </span>
            </button>
          </div>
        ) : null}

        {step === 'import' ? (
          <RfqImportView onBack={() => setStep('choose')} onClose={() => onOpenChange(false)} />
        ) : null}

        {step === 'manual' ? (
          <RfqManualView onBack={() => setStep('choose')} onClose={() => onOpenChange(false)} />
        ) : null}
      </DialogContent>
    </Dialog>
  );
}
