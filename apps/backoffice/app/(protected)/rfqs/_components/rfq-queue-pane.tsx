import { cn } from '@repo/ui/lib';

interface RfqQueuePaneProps {
  children: React.ReactNode;
  className?: string;
}

// The right-hand side of the queue: whatever the column's selection opened, or its landing screen.
export function RfqQueuePane({ children, className }: RfqQueuePaneProps) {
  return <main className={cn('min-w-0 flex-1 px-6 py-6 lg:px-10', className)}>{children}</main>;
}
