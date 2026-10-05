// The right-hand side of the queue: whatever the column's selection opened, or its landing screen.
export function RfqQueuePane({ children }: { children: React.ReactNode }) {
  return <main className="min-w-0 flex-1 px-6 py-6 lg:px-10">{children}</main>;
}
