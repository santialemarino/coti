'use client';

import { useEffect, useState } from 'react';
import { useRouter } from 'next/navigation';
import { zodResolver } from '@hookform/resolvers/zod';
import { useTranslations } from 'next-intl';
import { useForm } from 'react-hook-form';
import { toast } from 'sonner';
import { z } from 'zod';

import {
  Badge,
  Button,
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
  FormRootMessage,
  Input,
  PendingButton,
} from '@repo/ui/components';
import {
  confirmContactVerification,
  readContactVerification,
  requestContactVerification,
} from '@/app/profile/actions';
import { usePortalPath } from '@/components/portal-link';
import type { WhatsAppProof } from '@/lib/api/customer-profile';
import type { ContactKind } from '@/lib/api/customer-profile-model';

/* How often a screen waiting on the customer's WhatsApp message asks whether it arrived. */
const PROOF_POLL_MS = 4000;

type PendingProof = WhatsAppProof & { id: string };

interface ContactVerificationProps {
  kind: ContactKind;
  value: string;
  verified: boolean;
  /* The portal page the proof returns to once confirmed. */
  next?: string;
  /* A code already sent for this contact, whose entry opens at once. */
  initialChallenge?: string;
  /* A WhatsApp proof already started for this customer, whose message opens at once. */
  initialProof?: PendingProof;
}

export function ContactVerification({
  kind,
  value,
  verified,
  next,
  initialChallenge,
  initialProof,
}: ContactVerificationProps) {
  const t = useTranslations('customerProfile');
  const router = useRouter();
  const portalPath = usePortalPath();
  const destination = next ? portalPath(next) : '';
  const [sending, setSending] = useState(false);
  const [challenge, setChallenge] = useState<string | null>(initialChallenge ?? null);
  const [proof, setProof] = useState<PendingProof | null>(initialProof ?? null);
  const form = useForm({
    resolver: zodResolver(z.object({ code: z.string().regex(/^\d{6}$/, t('codeInvalid')) })),
    defaultValues: { code: '' },
  });

  useEffect(() => {
    if (!proof) return;
    const timer = window.setInterval(async () => {
      const status = await readContactVerification(proof.id);
      if (status === 'VERIFIED') {
        setProof(null);
        toast.success(t('verified'));
        if (destination) router.push(destination);
        else router.refresh();
      } else if (status === 'EXPIRED') {
        setProof(null);
        toast.error(t('whatsappProof.expired'));
      }
    }, PROOF_POLL_MS);
    return () => window.clearInterval(timer);
  }, [proof, destination, router, t]);

  async function request() {
    setSending(true);
    try {
      const result = await requestContactVerification(kind, value, next);
      if (result.error) {
        toast.error(result.error);
        return;
      }
      if (kind === 'EMAIL') {
        toast.success(t('verificationEmailSent'));
      } else if (result.id && result.whatsappProof) {
        setProof({ id: result.id, ...result.whatsappProof });
      } else if (result.id) {
        form.reset({ code: '' });
        setChallenge(result.id);
      }
    } catch {
      toast.error(t('errors.verificationUnavailable'));
    } finally {
      setSending(false);
    }
  }
  async function confirm(values: { code: string }) {
    if (!challenge) return;
    try {
      const result = await confirmContactVerification(challenge, values.code);
      if (result.error) {
        form.setError('root', { message: result.error });
        return;
      }
      setChallenge(null);
      toast.success(t('verified'));
      router.refresh();
    } catch {
      form.setError('root', { message: t('errors.verificationUnavailable') });
    }
  }
  return (
    <>
      <div className="flex items-center gap-x-2">
        <Badge tone={verified ? 'success' : 'warning'}>
          {t(verified ? 'verified' : 'verificationPending')}
        </Badge>
        {!verified ? (
          <PendingButton
            variant="ghost"
            size="sm"
            pending={sending}
            pendingLabel={t('verificationSending')}
            onClick={request}
          >
            {t('verify')}
          </PendingButton>
        ) : null}
      </div>
      <Dialog
        open={proof !== null}
        onOpenChange={(open) => {
          if (!open) setProof(null);
        }}
      >
        <DialogContent aria-describedby={undefined}>
          <DialogHeader>
            <DialogTitle>{t('whatsappProof.title')}</DialogTitle>
          </DialogHeader>
          {proof ? (
            <div className="flex flex-col gap-y-5">
              <p className="text-paragraph-sm text-foreground-muted">
                {t('whatsappProof.instructions')}
              </p>
              <p className="px-4 py-3 bg-muted rounded-lg text-center text-heading-sm select-all">
                VERIFICAR {proof.code}
              </p>
              <p className="text-paragraph-sm text-foreground-muted" role="status">
                {t('whatsappProof.waiting')}
              </p>
              <div className="flex justify-end gap-x-2">
                <Button variant="outline" onClick={() => setProof(null)}>
                  {t('cancel')}
                </Button>
                <Button asChild>
                  <a href={proof.link} target="_blank" rel="noopener noreferrer">
                    {t('whatsappProof.open')}
                  </a>
                </Button>
              </div>
            </div>
          ) : null}
        </DialogContent>
      </Dialog>
      <Dialog
        open={challenge !== null}
        onOpenChange={(open) => {
          if (!open) setChallenge(null);
        }}
      >
        <DialogContent aria-describedby={undefined}>
          <DialogHeader>
            <DialogTitle>{t('verifyPhone')}</DialogTitle>
          </DialogHeader>
          <Form {...form}>
            <form
              noValidate
              onSubmit={(event) => {
                event.stopPropagation();
                void form.handleSubmit(confirm)(event);
              }}
              className="flex flex-col gap-y-5"
            >
              <FormField
                control={form.control}
                name="code"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel required>{t('code')}</FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        inputMode="numeric"
                        autoComplete="one-time-code"
                        maxLength={6}
                        autoFocus
                        disabled={form.formState.isSubmitting}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormRootMessage />
              <div className="flex justify-end gap-x-2">
                <Button variant="outline" onClick={() => setChallenge(null)}>
                  {t('cancel')}
                </Button>
                <PendingButton
                  type="submit"
                  pending={form.formState.isSubmitting}
                  pendingLabel={t('verifying')}
                >
                  {t('confirmContact')}
                </PendingButton>
              </div>
            </form>
          </Form>
        </DialogContent>
      </Dialog>
    </>
  );
}
