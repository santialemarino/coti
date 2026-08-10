import { RfqSidebar } from '@/app/(protected)/rfqs/_components/rfq-sidebar';
import { getRfqDashboard } from '@/lib/api/rfqs';

/*
 * The frame every RFQ screen shares: the message rail on the left (search plus conversations with
 * status chips) and the section's content beside it, as the dashboard design puts the rail here
 * and a future list or detail screen gets it for free.
 */
export default async function RfqsLayout({ children }: { children: React.ReactNode }) {
  const { messages } = await getRfqDashboard();

  return (
    <div className="flex flex-1 items-stretch bg-body-background">
      <RfqSidebar messages={messages} />
      <main className="min-w-0 flex-1 px-6 py-8 lg:px-10">{children}</main>
    </div>
  );
}
