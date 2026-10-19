'use client';

import { useState } from 'react';
import Image from 'next/image';
import Link from 'next/link';
import { zodResolver } from '@hookform/resolvers/zod';
import { Building2Icon, CheckIcon, MailIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { useForm } from 'react-hook-form';

import {
  Button,
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
  InlineLink,
  Input,
  PendingButton,
  StatusScreen,
  TaxIdInput,
} from '@repo/ui/components';
import {
  completeCustomerGoogle,
  recoverCustomerPassword,
  resetCustomerPassword,
  signInCustomer,
  signUpCustomer,
  verifyCustomerEmail,
} from '@/app/login/actions';
import { customerAuthSchema, type CustomerAuthMode } from '@/components/customer-auth-schema';
import { ROUTES } from '@/config/routes';
import { FORM_VALIDATION } from '@/lib/forms/options';

interface CustomerAuthScreenProps {
  mode: CustomerAuthMode;
  supplierName: string;
  logoUrl?: string;
  next: string;
  minimum: number;
  token?: string;
  taxId?: string;
  initialError?: string;
  googleEnabled: boolean;
}

export function CustomerAuthScreen({
  mode,
  supplierName,
  logoUrl,
  next,
  minimum,
  token = '',
  taxId = '',
  initialError,
  googleEnabled,
}: CustomerAuthScreenProps) {
  const t = useTranslations('customerAuth');
  const common = useTranslations('common');
  const [received, setReceived] = useState(false);
  const form = useForm({
    ...FORM_VALIDATION,
    resolver: zodResolver(
      customerAuthSchema(mode, (key, values) => t(`validation.${key}`, values), minimum),
    ),
    defaultValues: { taxId, password: '', confirmation: '', email: '' },
  });
  const fields: ('taxId' | 'email' | 'password' | 'confirmation')[] = [];
  if (['login', 'signup', 'forgot', 'google', 'linkGoogle'].includes(mode)) fields.push('taxId');
  if (['signup', 'forgot'].includes(mode)) fields.push('email');
  if (['login', 'signup', 'reset'].includes(mode)) fields.push('password');
  if (['signup', 'reset'].includes(mode)) fields.push('confirmation');
  const loginUrl = `${ROUTES.login}?method=password&next=${encodeURIComponent(next)}`;
  const signupUrl = `${ROUTES.signup}?next=${encodeURIComponent(next)}`;

  async function onSubmit(values: {
    taxId: string;
    password: string;
    confirmation: string;
    email: string;
  }) {
    const result =
      mode === 'login'
        ? await signInCustomer({ ...values, next })
        : mode === 'signup'
          ? await signUpCustomer({ ...values, next })
          : mode === 'forgot'
            ? await recoverCustomerPassword(values)
            : mode === 'reset'
              ? await resetCustomerPassword({ token, password: values.password })
              : mode === 'verify'
                ? await verifyCustomerEmail(token)
                : await completeCustomerGoogle({
                    taxId: values.taxId,
                    next,
                    link: mode === 'linkGoogle',
                  });
    if (result.error) form.setError('root', { message: t(`errors.${result.error}`) });
    if (result.received) setReceived(true);
  }

  return (
    <main className="flex min-h-dvh flex-col items-center justify-center px-4 py-10 gap-y-8 sm:px-6">
      <div className="flex w-full max-w-sm flex-col gap-y-7">
        <header className="flex items-center justify-center gap-x-3">
          {logoUrl ? (
            <Image
              src={logoUrl}
              alt=""
              width={40}
              height={40}
              unoptimized
              className="size-10 rounded-lg object-contain"
            />
          ) : (
            <span className="flex size-10 items-center justify-center bg-accent rounded-lg text-primary">
              <Building2Icon className="size-5" aria-hidden="true" />
            </span>
          )}
          <p className="break-words text-heading-5">{supplierName}</p>
        </header>
        {received ? (
          <StatusScreen
            icon={mode === 'forgot' ? MailIcon : CheckIcon}
            tone="success"
            title={t(`success.${mode}`)}
            description={mode === 'forgot' ? t('recoverySent') : undefined}
          >
            <Button asChild>
              <Link href={ROUTES.login}>{t('backToLogin')}</Link>
            </Button>
          </StatusScreen>
        ) : (
          <Card>
            <CardHeader>
              <CardTitle>
                <h1>{t(`titles.${mode}`)}</h1>
              </CardTitle>
            </CardHeader>
            <CardContent className="flex flex-col gap-y-5">
              {mode === 'welcome' ? (
                <>
                  {googleEnabled ? (
                    <Button asChild variant="outline" size="lg">
                      <Link href={`${ROUTES.googleStart}?next=${encodeURIComponent(next)}`}>
                        <Image
                          src="https://developers.google.com/static/identity/images/g-logo.png"
                          width={20}
                          height={20}
                          alt=""
                          unoptimized
                        />
                        {t('google')}
                      </Link>
                    </Button>
                  ) : (
                    <Button variant="outline" size="lg" disabled>
                      {t('google')}
                    </Button>
                  )}
                  {!googleEnabled ? (
                    <p role="status" className="text-paragraph-sm text-foreground-muted">
                      {t('errors.googleUnavailable')}
                    </p>
                  ) : null}
                  {initialError ? (
                    <p role="alert" className="text-paragraph-sm text-danger-foreground">
                      {t(`errors.${initialError}`)}
                    </p>
                  ) : null}
                  <Button asChild variant="ghost">
                    <Link href={loginUrl}>{t('passwordLogin')}</Link>
                  </Button>
                  <InlineLink asChild className="self-center">
                    <Link href={signupUrl}>{t('createAccount')}</Link>
                  </InlineLink>
                </>
              ) : (
                <>
                  <Form {...form}>
                    <form
                      onSubmit={form.handleSubmit(onSubmit)}
                      noValidate
                      className="flex flex-col gap-y-5"
                    >
                      {fields.map((name) => (
                        <FormField
                          key={name}
                          control={form.control}
                          name={name}
                          render={({ field }) => (
                            <FormItem>
                              <FormLabel required={!(name === 'email' && mode === 'signup')}>
                                {t(
                                  `fields.${name}${name === 'email' && mode === 'signup' ? 'Optional' : ''}`,
                                )}
                              </FormLabel>
                              <FormControl>
                                {name === 'taxId' ? (
                                  <TaxIdInput
                                    kind="cuit"
                                    {...field}
                                    readOnly={mode === 'linkGoogle'}
                                    autoComplete="username"
                                  />
                                ) : (
                                  <Input
                                    {...field}
                                    type={name === 'email' ? 'email' : 'password'}
                                    maxLength={name === 'email' ? 255 : 72}
                                    autoComplete={
                                      name === 'email'
                                        ? 'email'
                                        : mode === 'login'
                                          ? 'current-password'
                                          : 'new-password'
                                    }
                                    passwordToggleLabel={common('form.togglePassword')}
                                  />
                                )}
                              </FormControl>
                              <FormMessage />
                            </FormItem>
                          )}
                        />
                      ))}
                      <FormRootMessage />
                      <PendingButton
                        type="submit"
                        pending={form.formState.isSubmitting}
                        pendingLabel={t('pending')}
                      >
                        {t(`submit.${mode}`)}
                      </PendingButton>
                    </form>
                  </Form>
                  {mode === 'login' ? (
                    <InlineLink asChild className="self-center">
                      <Link href={ROUTES.forgotPassword}>{t('forgot')}</Link>
                    </InlineLink>
                  ) : null}
                  {mode === 'google' ? (
                    <InlineLink asChild>
                      <Link
                        href={`${ROUTES.login}?method=password&next=${encodeURIComponent(`${ROUTES.googleComplete}?next=${encodeURIComponent(next)}`)}`}
                      >
                        {t('existingAccount')}
                      </Link>
                    </InlineLink>
                  ) : null}
                  <InlineLink asChild className="self-center">
                    <Link href={`${ROUTES.login}?next=${encodeURIComponent(next)}`}>
                      {t('backToLogin')}
                    </Link>
                  </InlineLink>
                </>
              )}
            </CardContent>
          </Card>
        )}
      </div>
    </main>
  );
}
