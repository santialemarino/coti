'use client';

import { useCallback, useEffect, useRef, useState } from 'react';
import Image from 'next/image';
import { InboxIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';

import {
  Button,
  Callout,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
  DropdownChevron,
  Skeleton,
} from '@repo/ui/components';
import { listRfqAttachments, type RfqAttachment } from '@/lib/api/rfq-attachments';

interface RfqOriginalRequestProps {
  rfqId: string;
  branchId: string;
  text?: string;
}
function safeURL(value: string): string | null {
  try {
    const url = new URL(value);
    return ['https:', 'http:'].includes(url.protocol) ? url.toString() : null;
  } catch {
    return null;
  }
}
export function RfqOriginalRequest({ rfqId, branchId, text }: RfqOriginalRequestProps) {
  const t = useTranslations('rfqs.originalRequest');
  const [attachments, setAttachments] = useState<RfqAttachment[]>([]);
  const [error, setError] = useState(false);
  const [loaded, setLoaded] = useState(false);
  const [open, setOpen] = useState(false);
  const request = useRef(0);
  const body = text?.trim() ? text : '';

  // Links are signed and short-lived, so each opening fetches fresh ones instead of offering a refresh.
  const load = useCallback(() => {
    const current = ++request.current;
    setError(false);
    void listRfqAttachments(rfqId, branchId)
      .then((files) => {
        if (current === request.current) setAttachments(files);
      })
      .catch(() => {
        if (current === request.current) setError(true);
      })
      .finally(() => {
        if (current === request.current) setLoaded(true);
      });
  }, [rfqId, branchId]);

  useEffect(() => {
    request.current += 1;
    setOpen(false);
    setLoaded(false);
    setAttachments([]);
  }, [rfqId, branchId]);

  function handleOpenChange(next: boolean) {
    setOpen(next);
    if (next) load();
  }

  return (
    <Card>
      <Collapsible open={open} onOpenChange={handleOpenChange}>
        <CardHeader>
          <CollapsibleTrigger className="group/panel flex w-full items-center gap-x-2 rounded-md outline-none transition-colors duration-200 ease-out-soft hover:text-primary focus-visible:text-primary">
            <InboxIcon aria-hidden="true" className="size-4 text-foreground-muted" />
            <CardTitle>{t('title')}</CardTitle>
            <DropdownChevron className="ml-auto group-data-[state=open]/panel:rotate-180" />
          </CollapsibleTrigger>
        </CardHeader>
        <CollapsibleContent>
          <CardContent className="flex flex-col pt-4 gap-y-4">
            {body ? (
              <p className="whitespace-pre-wrap break-words text-paragraph-sm">{body}</p>
            ) : null}
            {!loaded && !error ? <Skeleton className="h-9 w-full" /> : null}
            {error ? (
              <Callout
                tone="danger"
                action={
                  <Button variant="outline" size="sm" onClick={load}>
                    {t('retry')}
                  </Button>
                }
              >
                {t('error')}
              </Callout>
            ) : null}
            {loaded && !error && !body && !attachments.length ? (
              <p className="text-paragraph-sm text-foreground-muted">{t('unavailable')}</p>
            ) : null}
            {attachments.map((attachment) => {
              const url = safeURL(attachment.url);
              const name = attachment.name || t('unnamed');
              return (
                <div key={attachment.id} className="flex flex-col gap-y-3">
                  <div className="flex items-center justify-between gap-x-2">
                    <span className="min-w-0 text-paragraph-sm wrap-anywhere">{name}</span>
                    {url ? (
                      <Button asChild variant="outline" size="sm">
                        <a href={url} target="_blank" rel="noopener noreferrer">
                          {t('download')}
                        </a>
                      </Button>
                    ) : null}
                  </div>
                  {attachment.status === 'FAILED' ? (
                    <p className="text-paragraph-sm text-danger-foreground">{t('manualReview')}</p>
                  ) : null}
                  {url && attachment.type === 'IMAGE' ? (
                    <Image
                      src={url}
                      alt={name}
                      width={1200}
                      height={900}
                      unoptimized
                      className="h-auto max-h-96 w-full object-contain rounded-lg"
                    />
                  ) : null}
                  {url && attachment.type === 'AUDIO' ? (
                    <audio src={url} controls preload="none" className="w-full" aria-label={name} />
                  ) : null}
                </div>
              );
            })}
          </CardContent>
        </CollapsibleContent>
      </Collapsible>
    </Card>
  );
}
