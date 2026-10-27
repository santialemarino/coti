'use client';

import { useRouter } from 'next/navigation';
import { zodResolver } from '@hookform/resolvers/zod';
import { useTranslations } from 'next-intl';
import { useForm } from 'react-hook-form';
import { toast } from 'sonner';
import { z } from 'zod';

import {
  Badge,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
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
import { saveRecoveryEmail } from '@/app/profile/actions';
import type { CustomerRecoveryEmail } from '@/lib/api/customer-recovery-email';

interface RecoveryEmailFormProps {
  recovery: CustomerRecoveryEmail;
}

export function RecoveryEmailForm({ recovery }: RecoveryEmailFormProps) {
  const t = useTranslations('customerProfile.recovery');
  const router = useRouter();
  const schema = z.object({
    email: z.union([z.literal(''), z.string().trim().email(t('invalid')).max(255, t('invalid'))]),
  });
  const form = useForm({
    resolver: zodResolver(schema),
    defaultValues: { email: recovery.email ?? '' },
  });
  const unchanged = form.watch('email').trim() === (recovery.email ?? '');

  async function submit(values: { email: string }) {
    try {
      const result = await saveRecoveryEmail(values.email);
      if (result.error) {
        form.setError('root', { message: result.error });
        return;
      }
      toast.success(t(values.email ? 'sent' : 'removed'));
      router.refresh();
    } catch {
      form.setError('root', { message: t('unavailable') });
    }
  }
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('title')}</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-y-4">
        <p className="text-paragraph-sm text-foreground-muted">{t('description')}</p>
        {recovery.email ? (
          <Badge tone={recovery.verified ? 'success' : 'warning'}>
            {t(recovery.verified ? 'verified' : 'pending')}
          </Badge>
        ) : (
          <p className="text-paragraph-sm text-foreground-muted">{t('missing')}</p>
        )}
        <Form {...form}>
          <form noValidate onSubmit={form.handleSubmit(submit)} className="flex flex-col gap-y-4">
            <FormField
              control={form.control}
              name="email"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('email')}</FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      type="email"
                      autoComplete="email"
                      maxLength={255}
                      disabled={form.formState.isSubmitting}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormRootMessage />
            <div className="flex justify-end">
              <PendingButton
                type="submit"
                pending={form.formState.isSubmitting}
                pendingLabel={t('saving')}
                disabled={unchanged && (recovery.verified || !recovery.email)}
              >
                {t(unchanged && recovery.email && !recovery.verified ? 'resend' : 'save')}
              </PendingButton>
            </div>
          </form>
        </Form>
      </CardContent>
    </Card>
  );
}
