import { RfqQueueEmpty } from '@/app/(protected)/_components/rfq-queue-empty';
import { RfqQueueProvider } from '@/app/(protected)/rfqs/_components/rfq-queue-provider';
import { RfqSplitView } from '@/app/(protected)/rfqs/_components/rfq-split-view';
import { getOnboarding } from '@/lib/api/onboarding';
import { getSession } from '@/lib/auth/session';
import { ADMIN_ROLE } from '@/lib/constants/auth';
import { showsChecklistOnHome } from '@/lib/utils/onboarding-checklist';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('home');

/*
 * Home is the queue with nothing selected. Triaging orders is what the day is, so landing anywhere
 * else costs a click before the work starts.
 */
export default async function HomePage() {
  const session = await getSession();
  const onboarding = session?.role === ADMIN_ROLE ? await getOnboarding() : null;

  return (
    <RfqQueueProvider>
      <RfqSplitView activeRfqId={null}>
        <RfqQueueEmpty
          onboarding={onboarding && showsChecklistOnHome(onboarding) ? onboarding : null}
        />
      </RfqSplitView>
    </RfqQueueProvider>
  );
}
