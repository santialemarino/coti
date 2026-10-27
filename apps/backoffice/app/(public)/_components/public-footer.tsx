import Link from 'next/link';
import { getTranslations } from 'next-intl/server';

import { InlineLink } from '@repo/ui/components';
import { Brand } from '@/components/brand';
import { ROUTES } from '@/config/routes';

export async function PublicFooter() {
  const t = await getTranslations('landing.footer');
  const tCommon = await getTranslations('common');
  const columns = [
    {
      key: 'product',
      links: [
        { href: ROUTES.landingSection('steps'), label: 'steps' },
        { href: ROUTES.landingSection('features'), label: 'features' },
        { href: ROUTES.landingSection('faq'), label: 'faq' },
      ],
    },
    {
      key: 'legal',
      links: [
        { href: ROUTES.privacy, label: 'privacy' },
        { href: ROUTES.terms, label: 'terms' },
      ],
    },
  ];

  return (
    <footer className="border-t border-border bg-card">
      <div className="flex flex-col w-full max-w-6xl mx-auto px-4 sm:px-6">
        <div className="grid grid-cols-2 py-12 gap-x-8 gap-y-10 lg:grid-cols-12 lg:py-16">
          <div className="flex flex-col col-span-2 gap-y-3 lg:col-span-8">
            <Link
              href={ROUTES.landing}
              className="w-fit rounded-md outline-none focus-visible:ring-3 focus-visible:ring-ring/45"
            >
              <Brand label={tCommon('appName')} />
            </Link>
            <p className="text-paragraph-sm text-foreground-muted">{t('tagline')}</p>
          </div>
          <nav aria-label={t('nav')} className="grid col-span-2 grid-cols-2 gap-x-8 lg:col-span-4">
            {columns.map((column) => (
              <div key={column.key} className="flex flex-col gap-y-4">
                <h2 className="text-paragraph-sm-semibold text-foreground">
                  {t(`columns.${column.key}.title`)}
                </h2>
                <ul className="flex flex-col gap-y-3">
                  {column.links.map((link) => (
                    <li key={link.label}>
                      <InlineLink asChild tone="muted">
                        <Link href={link.href}>
                          {t(`columns.${column.key}.links.${link.label}`)}
                        </Link>
                      </InlineLink>
                    </li>
                  ))}
                </ul>
              </div>
            ))}
          </nav>
        </div>
        <div className="flex flex-col py-6 gap-y-2 border-t border-border text-paragraph-xs text-foreground-subtle sm:flex-row sm:items-center sm:justify-between">
          <p>{t('copyright', { year: new Date().getFullYear() })}</p>
          <p>{t('madeIn')}</p>
        </div>
      </div>
    </footer>
  );
}
