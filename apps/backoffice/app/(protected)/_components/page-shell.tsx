import { cn } from '@repo/ui/lib';

interface PageShellProps {
  children: React.ReactNode;
  className?: string;
}

// The frame a top-level protected page sits in, so every section keeps one gutter and one rhythm.
export function PageShell({ children, className }: PageShellProps) {
  return (
    <main className={cn('flex flex-col min-w-0 flex-1 px-6 py-10 gap-y-8 lg:px-10', className)}>
      {children}
    </main>
  );
}
