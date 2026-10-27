'use client';

import { useEffect, useState, useTransition } from 'react';
import { useTranslations } from 'next-intl';

import { Button, Callout, Card, CardContent, CardHeader, CardTitle } from '@repo/ui/components';
import { listRfqAttachments, type RfqAttachment } from '@/lib/api/rfq-attachments';

function safeURL(value: string): string | null {
  try {
    const url = new URL(value);
    return url.protocol === 'https:' || url.protocol === 'http:' ? url.toString() : null;
  } catch {
    return null;
  }
}

export function RfqEmailAttachments({ rfqId, branchId }: { rfqId: string; branchId: string }) {
  const t = useTranslations('rfqs.emailAttachments');
  const [attachments, setAttachments] = useState<RfqAttachment[]>([]);
  const [error, setError] = useState(false);
  const [pending, startRefresh] = useTransition();

  useEffect(() => {
    let active = true;
    void listRfqAttachments(rfqId, branchId)
      .then((files) => {
        if (active) setAttachments(files);
      })
      .catch(() => {
        if (active) setError(true);
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

  if (!error && attachments.length === 0) return null;
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('title')}</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-y-3">
        {error ? <Callout tone="danger">{t('error')}</Callout> : null}
        {attachments.map((attachment) => {
          const url = safeURL(attachment.url);
          return (
            <div key={attachment.id} className="flex flex-wrap items-center justify-between gap-2">
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
        <Button variant="outline" size="sm" disabled={pending} onClick={refresh}>
          {t('refresh')}
        </Button>
      </CardContent>
    </Card>
  );
}
