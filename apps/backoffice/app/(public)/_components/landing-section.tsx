import { cn } from '@repo/ui/lib';
import { Reveal } from '@/app/(public)/_components/reveal';

interface LandingSectionProps {
  id: string;
  eyebrow: string;
  title: string;
  description?: string;
  children: React.ReactNode;
  className?: string;
}

// One band of the landing: the same width, rhythm and heading shape for every section.
export function LandingSection({
  id,
  eyebrow,
  title,
  description,
  children,
  className,
}: LandingSectionProps) {
  const headingId = `${id}-title`;

  return (
    <section
      id={id}
      aria-labelledby={headingId}
      className={cn('py-16 scroll-mt-16 lg:py-24', className)}
    >
      <Reveal className="flex flex-col w-full max-w-6xl mx-auto px-4 gap-y-10 sm:px-6 lg:gap-y-12">
        <div className="flex flex-col max-w-2xl gap-y-3">
          <p className="text-paragraph-sm-semibold text-primary">{eyebrow}</p>
          <h2 id={headingId} className="text-heading-3 text-foreground lg:text-heading-2">
            {title}
          </h2>
          {description ? (
            <p className="text-paragraph text-foreground-muted">{description}</p>
          ) : null}
        </div>
        {children}
      </Reveal>
    </section>
  );
}
