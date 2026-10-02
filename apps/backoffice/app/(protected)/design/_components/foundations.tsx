import { Item, Row, Section } from '@/app/(protected)/design/_components/section';

/*
 * Every class here is a literal string. Tailwind scans source text, so a class assembled at runtime
 * (`bg-brand-${step}`) is invisible to it and simply never gets generated — the swatch would render
 * transparent. The ramp primitives that are not exposed as utilities are read from their CSS variable
 * through an inline style instead.
 */
const BRAND = [
  ['50', 'bg-brand-50'],
  ['100', 'bg-brand-100'],
  ['200', 'bg-brand-200'],
  ['300', 'bg-brand-300'],
  ['400', 'bg-brand-400'],
  ['500', 'bg-brand-500'],
  ['600', 'bg-brand-600'],
  ['700', 'bg-brand-700'],
  ['800', 'bg-brand-800'],
  ['900', 'bg-brand-900'],
  ['950', 'bg-brand-950'],
] as const;

const NEUTRAL = ['0', '50', '100', '200', '300', '400', '500', '600', '700', '800', '900'] as const;

const SEMANTIC = [
  ['background', 'bg-background'],
  ['body-background', 'bg-body-background'],
  ['card', 'bg-card'],
  ['sunken', 'bg-sunken'],
  ['muted', 'bg-muted'],
  ['accent', 'bg-accent'],
  ['accent-strong', 'bg-accent-strong'],
  ['primary', 'bg-primary'],
  ['primary-hover', 'bg-primary-hover'],
  ['primary-active', 'bg-primary-active'],
  ['border', 'bg-border'],
  ['border-strong', 'bg-border-strong'],
  ['input', 'bg-input'],
  ['ring', 'bg-ring'],
] as const;

const STATUS = [
  ['success-subtle', 'bg-success-subtle'],
  ['success-border', 'bg-success-border'],
  ['success', 'bg-success'],
  ['success-foreground', 'bg-success-foreground'],
  ['warning-subtle', 'bg-warning-subtle'],
  ['warning-border', 'bg-warning-border'],
  ['warning', 'bg-warning'],
  ['warning-foreground', 'bg-warning-foreground'],
  ['danger-subtle', 'bg-danger-subtle'],
  ['danger-border', 'bg-danger-border'],
  ['danger', 'bg-danger'],
  ['danger-foreground', 'bg-danger-foreground'],
] as const;

const HEADINGS = [
  ['text-heading-1', 'Heading 1 — 40 / 600'],
  ['text-heading-2', 'Heading 2 — 32 / 600'],
  ['text-heading-3', 'Heading 3 — 24 / 600'],
  ['text-heading-4', 'Heading 4 — 20 / 600'],
  ['text-heading-5', 'Heading 5 — 18 / 600'],
  ['text-heading-6', 'Heading 6 — 16 / 600'],
] as const;

const PARAGRAPHS = [
  'text-paragraph',
  'text-paragraph-medium',
  'text-paragraph-semibold',
  'text-paragraph-sm',
  'text-paragraph-sm-medium',
  'text-paragraph-sm-semibold',
  'text-paragraph-xs',
  'text-paragraph-xs-medium',
  'text-paragraph-xs-semibold',
  'text-paragraph-mini',
  'text-paragraph-mini-medium',
  'text-paragraph-mini-semibold',
] as const;

const SHADOWS = [
  ['shadow-e1', 'shadow-e1'],
  ['shadow-e2', 'shadow-e2'],
  ['shadow-e3', 'shadow-e3'],
  ['shadow-e4', 'shadow-e4'],
] as const;

const RADII = [
  ['rounded-mini', 'rounded-mini'],
  ['rounded-sm', 'rounded-sm'],
  ['rounded-md', 'rounded-md'],
  ['rounded-lg', 'rounded-lg'],
  ['rounded-xl', 'rounded-xl'],
  ['rounded-1.5xl', 'rounded-1.5xl'],
  ['rounded-2xl', 'rounded-2xl'],
  ['rounded-full', 'rounded-full'],
] as const;

function Swatch({ label, className }: { label: string; className: string }) {
  return (
    <div className="flex flex-col items-center gap-y-1">
      <div className={`size-14 border border-border rounded-lg ${className}`} />
      <span className="text-paragraph-mini text-foreground-muted">{label}</span>
    </div>
  );
}

export function Foundations() {
  return (
    <>
      <Section title="Tipografía" hint="Poppins en los títulos, Inter en todo lo demás.">
        <div className="flex flex-col w-full gap-y-2">
          {HEADINGS.map(([cls, label]) => (
            <p key={cls} className={cls}>
              {label}
            </p>
          ))}
        </div>
        <div className="flex flex-col w-full gap-y-1">
          {PARAGRAPHS.map((cls) => (
            <p key={cls} className={cls}>
              {cls} — el zorro marrón salta sobre el perro perezoso
            </p>
          ))}
        </div>
      </Section>

      <Section
        title="Color de marca"
        hint="La rampa sale del logo: brand-400 y brand-600 son los dos extremos del degradado del isotipo, brand-500 el punto de la i, brand-950 la tinta del wordmark."
      >
        <Row>
          {BRAND.map(([step, cls]) => (
            <Swatch key={step} label={`brand-${step}`} className={cls} />
          ))}
        </Row>
      </Section>

      <Section
        title="Neutrales"
        hint="Con un rastro del tono de marca, para que los grises no se lean sucios."
      >
        <Row>
          {NEUTRAL.map((step) => (
            <div key={step} className="flex flex-col items-center gap-y-1">
              <div
                className="size-14 border border-border rounded-lg"
                style={{ backgroundColor: `var(--neutral-${step})` }}
              />
              <span className="text-paragraph-mini text-foreground-muted">{step}</span>
            </div>
          ))}
        </Row>
      </Section>

      <Section title="Tokens semánticos" hint="Lo que consumen los componentes.">
        <Row>
          {SEMANTIC.map(([label, cls]) => (
            <Swatch key={label} label={label} className={cls} />
          ))}
        </Row>
      </Section>

      <Section
        title="Estados"
        hint="Cada -foreground pasa AA sobre blanco. El base es para rellenos e iconos, no para texto."
      >
        <Row>
          {STATUS.map(([label, cls]) => (
            <Swatch key={label} label={label} className={cls} />
          ))}
        </Row>
      </Section>

      <Section title="Elevación">
        <Row>
          {SHADOWS.map(([label, cls]) => (
            <div
              key={label}
              className={`grid size-24 place-items-center bg-card border border-border rounded-xl text-paragraph-xs text-foreground-muted ${cls}`}
            >
              {label}
            </div>
          ))}
        </Row>
      </Section>

      <Section title="Radios">
        <Row>
          {RADII.map(([label, cls]) => (
            <Item key={label} label={label}>
              <div className={`size-16 bg-accent-strong border border-brand-200 ${cls}`} />
            </Item>
          ))}
        </Row>
      </Section>
    </>
  );
}
