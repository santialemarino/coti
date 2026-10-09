import Link from 'next/link';
import { getTranslations } from 'next-intl/server';

import { InlineLink } from '@repo/ui/components';
import { Brand } from '@/components/brand';
import { ROUTES } from '@/config/routes';

export async function PublicFooter() {
  const t = await getTranslations('landing.footer');
  const tCommon = await getTranslations('common');
  const links = [
    { href: ROUTES.privacy, label: t('privacy') },
    { href: ROUTES.terms, label: t('terms') },
  ];

  return (
    <footer className="border-t border-border bg-card">
      <div className="flex flex-col w-full max-w-6xl mx-auto px-4 py-10 gap-y-6 sm:flex-row sm:items-start sm:justify-between sm:px-6">
        <div className="flex flex-col gap-y-2">
          <Brand size="sm" label={tCommon('appName')} />
          <p className="text-paragraph-sm text-foreground-muted">{t('tagline')}</p>
        </div>
        <div className="flex flex-col gap-y-3">
          <nav aria-label={t('nav')}>
            <ul className="flex flex-wrap gap-x-6 gap-y-2">
              {links.map((link) => (
                <li key={link.href}>
                  <InlineLink asChild tone="muted">
                    <Link href={link.href}>{link.label}</Link>
                  </InlineLink>
                </li>
              ))}
            </ul>
          </nav>
          <p className="text-paragraph-xs text-foreground-subtle">
            {t('copyright', { year: new Date().getFullYear() })}
          </p>
        </div>
      </div>
    </footer>
  );
}
