'use client';

import { useMemo, useState } from 'react';
import { MailIcon, MessageCircleIcon, SendIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { toast } from 'sonner';

import {
  Button,
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Label,
  RadioGroup,
  RadioGroupItem,
  Textarea,
} from '@repo/ui/components';
import type { RfqDetailResponse } from '@/lib/api/rfqs';
import { useFormatters } from '@/lib/i18n/formatters';

type SendChannel = 'whatsapp' | 'email';

function formatId(id: string): string {
  if (/^\d{1,6}$/.test(id)) return id;
  return id.replace(/-/g, '').slice(0, 6).toUpperCase();
}

interface SendQuoteDialogProps {
  detail: RfqDetailResponse;
}

/*
 * Send modal shared by QUOTED and CHANGE_REQUESTED reads. The real handoff
 * (WhatsApp/email) is not wired yet, so the send button acknowledges the draft
 * is ready without leaving the screen.
 */
export function SendQuoteDialog({ detail }: SendQuoteDialogProps) {
  const t = useTranslations('rfqs.detail.send');
  const fmt = useFormatters();
  const [open, setOpen] = useState(false);
  const [channel, setChannel] = useState<SendChannel>(
    detail.rfq.channel === 'email' ? 'email' : 'whatsapp',
  );
  const [message, setMessage] = useState('');
  const [sending, setSending] = useState(false);

  const defaultMessage = useMemo(() => {
    const client = detail.rfq.client;
    return [
      t('greeting', { client: client?.trim() ? client.trim() : t('noClient') }),
      t('body', { id: formatId(detail.rfq.id), total: fmt.currency(detail.version?.total ?? '0') }),
      t('closing'),
    ].join(' ');
  }, [detail.rfq.client, detail.rfq.id, detail.version?.total, fmt, t]);

  function handleOpenChange(next: boolean) {
    if (sending) return;
    if (next) setMessage(defaultMessage);
    setOpen(next);
  }

  function handleSend() {
    setSending(true);
    // The handoff is next screen; nothing leaves here yet.
    window.setTimeout(() => {
      setSending(false);
      setOpen(false);
      toast.success(t('toast.comingSoon'));
    }, 600);
  }

  return (
    <>
      <Button type="button" onClick={() => handleOpenChange(true)}>
        <SendIcon className="size-4" />
        {t('button')}
      </Button>

      <Dialog open={open} onOpenChange={handleOpenChange}>
        <DialogContent className="sm:max-w-lg" closeOnClickOutside={!sending}>
          <DialogHeader>
            <DialogTitle>{t('title')}</DialogTitle>
          </DialogHeader>

          <div className="flex flex-col gap-y-4">
            <div className="flex flex-col gap-y-2">
              <Label className="text-foreground">{t('channelLabel')}</Label>
              <RadioGroup
                value={channel}
                onValueChange={(value: SendChannel) => setChannel(value)}
                className="flex flex-col gap-y-2"
              >
                <label
                  htmlFor="send-channel-whatsapp"
                  className="flex cursor-pointer items-center gap-x-3 rounded-lg border border-border bg-card p-3 text-paragraph-sm transition-colors hover:bg-accent"
                >
                  <RadioGroupItem id="send-channel-whatsapp" value="whatsapp" />
                  <MessageCircleIcon className="size-4 text-foreground-muted" />
                  <span className="font-medium text-foreground">{t('whatsapp')}</span>
                </label>
                <label
                  htmlFor="send-channel-email"
                  className="flex cursor-pointer items-center gap-x-3 rounded-lg border border-border bg-card p-3 text-paragraph-sm transition-colors hover:bg-accent"
                >
                  <RadioGroupItem id="send-channel-email" value="email" />
                  <MailIcon className="size-4 text-foreground-muted" />
                  <span className="font-medium text-foreground">{t('email')}</span>
                </label>
              </RadioGroup>
            </div>

            <div className="flex flex-col gap-y-1.5">
              <Label htmlFor="send-message" className="text-foreground">
                {t('messageLabel')}
              </Label>
              <Textarea
                id="send-message"
                value={message}
                onChange={(event) => setMessage(event.target.value)}
                rows={4}
              />
            </div>
          </div>

          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              disabled={sending}
              onClick={() => handleOpenChange(false)}
            >
              {t('cancel')}
            </Button>
            <Button type="button" disabled={sending} onClick={handleSend}>
              <SendIcon className="size-4" />
              {sending ? t('sending') : t('send')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
