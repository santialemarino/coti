'use client';

import { useRef, useState } from 'react';
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
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
  FormRootMessage,
  PendingButton,
  RowActionButton,
  Textarea,
} from '@repo/ui/components';
import { MOTION } from '@repo/ui/lib';
import {
  CLIENT_NOTES_MAX_LENGTH,
  clientNotesSchema,
  type ClientNotesValues,
} from '@/app/(protected)/clients/[id]/form-schema';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import { updateClientNotes } from '@/lib/api/clients-client';
import { errorCodeOf } from '@/lib/api/errors';
import { FORM_VALIDATION } from '@/lib/forms/options';

interface ClientNotesEditorProps {
  clientId: string;
  name: string;
  notes: string | null;
}

export function ClientNotesEditor({ clientId, name, notes }: ClientNotesEditorProps) {
  const router = useRouter();
  const t = useTranslations('clients.notes');
  const tErrors = useTranslations('common.form.errors');
  const message = useApiErrorMessage('clients.notes.errors');
  const reduced = useReducedMotion();
  const [editing, setEditing] = useState(false);
  const [saved, setSaved] = useState<{ previous: string | null; value: string | null }>();
  const focusOnMount = useRef(false);
  const displayed = saved?.previous === notes ? saved.value : notes;
  const form = useForm<ClientNotesValues>({
    ...FORM_VALIDATION,
    resolver: zodResolver(clientNotesSchema({ field: t, shared: tErrors })),
    defaultValues: { notes: displayed ?? '' },
  });
  const { isSubmitting } = form.formState;
  const transition = { duration: reduced ? 0 : MOTION.fast };

  function open() {
    form.reset({ notes: displayed ?? '' });
    focusOnMount.current = true;
    setEditing(true);
  }

  function close() {
    focusOnMount.current = true;
    setEditing(false);
  }

  async function onSubmit(values: ClientNotesValues) {
    try {
      const client = await updateClientNotes(clientId, values.notes);
      setSaved({ previous: notes, value: client.notes });
      toast.success(t('saved', { name }));
      close();
      router.refresh();
    } catch (cause) {
      form.setError('root', { message: message(errorCodeOf(cause)) });
    }
  }

  return (
    <AnimatePresence initial={false} mode="wait">
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
              onKeyDown={(event) => {
                if (event.key === 'Escape' && !isSubmitting) {
                  event.preventDefault();
                  event.stopPropagation();
                  close();
                }
              }}
              noValidate
              className="flex flex-col gap-y-3"
            >
              <FormField
                control={form.control}
                name="notes"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('label')}</FormLabel>
                    <FormControl>
                      <Textarea
                        {...field}
                        ref={(node) => {
                          field.ref(node);
                          if (node && focusOnMount.current) {
                            focusOnMount.current = false;
                            node.focus();
                          }
                        }}
                        maxLength={CLIENT_NOTES_MAX_LENGTH}
                        placeholder={t('placeholder')}
                        disabled={isSubmitting}
                        rows={3}
                      />
                    </FormControl>
                    <FormDescription>{t('hint')}</FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormRootMessage />
              <div className="flex justify-end gap-x-2">
                <Button variant="outline" size="sm" disabled={isSubmitting} onClick={close}>
                  {t('cancel')}
                </Button>
                <PendingButton
                  type="submit"
                  size="sm"
                  pending={isSubmitting}
                  pendingLabel={t('saving')}
                >
                  {t('save')}
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
          className="flex min-w-0 flex-col gap-y-1"
        >
          <div className="flex items-center gap-x-1">
            <span className="text-paragraph-xs text-foreground-muted">{t('label')}</span>
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
          </div>
          {displayed ? (
            <p className="wrap-anywhere whitespace-pre-wrap text-paragraph-sm text-foreground">
              {displayed}
            </p>
          ) : (
            <p className="text-paragraph-sm text-foreground-subtle">{t('empty')}</p>
          )}
        </motion.div>
      )}
    </AnimatePresence>
  );
}
