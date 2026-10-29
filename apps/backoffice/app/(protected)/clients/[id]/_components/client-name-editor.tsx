'use client';

import { useMemo, useRef, useState } from 'react';
import { useRouter } from 'next/navigation';
import { zodResolver } from '@hookform/resolvers/zod';
import { PencilIcon } from 'lucide-react';
import { AnimatePresence, motion, useReducedMotion } from 'motion/react';
import { useTranslations } from 'next-intl';
import { useForm } from 'react-hook-form';
import { toast } from 'sonner';

import {
  Button,
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
  FormRootMessage,
  Hint,
  Input,
  PendingButton,
  RowActionButton,
} from '@repo/ui/components';
import { MOTION } from '@repo/ui/lib';
import {
  clientNameSchema,
  type ClientNameValues,
} from '@/app/(protected)/clients/[id]/form-schema';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import { renameClient } from '@/lib/api/clients-client';
import { errorCodeOf } from '@/lib/api/errors';
import { TEXT_FIELD_MAX_LENGTH } from '@/lib/constants/forms';
import { FORM_VALIDATION } from '@/lib/forms/options';

interface ClientNameEditorProps {
  clientId: string;
  name: string | null;
  registered: boolean;
}

/*
 * A provisional profile often keeps the WhatsApp display name it arrived with, so the seller can
 * correct it in place. A registered profile's name is its customer's, so it only says so.
 */
export function ClientNameEditor({ clientId, name, registered }: ClientNameEditorProps) {
  const router = useRouter();
  const t = useTranslations('clients.rename');
  const tErrors = useTranslations('common.form.errors');
  const message = useApiErrorMessage('clients.rename');
  const reduced = useReducedMotion();
  const [editing, setEditing] = useState(false);
  // The incoming stage mounts only once the outgoing one has faded, so each takes focus on mount.
  const focusOnMount = useRef(false);
  const schema = useMemo(() => clientNameSchema({ field: t, shared: tErrors }), [t, tErrors]);
  const form = useForm<ClientNameValues>({
    ...FORM_VALIDATION,
    resolver: zodResolver(schema),
    defaultValues: { name: name ?? '' },
  });
  const transition = { duration: reduced ? 0 : MOTION.fast };

  function open() {
    form.reset({ name: name ?? '' });
    focusOnMount.current = true;
    setEditing(true);
  }

  function close() {
    focusOnMount.current = true;
    setEditing(false);
  }

  async function onSubmit(values: ClientNameValues) {
    try {
      const saved = await renameClient(clientId, values.name);
      toast.success(t('saved', { name: saved.name ?? values.name }));
      close();
      router.refresh();
    } catch (cause) {
      form.setError('root', { message: message(errorCodeOf(cause)) });
    }
  }

  return (
    <div className="flex flex-col gap-y-1">
      <span className="text-paragraph-xs text-foreground-muted">{t('label')}</span>
      <AnimatePresence mode="wait" initial={false}>
        {editing ? (
          <motion.div
            key="edit"
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            transition={transition}
          >
            <Form {...form}>
              <form
                onSubmit={form.handleSubmit(onSubmit)}
                noValidate
                aria-label={t('formLabel')}
                className="flex flex-col gap-y-3"
              >
                <FormField
                  control={form.control}
                  name="name"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel className="sr-only">{t('label')}</FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          ref={(node) => {
                            field.ref(node);
                            if (node && focusOnMount.current) {
                              focusOnMount.current = false;
                              node.focus();
                              node.select();
                            }
                          }}
                          maxLength={TEXT_FIELD_MAX_LENGTH}
                          placeholder={t('placeholder')}
                          disabled={form.formState.isSubmitting}
                          onKeyDown={(event) => {
                            if (event.key === 'Escape' && !form.formState.isSubmitting) {
                              event.preventDefault();
                              close();
                            }
                          }}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormRootMessage />
                <div className="flex justify-end gap-x-2">
                  <Button
                    variant="outline"
                    size="sm"
                    disabled={form.formState.isSubmitting}
                    onClick={close}
                  >
                    {t('cancel')}
                  </Button>
                  <PendingButton
                    type="submit"
                    size="sm"
                    pending={form.formState.isSubmitting}
                    pendingLabel={t('submitting')}
                  >
                    {t('submit')}
                  </PendingButton>
                </div>
              </form>
            </Form>
          </motion.div>
        ) : (
          <motion.div
            key="view"
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            transition={transition}
            className="flex flex-col gap-y-0.5"
          >
            <div className="flex items-center gap-x-1">
              <span className="text-paragraph-sm text-foreground wrap-anywhere">
                {name ?? t('unnamed')}
              </span>
              {registered ? null : (
                <RowActionButton
                  ref={(node) => {
                    if (node && focusOnMount.current) {
                      focusOnMount.current = false;
                      node.focus();
                    }
                  }}
                  icon={PencilIcon}
                  label={t('edit')}
                  onClick={open}
                />
              )}
            </div>
            {registered ? <Hint>{t('registered')}</Hint> : null}
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  );
}
