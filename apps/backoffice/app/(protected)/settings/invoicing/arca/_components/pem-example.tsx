// What a PEM block's opening line follows, so the example shows the exact delimiters to copy.
export type PemLabel = 'CERTIFICATE' | 'CERTIFICATE REQUEST';

// A block's first line of content, cut short: the shape is what matters, not the bytes.
const PEM_BODY_SAMPLE = 'MIIDazCCAlOgAwIBAgIU…';

// The block as it looks, middle elided — first dash to last dash is what gets copied.
export function pemExample(label: PemLabel): string {
  return [`-----BEGIN ${label}-----`, PEM_BODY_SAMPLE, '…', `-----END ${label}-----`].join('\n');
}

interface PemExampleProps {
  label: PemLabel;
  // One line saying what to copy, read beside the example.
  caption: string;
}

export function PemExample({ label, caption }: PemExampleProps) {
  return (
    <figure className="flex flex-col gap-y-2">
      <pre
        aria-hidden="true"
        className="w-fit max-w-full px-3 py-2 bg-sunken border border-border rounded-lg text-paragraph-xs font-mono text-foreground-muted overflow-x-auto"
      >
        {pemExample(label)}
      </pre>
      <figcaption className="text-paragraph-xs text-foreground-muted">{caption}</figcaption>
    </figure>
  );
}
