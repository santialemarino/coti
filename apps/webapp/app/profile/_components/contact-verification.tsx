'use client';

import { useState } from 'react';
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
import { confirmContactVerification, requestContactVerification } from '@/app/profile/actions';
import type { ContactKind } from '@/lib/api/customer-profile-model';

interface ContactVerificationProps {
  kind: ContactKind;
  value: string;
  verified: boolean;
}

export function ContactVerification({ kind, value, verified }: ContactVerificationProps) {
  const t = useTranslations('customerProfile');
  const router = useRouter();
  const [sending, setSending] = useState(false);
  const [challenge, setChallenge] = useState<string | null>(null);
  const form = useForm({
    resolver: zodResolver(z.object({ code: z.string().regex(/^\d{6}$/, t('codeInvalid')) })),
    defaultValues: { code: '' },
  });

  async function request() {
    setSending(true);
    try {
      const result = await requestContactVerification(kind, value);
      if (result.error) {
        toast.error(result.error);
        return;
      }
      if (kind === 'EMAIL') {
        toast.success(t('verificationEmailSent'));
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
