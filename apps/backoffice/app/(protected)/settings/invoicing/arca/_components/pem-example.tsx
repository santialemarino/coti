import { useTranslations } from 'next-intl';

// The delimiter lines are what has to be copied exactly, so they are shown as they are.
const PEM_BEGIN = '-----BEGIN CERTIFICATE-----';
const PEM_END = '-----END CERTIFICATE-----';

interface PemExampleProps {
  // The one line under the block saying what to copy.
  instruction: string;
}

// What a certificate looks like: its real first and last lines, and a description of the middle.
export function PemExample({ instruction }: PemExampleProps) {
  const t = useTranslations('arca.pem');

  return (
    <figure className="flex flex-col gap-y-2">
      <div className="flex flex-col w-fit max-w-full px-3 py-2 gap-y-0.5 bg-sunken border border-border rounded-lg overflow-x-auto">
        <code className="text-paragraph-xs font-mono text-foreground whitespace-nowrap">
          {PEM_BEGIN}
        </code>
        <span className="text-paragraph-xs text-foreground-subtle italic">{t('middle')}</span>
        <code className="text-paragraph-xs font-mono text-foreground whitespace-nowrap">
          {PEM_END}
        </code>
      </div>
      <figcaption className="text-paragraph-xs text-foreground-muted">{instruction}</figcaption>
    </figure>
  );
}
