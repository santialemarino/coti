interface SectionProps {
  title: string;
  hint?: string;
  children: React.ReactNode;
}

export function Section({ title, hint, children }: SectionProps) {
  return (
    <section className="flex flex-col w-full gap-y-4 scroll-mt-24" id={title.toLowerCase()}>
      <div className="flex flex-col gap-y-1">
        <h2 className="text-heading-4 text-foreground">{title}</h2>
        {hint ? <p className="text-paragraph-sm text-foreground-muted">{hint}</p> : null}
      </div>
      {children}
    </section>
  );
}

/* A labelled cell, so a variant is never shown without its name. */
export function Item({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex flex-col items-start gap-y-2">
      <span className="text-paragraph-mini text-foreground-subtle">{label}</span>
      {children}
    </div>
  );
}

export function Row({ children }: { children: React.ReactNode }) {
  return <div className="flex flex-wrap items-end gap-4">{children}</div>;
}
