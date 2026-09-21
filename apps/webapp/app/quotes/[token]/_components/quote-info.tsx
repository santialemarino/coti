import { CalendarIcon, DownloadIcon, FileTextIcon } from 'lucide-react';
import { getTranslations } from 'next-intl/server';

import { Button, Card } from '@repo/ui/components';
import { getFormatters } from '@/lib/i18n/formatters-server';

interface QuoteInfoProps {
  expiresAt: string;
  validityNote: string;
  pdfUrl?: string;
}

/*
 * One balanced information panel below the items: validity, the seller's observation and the PDF
 * copy in equal columns divided by hairlines on desktop, stacked on a phone. The PDF lives inside
 * the panel like the other two columns do, not as a button floating in an empty card.
 */
export async function QuoteInfo({ expiresAt, validityNote, pdfUrl }: QuoteInfoProps) {
  const fmt = await getFormatters();
  const t = await getTranslations('quote');

  return (
    <Card className="gap-y-0 overflow-hidden py-0">
      <div className="grid grid-cols-1 md:grid-cols-3 md:divide-x md:divide-border">
        <div className="flex items-start gap-x-3 px-6 py-5">
          <CalendarIcon
            aria-hidden="true"
            className="mt-0.5 size-4 shrink-0 text-foreground-muted"
          />
          <div className="flex flex-col gap-y-0.5">
            <p className="text-paragraph-xs-medium text-foreground-subtle">{t('infoValidity')}</p>
            <p className="text-paragraph-sm-medium text-foreground">{fmt.date(expiresAt)}</p>
          </div>
        </div>

        {validityNote ? (
          <div className="flex items-start gap-x-3 px-6 py-5">
            <FileTextIcon
              aria-hidden="true"
              className="mt-0.5 size-4 shrink-0 text-foreground-muted"
            />
            <div className="flex flex-col gap-y-0.5">
              <p className="text-paragraph-xs-medium text-foreground-subtle">
                {t('infoObservations')}
              </p>
              <p className="text-paragraph-sm text-foreground-muted">{validityNote}</p>
            </div>
          </div>
        ) : null}

        {pdfUrl ? (
          <div className="flex flex-col items-start gap-y-2 px-6 py-5">
            <div className="flex items-center gap-x-3">
              <DownloadIcon aria-hidden="true" className="size-4 shrink-0 text-foreground-muted" />
              <p className="text-paragraph-xs-medium text-foreground-subtle">{t('infoPdf')}</p>
            </div>
            <Button asChild variant="outline" className="w-full sm:w-auto">
              <a href={pdfUrl} target="_blank" rel="noopener noreferrer">
                {t('downloadPdf')}
              </a>
            </Button>
          </div>
        ) : null}
      </div>
    </Card>
  );
}
