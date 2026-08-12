export default function RfqsLayout({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex flex-1 items-stretch bg-body-background">
      <main className="min-w-0 flex-1 px-6 pt-10 pb-8 lg:px-10">{children}</main>
    </div>
  );
}
