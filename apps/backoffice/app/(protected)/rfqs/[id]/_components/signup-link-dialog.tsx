'use client';

import { useTranslations } from 'next-intl';
import { toast } from 'sonner';

import {
  Button,
  CopyButton,
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@repo/ui/components';

interface SignupLinkDialogProps {
  // The supplier's customer sign-up page.
  url: string;
}

/*
 * «Enviar link de registro»: a client's CUIT only enters Coti when they create their account, so the
 * seller hands them the sign-up page. The seller sends it through their own channel; Coti does not.
 */
export function SignupLinkDialog({ url }: SignupLinkDialogProps) {
  const t = useTranslations('invoicing.card.signup');

  return (
    <Dialog>
      <DialogTrigger asChild>
        <Button variant="outline" size="sm">
          {t('action')}
        </Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('title')}</DialogTitle>
          <DialogDescription>{t('description')}</DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-y-1">
          <span className="text-paragraph-xs-medium text-foreground-muted">{t('linkLabel')}</span>
          <span className="break-all text-paragraph-sm text-foreground">{url}</span>
        </div>
        <DialogFooter>
          <DialogClose asChild>
            <Button variant="outline">{t('close')}</Button>
          </DialogClose>
          <CopyButton
            variant="default"
            size="default"
            value={url}
            labels={{ copy: t('copy'), copied: t('copied') }}
            onCopyError={() => toast.error(t('copyFailed'))}
          />
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
