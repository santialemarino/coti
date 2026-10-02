import { cn } from '@repo/ui/lib';

interface AttentionDotProps {
  /* Read to assistive tech, since the dot alone is colour. */
  label: string;
  className?: string;
}

// Marks the way to something that needs the caller's attention, on every step of the way there.
export function AttentionDot({ label, className }: AttentionDotProps) {
  return (
    <>
      <span
        aria-hidden="true"
        className={cn('size-2 shrink-0 bg-warning rounded-full', className)}
      />
      <span className="sr-only">{label}</span>
    </>
  );
}
