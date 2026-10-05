// The right-hand side of the queue: whatever the column's selection opened, or its landing screen.
export function RfqQueuePane({ children }: { children: React.ReactNode }) {
  // Below lg an order is a page of its own, so it starts where every page's title does.
  return <main className="min-w-0 flex-1 px-6 py-10 lg:px-10 lg:py-6">{children}</main>;
}
