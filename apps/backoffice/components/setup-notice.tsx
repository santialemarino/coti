import Link from 'next/link';
import { useTranslations } from 'next-intl';

import { Button, Callout } from '@repo/ui/components';
import { SETUP_FIX_ROUTE, type SetupIssue } from '@/lib/utils/setup-issues';

interface SetupNoticeProps {
  issue: SetupIssue;
  /* An administrator is sent to the fix; anyone else is told who can make it. */
  isAdmin: boolean;
  className?: string;
}

/*
 * Missing configuration, reported where it bites: what is missing, what that changes, and who
 * fixes it. Deliberately not dismissible — the consequence stands until the setting exists.
 */
export function SetupNotice({ issue, isAdmin, className }: SetupNoticeProps) {
  const t = useTranslations('common.setup');
  const fix = SETUP_FIX_ROUTE[issue];

  return (
    <Callout
      tone="warning"
      title={t(`${issue}.title`)}
      className={className}
      action={
        isAdmin && fix ? (
          <Button asChild variant="outline" size="sm">
            <Link href={fix}>{t(`${issue}.fix`)}</Link>
          </Button>
        ) : undefined
      }
    >
      {t(`${issue}.body`)}
      {!isAdmin && fix ? ` ${t('askAdmin')}` : null}
    </Callout>
  );
}
