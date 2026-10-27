import { getTranslations } from 'next-intl/server';

import { Reveal } from '@/app/(public)/_components/reveal';
import { revealItem } from '@/app/(public)/_components/reveal-item';
import { getFormatters } from '@/lib/i18n/formatters-server';

// The heading, its date and the intro take the first steps of the entrance.
const SECTION_STEP = 3;

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
        <h1 {...revealItem(0, 'text-heading-2 text-foreground')}>{t('title')}</h1>
        <p {...revealItem(1, 'text-paragraph-sm text-foreground-subtle')}>
          {tLegal('updatedAt', { date: fmt.date(updatedAt) })}
        </p>
        <p {...revealItem(2, 'text-paragraph text-foreground-muted')}>{t('intro')}</p>
      </header>
      {sections.map((section) => (
        <Reveal key={section.title}>
          {/* After the heading's three steps, so the opening screen reads top to bottom. */}
          <section {...revealItem(SECTION_STEP, 'flex flex-col gap-y-3')}>
            <h2 className="text-heading-5 text-foreground">{section.title}</h2>
            {section.paragraphs.map((paragraph) => (
              <p key={paragraph} className="text-paragraph-sm text-foreground-muted">
                {paragraph}
              </p>
            ))}
          </section>
        </Reveal>
      ))}
    </article>
  );
}
