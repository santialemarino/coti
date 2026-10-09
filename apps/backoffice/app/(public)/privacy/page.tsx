import { LegalPage } from '@/app/(public)/_components/legal-page';
import { ROUTES } from '@/config/routes';
import { generatePublicPageMetadata } from '@/lib/utils/page';

// Moves with every change to the text, which the page then states.
const UPDATED_AT = '2026-10-09';

export const generateMetadata = () => generatePublicPageMetadata('privacy', ROUTES.privacy);

export default function PrivacyPage() {
  return <LegalPage namespace="legal.privacy" updatedAt={UPDATED_AT} />;
}
