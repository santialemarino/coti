'use client';

import { useEffect, useState, useTransition } from 'react';
import { useTranslations } from 'next-intl';

import {
  Button,
  Callout,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  Skeleton,
} from '@repo/ui/components';
import { cn } from '@repo/ui/lib';
import { listRfqAttachments, type RfqAttachment } from '@/lib/api/rfq-attachments';

interface RfqOriginalRequestProps {
  rfqId: string;
  branchId: string;
  text?: string;
}

function safeURL(value: string): string | null {
  try {
    const url = new URL(value);
    return url.protocol === 'https:' || url.protocol === 'http:' ? url.toString() : null;
  } catch {
    return null;
  }
}

export function RfqOriginalRequest({ rfqId, branchId, text }: RfqOriginalRequestProps) {
  const t = useTranslations('rfqs.originalRequest');
  const [attachments, setAttachments] = useState<RfqAttachment[]>([]);
  const [error, setError] = useState(false);
  const [loaded, setLoaded] = useState(false);
  const [pending, startRefresh] = useTransition();
  const originalText = text?.trim();

  useEffect(() => {
    let active = true;
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

  const filesShown = attachments.length > 0 || error;
  if (loaded && !originalText && !filesShown) return null;
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('title')}</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-y-4">
        {originalText ? (
          <p className="p-4 bg-sunken rounded-lg text-paragraph-sm whitespace-pre-wrap break-words">
            {originalText}
          </p>
        ) : null}
        {!loaded && !originalText ? <Skeleton className="h-9 w-full" /> : null}
        {/* The files arrive after the text: their block grows into place instead of pushing the page. */}
        <div
          aria-hidden={!filesShown}
          className={cn(
            'grid -mt-4 transition-[grid-template-rows] duration-200 ease-out-soft motion-reduce:transition-none',
            filesShown ? 'grid-rows-[1fr]' : 'grid-rows-[0fr]',
          )}
        >
          <div className="flex flex-col min-h-0 pt-4 gap-y-4 overflow-hidden">
            {error ? <Callout tone="danger">{t('error')}</Callout> : null}
            {attachments.length ? (
              <div className="flex flex-col gap-y-3">
                <h3 className="text-paragraph-sm-medium">{t('files')}</h3>
                {attachments.map((attachment) => {
                  const url = safeURL(attachment.url);
                  return (
                    <div
                      key={attachment.id}
                      className="flex flex-wrap items-center justify-between gap-2"
                    >
                      <span className="text-paragraph-sm text-foreground">
                        {attachment.name || t('unnamed')}
                      </span>
                      {attachment.status === 'FAILED' ? (
                        <span className="text-paragraph-sm text-danger-foreground">
                          {t('manualReview')}
                        </span>
                      ) : null}
                      {url ? (
                        <Button asChild variant="outline" size="sm">
                          <a href={url} target="_blank" rel="noopener noreferrer">
                            {t('download')}
                          </a>
                        </Button>
                      ) : null}
                    </div>
                  );
                })}
              </div>
            ) : null}
            {filesShown ? (
              <Button variant="outline" size="sm" disabled={pending} onClick={refresh}>
                {t('refresh')}
              </Button>
            ) : null}
          </div>
        </div>
      </CardContent>
    </Card>
  );
}
