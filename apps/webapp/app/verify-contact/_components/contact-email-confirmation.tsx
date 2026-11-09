'use client';

import { useRouter } from 'next/navigation';
import { useTranslations } from 'next-intl';
import { useForm } from 'react-hook-form';

import { Button, Form, FormRootMessage, PendingButton } from '@repo/ui/components';
import { confirmContactVerification } from '@/app/profile/actions';
import { PortalLink as Link } from '@/components/portal-link';
import { ROUTES } from '@/config/routes';
import { browserPortalPath } from '@/lib/portal/routing';

export function ContactEmailConfirmation({
  challenge,
  token,
}: {
  challenge: string;
  token: string;
}) {
  const t = useTranslations('customerProfile');
  const router = useRouter();
  const form = useForm();
  async function confirm() {
    const result = await confirmContactVerification(challenge, token);
    if (result.error) {
      form.setError('root', { message: result.error });
      return;
    }
    router.replace(browserPortalPath(ROUTES.profile));
    router.refresh();
  }
  return (
    <Form {...form}>
      <form noValidate onSubmit={form.handleSubmit(confirm)} className="flex flex-col gap-y-5">
        <FormRootMessage />
        <PendingButton
          type="submit"
          pending={form.formState.isSubmitting}
          pendingLabel={t('verifying')}
        >
          {t('confirmContact')}
        </PendingButton>
        <Button asChild variant="ghost">
          <Link href={ROUTES.profile}>{t('title')}</Link>
        </Button>
      </form>
    </Form>
  );
}
