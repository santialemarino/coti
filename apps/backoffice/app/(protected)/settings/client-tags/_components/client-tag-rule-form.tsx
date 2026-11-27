'use client';

import { zodResolver } from '@hookform/resolvers/zod';
import { useTranslations } from 'next-intl';
import { useForm } from 'react-hook-form';
import { toast } from 'sonner';

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
  FormRootMessage,
  Input,
  PendingButton,
  Switch,
} from '@repo/ui/components';
import { updateClientTagRule } from '@/app/(protected)/settings/client-tags/actions';
import { clientTagRuleSchema } from '@/app/(protected)/settings/client-tags/form-schema';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import type { ClientTagRule } from '@/lib/api/client-tag-rules';
import { FORM_VALIDATION } from '@/lib/forms/options';

interface ClientTagRuleFormProps {
  rule: ClientTagRule;
}

export function ClientTagRuleForm({ rule }: ClientTagRuleFormProps) {
  const t = useTranslations('clientTagSettings');
  const message = useApiErrorMessage();
  const form = useForm<ClientTagRule>({
    ...FORM_VALIDATION,
    resolver: zodResolver(clientTagRuleSchema(t)),
    defaultValues: rule,
  });
  const { isSubmitting, isDirty } = form.formState;

  async function save(values: ClientTagRule) {
    const error = await updateClientTagRule(values);
    if (error) {
      form.setError('root', { message: message(error) });
      return;
    }
    form.reset(values);
    toast.success(t('saved'));
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t(`rules.${rule.kind}.name`)}</CardTitle>
        <CardDescription>{t(`rules.${rule.kind}.description`)}</CardDescription>
      </CardHeader>
      <CardContent>
        <Form {...form}>
          <form className="flex flex-col gap-y-5" noValidate onSubmit={form.handleSubmit(save)}>
            <FormField
              control={form.control}
              name="enabled"
              render={({ field }) => (
                <FormItem className="flex flex-row items-center justify-between">
                  <FormLabel>{t('enabled')}</FormLabel>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                      disabled={isSubmitting}
                    />
                  </FormControl>
                </FormItem>
              )}
            />
            {(['minimum', 'windowDays'] as const).map((name) => (
              <FormField
                key={name}
                control={form.control}
                name={name}
                render={({ field }) => (
                  <FormItem>
                    <FormLabel required>{t(name)}</FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        type="number"
                        inputMode="numeric"
                        value={Number.isNaN(field.value) ? '' : field.value}
                        onChange={(event) => field.onChange(event.target.valueAsNumber)}
                        disabled={isSubmitting}
                      />
                    </FormControl>
                    {name === 'windowDays' ? (
                      <FormDescription>{t('windowHint')}</FormDescription>
                    ) : null}
                    <FormMessage />
                  </FormItem>
                )}
              />
            ))}
            <FormRootMessage />
            <div className="flex justify-end">
              <PendingButton
                type="submit"
                pending={isSubmitting}
                pendingLabel={t('saving')}
                disabled={!isDirty}
              >
                {t('save')}
              </PendingButton>
            </div>
          </form>
        </Form>
      </CardContent>
    </Card>
  );
}
