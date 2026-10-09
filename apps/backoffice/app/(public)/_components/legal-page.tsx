import { getTranslations } from 'next-intl/server';

import { getFormatters } from '@/lib/i18n/formatters-server';

interface LegalSection {
  title: string;
  paragraphs: string[];
}

interface LegalPageProps {
  namespace: 'legal.privacy' | 'legal.terms';
  updatedAt: string;
}

// A legal text set as prose: its sections come from the catalog, its date from the page.
export async function LegalPage({ namespace, updatedAt }: LegalPageProps) {
  const fmt = await getFormatters();
  const t = await getTranslations(namespace);
  const tLegal = await getTranslations('legal');
  const sections = t.raw('sections') as LegalSection[];

  return (
    <article className="flex flex-col w-full max-w-3xl mx-auto px-4 py-12 gap-y-10 sm:px-6 lg:py-16">
      <header className="flex flex-col gap-y-3">
        <h1 className="text-heading-2 text-foreground">{t('title')}</h1>
        <p className="text-paragraph-sm text-foreground-subtle">
          {tLegal('updatedAt', { date: fmt.date(updatedAt) })}
        </p>
        <p className="text-paragraph text-foreground-muted">{t('intro')}</p>
      </header>
      {sections.map((section) => (
        <section key={section.title} className="flex flex-col gap-y-3">
          <h2 className="text-heading-5 text-foreground">{section.title}</h2>
          {section.paragraphs.map((paragraph) => (
            <p key={paragraph} className="text-paragraph-sm text-foreground-muted">
              {paragraph}
            </p>
          ))}
        </section>
      ))}
    </article>
  );
}
