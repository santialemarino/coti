import Link from 'next/link';
import { redirect } from 'next/navigation';
import { CircleXIcon } from 'lucide-react';
import { getTranslations } from 'next-intl/server';

import { Card, InlineLink, StatusScreen } from '@repo/ui/components';
import { SignupForm } from '@/app/(auth)/signup/_components/signup-form';
import { ROUTES } from '@/config/routes';
import { signupInviteRefusal } from '@/lib/api/signup-invites';
import { SIGNUP_ENABLED } from '@/lib/config';
import { apiErrorMessage } from '@/lib/i18n/api-error';
import { generatePageMetadata } from '@/lib/utils/page';

// The link cmd/invite prints, so its shape is a contract: WEB_BACKOFFICE_URL plus /signup?invite=…
const INVITE_PARAM = 'invite';

export const generateMetadata = () => generatePageMetadata('signup');

interface SignupPageProps {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}

export default async function SignupPage({ searchParams }: SignupPageProps) {
  const t = await getTranslations('auth.signup.invite');
  const tRoot = await getTranslations();
  const params = await searchParams;
  const invite = typeof params[INVITE_PARAM] === 'string' ? params[INVITE_PARAM].trim() : '';

  // Closed, the way in is the plans and the request for Coti, which is where this sends them.
  if (!invite) {
    if (!SIGNUP_ENABLED) redirect(ROUTES.landingSection('probar'));
    return <SignupForm />;
  }

  // Checked before the wizard, so a dead link is not found out three steps later.
  const refusal = await signupInviteRefusal(invite);
  if (refusal) {
    const used = refusal === 'SIGNUP_INVITE_USED';
    return (
      <Card>
        <StatusScreen
          icon={CircleXIcon}
          tone="danger"
          title={t('title')}
          description={apiErrorMessage(tRoot, undefined, refusal)}
        >
          <InlineLink asChild>
            <Link href={used ? ROUTES.login : ROUTES.landingSection('probar')}>
              {used ? t('login') : t('requestAnother')}
            </Link>
          </InlineLink>
        </StatusScreen>
      </Card>
    );
  }

  return <SignupForm invite={invite} />;
}
