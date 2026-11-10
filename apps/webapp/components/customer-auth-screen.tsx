'use client';

import { Fragment } from 'react';
import Image from 'next/image';
import { zodResolver } from '@hookform/resolvers/zod';
import { Building2Icon } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { useForm, useWatch } from 'react-hook-form';

import {
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  Checkbox,
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
  PhoneInput,
  TaxIdInput,
} from '@repo/ui/components';
import { AccountRecovery } from '@/app/forgot-password/_components/account-recovery';
import { completeCustomerGoogle, signInCustomer, signUpCustomer } from '@/app/login/actions';
import { customerAuthSchema, type CustomerAuthMode } from '@/components/customer-auth-schema';
import { PortalLink as Link, usePortalPath } from '@/components/portal-link';
import { ROUTES } from '@/config/routes';
import { FORM_VALIDATION } from '@/lib/forms/options';
import { useFormatters } from '@/lib/i18n/formatters';

interface CustomerAuthScreenProps {
  mode: CustomerAuthMode;
  supplierName: string;
  logoUrl?: string;
  next: string;
  minimum: number;
  taxId?: string;
  googleEmail?: string;
  initialError?: string;
  googleEnabled: boolean;
}

export function CustomerAuthScreen({
  mode,
  supplierName,
  logoUrl,
  next,
  minimum,
  taxId = '',
  googleEmail,
  initialError,
  googleEnabled,
}: CustomerAuthScreenProps) {
  const t = useTranslations('customerAuth');
  const portalPath = usePortalPath();
  const common = useTranslations('common');
  const phoneLabels = useTranslations('rfq.fields.phone');
  const formatters = useFormatters();
  const form = useForm({
    ...FORM_VALIDATION,
    resolver: zodResolver(
      customerAuthSchema(mode, (key, values) => t(`validation.${key}`, values), minimum),
    ),
    defaultValues: {
      taxId,
      password: '',
      confirmation: '',
      email: '',
      name: '',
      phone: '',
      useGoogleEmail: false,
    },
  });
  const useGoogleEmail = useWatch({ control: form.control, name: 'useGoogleEmail' });
  const fields: ('name' | 'taxId' | 'email' | 'phone' | 'password' | 'confirmation')[] = [];
  if (mode === 'signup' || mode === 'google') fields.push('name');
  if (['login', 'signup', 'google', 'linkGoogle'].includes(mode)) fields.push('taxId');
  if (mode === 'signup' || mode === 'google') fields.push('email', 'phone');
  if (['login', 'signup'].includes(mode)) fields.push('password');
  if (mode === 'signup') fields.push('confirmation');
  const loginUrl = `${ROUTES.login}?method=password&next=${encodeURIComponent(next)}`;
  const signupUrl = `${ROUTES.signup}?next=${encodeURIComponent(next)}`;

  const googleLabel = (
    <>
      <Image src="/icons/google.png" width={20} height={20} alt="" className="shrink-0" />
      {t('google')}
    </>
  );

  async function onSubmit(values: {
    taxId: string;
    password: string;
    confirmation: string;
    email: string;
    name?: string;
    phone?: string;
    useGoogleEmail?: boolean;
  }) {
    const result =
      mode === 'login'
        ? await signInCustomer({ ...values, next })
        : mode === 'signup'
          ? await signUpCustomer({
              ...values,
              name: values.name || '',
              phone: values.phone || '',
              next,
            })
          : await completeCustomerGoogle({
              taxId: values.taxId,
              name: values.name,
              next,
              link: mode === 'linkGoogle',
              ...(mode === 'google'
                ? {
                    email: values.useGoogleEmail ? '' : values.email.trim(),
                    phone: values.phone || '',
                    useGoogleEmail: Boolean(googleEmail && values.useGoogleEmail),
                  }
                : {}),
            });
    if (result.error) form.setError('root', { message: t(`errors.${result.error}`) });
  }

  return (
    <main className="flex min-h-dvh flex-col items-center justify-center px-4 py-10 gap-y-8 sm:px-6 sm:py-16">
      <div className="flex w-full max-w-md flex-col gap-y-8">
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
          <div className="flex min-w-0 flex-col gap-y-1">
            <p className="break-words text-heading-5">{supplierName}</p>
            <p className="text-paragraph-xs text-foreground-muted">{t('portalLabel')}</p>
          </div>
        </header>
        {mode === 'forgot' ? (
          <AccountRecovery minimum={minimum} next={next} />
        ) : (
          <Card>
            <CardHeader className="text-center">
              <CardTitle>
                <h1>{t(`titles.${mode}`)}</h1>
              </CardTitle>
            </CardHeader>
            <CardContent className="flex flex-col gap-y-5">
              {mode === 'welcome' ? (
                <>
                  {googleEnabled ? (
                    <Button asChild variant="outline" size="lg">
                      <a
                        href={portalPath(`${ROUTES.googleStart}?next=${encodeURIComponent(next)}`)}
                      >
                        {googleLabel}
                      </a>
                    </Button>
                  ) : (
                    <Button variant="outline" size="lg" disabled>
                      {googleLabel}
                    </Button>
                  )}
                  {!googleEnabled ? (
                    <p
                      role="status"
                      className="text-center text-paragraph-sm text-foreground-muted"
                    >
                      {t('errors.googleUnavailable')}
                    </p>
                  ) : null}
                  {initialError ? (
                    <p
                      role="alert"
                      className="text-center text-paragraph-sm text-danger-foreground"
                    >
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
                        <Fragment key={name}>
                          <FormField
                            control={form.control}
                            name={name}
                            render={({ field }) => (
                              <FormItem>
                                <FormLabel
                                  required={
                                    !(
                                      ['email', 'phone'].includes(name) &&
                                      ['signup', 'google'].includes(mode)
                                    )
                                  }
                                >
                                  {t(`fields.${name}`)}
                                </FormLabel>
                                <FormControl>
                                  {name === 'phone' ? (
                                    <PhoneInput
                                      {...field}
                                      value={field.value || ''}
                                      formatters={formatters}
                                      countryLabel={phoneLabels('country')}
                                      countrySearchPlaceholder={phoneLabels('search')}
                                      countryEmptyLabel={phoneLabels('empty')}
                                      landlineNotice={{
                                        message: phoneLabels('landline'),
                                        action: (number) => phoneLabels('mobile', { number }),
                                      }}
                                    />
                                  ) : name === 'name' ? (
                                    <Input
                                      {...field}
                                      value={field.value || ''}
                                      autoComplete="name"
                                      maxLength={255}
                                    />
                                  ) : name === 'taxId' ? (
                                    <TaxIdInput
                                      kind="cuit"
                                      {...field}
                                      readOnly={mode === 'linkGoogle'}
                                      autoComplete="username"
                                    />
                                  ) : (
                                    <Input
                                      {...field}
                                      value={
                                        name === 'email' && useGoogleEmail
                                          ? googleEmail || ''
                                          : field.value
                                      }
                                      disabled={
                                        mode === 'google' && name === 'email' && useGoogleEmail
                                      }
                                      containerClassName={
                                        mode === 'google' && name === 'email' && useGoogleEmail
                                          ? 'bg-surface-hover opacity-100'
                                          : undefined
                                      }
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
                          {mode === 'google' && name === 'email' ? (
                            <div className="flex flex-col gap-y-3">
                              {googleEmail ? (
                                <FormField
                                  control={form.control}
                                  name="useGoogleEmail"
                                  render={({ field }) => (
                                    <FormItem className="flex-row items-start gap-x-2">
                                      <FormControl>
                                        <Checkbox
                                          checked={Boolean(field.value)}
                                          onCheckedChange={(checked) => {
                                            field.onChange(checked === true);
                                            if (form.formState.isSubmitted)
                                              void form.trigger('email');
                                          }}
                                          className="mt-0.5"
                                        />
                                      </FormControl>
                                      <FormLabel className="min-w-0 cursor-pointer [overflow-wrap:anywhere] text-paragraph-sm">
                                        {t('useGoogleEmail', { email: googleEmail })}
                                      </FormLabel>
                                    </FormItem>
                                  )}
                                />
                              ) : null}
                            </div>
                          ) : null}
                        </Fragment>
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
