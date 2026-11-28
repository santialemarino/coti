'use client';

import { useEffect, useRef, useState } from 'react';
import { CircleCheckIcon, LinkIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { useForm } from 'react-hook-form';

import {
  Button,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  Form,
  FormRootMessage,
  InlineLink,
  PendingButton,
  StatusScreen,
} from '@repo/ui/components';
import { confirmContactVerification } from '@/app/profile/actions';
import { PortalLink as Link } from '@/components/portal-link';
import { ROUTES } from '@/config/routes';

export type EmailConfirmationState = 'pending' | 'verified' | 'invalid';

interface ContactEmailConfirmationProps {
  challenge: string;
  token: string;
  email?: string;
  next?: string;
  supplierName: string;
  initialState: EmailConfirmationState;
}

const QUOTE_DESTINATION = /^\/quotes\//;

export function ContactEmailConfirmation({
  challenge,
  token,
  email,
  next,
  supplierName,
  initialState,
}: ContactEmailConfirmationProps) {
  const t = useTranslations('customerVerification');
  const form = useForm();
  const [state, setState] = useState(initialState);
  const [destination, setDestination] = useState(next);
  const continueRef = useRef<HTMLAnchorElement>(null);
  const forQuote = QUOTE_DESTINATION.test(destination ?? '');
  const continueHref = destination || ROUTES.orders;

  // The confirm button leaves with the swap, so focus moves to the next step instead of the page.
  useEffect(() => {
    if (state === 'verified' && initialState !== 'verified') continueRef.current?.focus();
  }, [state, initialState]);

  async function confirm() {
    const result = await confirmContactVerification(challenge, token);
    if (result.error) {
      form.setError('root', { message: result.error });
      return;
    }
    if (result.next) setDestination(result.next);
    setState('verified');
  }

  if (state === 'invalid')
    return (
      <Card>
        <h1 className="sr-only">{t('invalid.title')}</h1>
        <StatusScreen
          icon={LinkIcon}
          tone="warning"
          title={t('invalid.title')}
          description={t('invalid.description')}
        >
          <Button asChild size="lg">
            <Link href={`${ROUTES.verifyContact}?${new URLSearchParams(next ? { next } : {})}`}>
              {t('invalid.action')}
            </Link>
          </Button>
        </StatusScreen>
      </Card>
    );

  if (state === 'verified')
    return (
      <Card key="verified">
        <h1 className="sr-only">{t('verified.title')}</h1>
        <StatusScreen
          icon={CircleCheckIcon}
          tone="success"
          title={t('verified.title')}
          description={t('verified.description', { email: email ?? '', supplier: supplierName })}
        >
          <Button asChild size="lg">
            <Link ref={continueRef} href={continueHref}>
              {t(forQuote ? 'verified.quote' : 'verified.orders')}
            </Link>
          </Button>
        </StatusScreen>
      </Card>
    );

  return (
    <Card>
      <CardHeader className="text-center">
        <CardTitle>
          <h1>{t('confirm.title')}</h1>
        </CardTitle>
        <CardDescription className="[overflow-wrap:anywhere]">
          {t(forQuote ? 'confirm.descriptionQuote' : 'confirm.description', {
            email: email ?? '',
            supplier: supplierName,
          })}
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-y-5">
        <Form {...form}>
          <form noValidate onSubmit={form.handleSubmit(confirm)} className="flex flex-col gap-y-5">
            <FormRootMessage />
            <PendingButton
              type="submit"
              size="lg"
              pending={form.formState.isSubmitting}
              pendingLabel={t('confirm.pending')}
            >
              {t('confirm.submit')}
            </PendingButton>
          </form>
        </Form>
        <InlineLink asChild tone="muted" className="self-center">
          <Link href={ROUTES.profile}>{t('confirm.profile')}</Link>
        </InlineLink>
      </CardContent>
    </Card>
  );
}
