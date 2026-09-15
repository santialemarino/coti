import { getTranslations } from 'next-intl/server';

import type { Branch, Supplier } from '@/lib/api/public-quotes';
import { getFormatters } from '@/lib/i18n/formatters-server';

interface QuoteHeaderProps {
  supplier: Supplier;
  branch: Branch;
  reference: string;
  versionNumber: number;
  approvedAt: string;
}

/*
 * The corralón's brand colour arrives as tenant data, not as a design token, so it is the one
 * colour on this page set inline. It paints a rule rather than any text: the value comes from an
 * account we do not control, and a hex chosen for a logo carries no contrast guarantee against the
 * card behind it.
 */
export async function QuoteHeader({
  supplier,
  branch,
  reference,
  versionNumber,
  approvedAt,
}: QuoteHeaderProps) {
  const fmt = await getFormatters();
  const t = await getTranslations('quote');

  return (
    <header className="flex flex-col gap-y-4">
      <span
        aria-hidden="true"
        className="h-1 w-16 rounded-full"
        style={{ backgroundColor: supplier.brandColor }}
      />
      <div className="flex flex-wrap items-end justify-between gap-x-6 gap-y-3">
        <div className="flex flex-col min-w-0 gap-y-1">
          {supplier.logoUrl ? (
            // Not next/image: the host is a tenant's own, unknown at build time.
            // eslint-disable-next-line @next/next/no-img-element
            <img
              src={supplier.logoUrl}
              alt={t('logoAlt', { name: supplier.name })}
              className="mb-1 h-10 w-auto max-w-56 self-start object-contain"
            />
          ) : null}
          <p className="text-heading-4 text-foreground">{supplier.name}</p>
          <p className="text-paragraph-sm text-foreground-muted">
            {branch.address ? `${branch.name} · ${branch.address}` : branch.name}
          </p>
        </div>
        <div className="flex flex-col items-start gap-y-1 sm:items-end">
          <p className="text-paragraph-medium text-foreground">{t('reference', { reference })}</p>
          <p className="text-paragraph-sm text-foreground-muted">
            {t('version', { number: versionNumber })} ·{' '}
            {t('issuedOn', { date: fmt.date(approvedAt) })}
          </p>
        </div>
      </div>
    </header>
  );
}
