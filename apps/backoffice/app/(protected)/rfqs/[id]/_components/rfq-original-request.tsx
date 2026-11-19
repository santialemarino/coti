'use client';

import { useEffect, useState, useTransition } from 'react';
import Image from 'next/image';
import { useTranslations } from 'next-intl';

import {
  Button,
  Callout,
  Card,
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
  const [pending, startRefresh] = useTransition();
  useEffect(() => {
    let active = true;
    setLoaded(false);
    setError(false);
    setAttachments([]);
    setOpen(false);
    void listRfqAttachments(rfqId, branchId)
      .then((files) => {
        if (active) setAttachments(files);
      })
      .catch(() => {
        if (active) setError(true);
      })
      .finally(() => {
        if (active) setLoaded(true);
      });
    return () => {
      active = false;
    };
  }, [rfqId, branchId]);
  function refresh() {
    startRefresh(async () => {
      try {
        setAttachments(await listRfqAttachments(rfqId, branchId));
        setError(false);
      } catch {
        setError(true);
      }
    });
  }
  return (
    <Card>
      <Collapsible open={open} onOpenChange={setOpen}>
        <CollapsibleTrigger asChild>
          <Button variant="ghost" className="h-auto w-full justify-between p-5">
            <span className="text-heading-6">{t('title')}</span>
            <DropdownChevron open={open} />
          </Button>
        </CollapsibleTrigger>
        <CollapsibleContent>
          <div className="flex flex-col px-5 pb-5 gap-4">
            {text?.trim() ? (
              <p className="p-4 bg-sunken rounded-lg whitespace-pre-wrap break-words text-paragraph-sm">
                {text}
              </p>
            ) : null}
            {!loaded ? <Skeleton className="h-9 w-full" /> : null}
            {error ? <Callout tone="danger">{t('error')}</Callout> : null}
            {loaded && !error && !text?.trim() && !attachments.length ? (
              <p className="text-paragraph-sm text-foreground-muted">{t('unavailable')}</p>
            ) : null}
            {attachments.map((attachment) => {
              const url = safeURL(attachment.url);
              return (
                <div key={attachment.id} className="flex flex-col gap-3">
                  <div className="flex items-center justify-between gap-2">
                    <span className="text-paragraph-sm">{attachment.name || t('unnamed')}</span>
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
                      alt={attachment.name || t('unnamed')}
                      width={1200}
                      height={900}
                      unoptimized
                      className="h-auto max-h-96 w-full object-contain rounded-lg"
                    />
                  ) : null}
                  {url && attachment.type === 'AUDIO' ? (
                    <audio
                      src={url}
                      controls
                      preload="none"
                      className="w-full"
                      aria-label={attachment.name || t('unnamed')}
                    />
                  ) : null}
                </div>
              );
            })}
            {attachments.length || error ? (
              <Button variant="outline" size="sm" disabled={pending} onClick={refresh}>
                {t('refresh')}
              </Button>
            ) : null}
          </div>
        </CollapsibleContent>
      </Collapsible>
    </Card>
  );
}
