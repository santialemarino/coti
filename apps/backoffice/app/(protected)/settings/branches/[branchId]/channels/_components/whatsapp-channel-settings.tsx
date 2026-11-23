'use client';

import { useState } from 'react';
import { zodResolver } from '@hookform/resolvers/zod';
import { useTranslations } from 'next-intl';
import { useForm } from 'react-hook-form';
import { toast } from 'sonner';

import {
  Badge,
  Button,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
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
  Switch,
} from '@repo/ui/components';
import { saveWhatsAppChannel } from '@/app/(protected)/settings/branches/[branchId]/channels/actions';
import {
  CHANNEL_SECRET_MAX_LENGTH,
  whatsappChannelSchema,
  type WhatsAppChannelValues,
} from '@/app/(protected)/settings/branches/[branchId]/channels/form-schema';
import { PasswordField } from '@/components/password-field';
import { PhoneInput } from '@/components/phone-input';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import type { BranchChannel } from '@/lib/api/branch-channels';
import { TEXT_FIELD_MAX_LENGTH } from '@/lib/constants/forms';
import { FORM_VALIDATION } from '@/lib/forms/options';

interface WhatsAppChannelSettingsProps {
  branchName: string;
  channel: BranchChannel;
}

export function WhatsAppChannelSettings({ branchName, channel }: WhatsAppChannelSettingsProps) {
  const t = useTranslations('branchChannels');
  const tErrors = useTranslations('common.form.errors');
  const phoneCopy = useTranslations('common.phone');
  const message = useApiErrorMessage('branchChannels');
  const [open, setOpen] = useState(false);
  const form = useForm<WhatsAppChannelValues>({
    ...FORM_VALIDATION,
    resolver: zodResolver(
      whatsappChannelSchema(
        { field: t, shared: tErrors },
        channel.isConfigured,
        channel.identifier,
      ),
    ),
    defaultValues: {
      identifier: channel.identifier ?? '',
      phoneNumberId: '',
      businessAccountId: '',
      accessToken: '',
      webhookVerifyToken: '',
      isActive: channel.isActive,
    },
  });
  const pending = form.formState.isSubmitting;
  const state = !channel.isActive ? 'inactive' : channel.isConfigured ? 'configured' : 'missing';

  function onOpen() {
    form.reset({
      identifier: channel.identifier ?? '',
      phoneNumberId: '',
      businessAccountId: '',
      accessToken: '',
      webhookVerifyToken: '',
      isActive: channel.isActive,
    });
    setOpen(true);
  }

  async function onSubmit(values: WhatsAppChannelValues) {
    let result;
    try {
      result = await saveWhatsAppChannel(channel.branchId, channel.id, values);
    } catch {
      form.setError('root', { message: message('UNREACHABLE') });
      return;
    }
    if (!result.ok) {
      form.setError('root', { message: message(result.error) });
      return;
    }
    toast.success(t('saved', { name: branchName }));
    setOpen(false);
  }

  return (
    <Card>
      <CardHeader>
        <div className="flex flex-wrap items-center justify-between gap-x-3 gap-y-2">
          <CardTitle>
            <h2>{t('whatsapp')}</h2>
          </CardTitle>
          <Badge
            tone={state === 'configured' ? 'success' : state === 'missing' ? 'warning' : 'neutral'}
            size="sm"
            dot
          >
            {t(`status.${state}`)}
          </Badge>
        </div>
        <CardDescription>{t('whatsappDescription')}</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col items-start gap-y-4">
        <p className="text-paragraph-sm-medium text-foreground wrap-anywhere">
          {channel.identifier ?? t('noNumber')}
        </p>
        <p className="text-paragraph-sm text-foreground-muted">{t('statusHint')}</p>
        <Button variant="outline" onClick={onOpen}>
          {t(channel.isConfigured ? 'update' : 'configure')}
        </Button>
      </CardContent>
      <Dialog open={open} onOpenChange={(next) => !pending && setOpen(next)}>
        <DialogContent
          className="sm:max-w-xl"
          closeOnClickOutside={!pending && !form.formState.isDirty}
        >
          <DialogHeader>
            <DialogTitle>{t('formTitle', { name: branchName })}</DialogTitle>
            <DialogDescription>
              {t(channel.isConfigured ? 'editDescription' : 'formDescription')}
            </DialogDescription>
          </DialogHeader>
          <p className="text-paragraph-sm text-foreground-muted">{t('productionHint')}</p>
          <Form {...form}>
            <form
              onSubmit={form.handleSubmit(onSubmit)}
              noValidate
              className="flex flex-col gap-y-5"
            >
              <FormField
                control={form.control}
                name="identifier"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel required>{t('number.label')}</FormLabel>
                    <FormControl>
                      <PhoneInput
                        {...field}
                        disabled={pending}
                        placeholder={t('number.placeholder')}
                        countryLabel={phoneCopy('country')}
                        countrySearchPlaceholder={phoneCopy('countrySearch')}
                        countryEmptyLabel={phoneCopy('countryEmpty')}
                        landlineNotice={{
                          message: phoneCopy('landline'),
                          action: (mobile) => phoneCopy('useMobile', { number: mobile }),
                        }}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              {(['phoneNumberId', 'businessAccountId'] as const).map((name) => (
                <FormField
                  key={name}
                  control={form.control}
                  name={name}
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel required={!channel.isConfigured}>{t(`${name}.label`)}</FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          disabled={pending}
                          inputMode="numeric"
                          maxLength={TEXT_FIELD_MAX_LENGTH}
                          placeholder={t(
                            channel.isConfigured ? 'keepSetting' : `${name}.placeholder`,
                          )}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              ))}
              {(['accessToken', 'webhookVerifyToken'] as const).map((name) => (
                <PasswordField
                  key={name}
                  control={form.control}
                  name={name}
                  existing
                  maxLength={CHANNEL_SECRET_MAX_LENGTH}
                  autoComplete="off"
                  disabled={pending}
                  required={!channel.isConfigured}
                  label={t(`${name}.label`)}
                  placeholder={t(channel.isConfigured ? 'keepSetting' : `${name}.placeholder`)}
                />
              ))}
              <p className="text-paragraph-xs text-foreground-muted">{t('webhookHint')}</p>
              <FormField
                control={form.control}
                name="isActive"
                render={({ field }) => (
                  <FormItem className="flex items-center justify-between gap-x-4">
                    <FormLabel>{t('active')}</FormLabel>
                    <FormControl>
                      <Switch
                        checked={field.value}
                        onCheckedChange={field.onChange}
                        disabled={pending}
                      />
                    </FormControl>
                  </FormItem>
                )}
              />
              <FormRootMessage />
              <DialogFooter>
                <Button variant="outline" disabled={pending} onClick={() => setOpen(false)}>
                  {t('cancel')}
                </Button>
                <PendingButton type="submit" pending={pending} pendingLabel={t('saving')}>
                  {t('save')}
                </PendingButton>
              </DialogFooter>
            </form>
          </Form>
        </DialogContent>
      </Dialog>
    </Card>
  );
}
