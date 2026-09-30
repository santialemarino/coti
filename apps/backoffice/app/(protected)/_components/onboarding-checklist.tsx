'use client';

import { useState, useTransition } from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import {
  CheckIcon,
  FileSpreadsheetIcon,
  PaletteIcon,
  UsersIcon,
  type LucideIcon,
} from 'lucide-react';
import { useTranslations } from 'next-intl';
import { toast } from 'sonner';

import {
  Badge,
  Button,
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
  ConfirmDialog,
  PendingButton,
  Progress,
} from '@repo/ui/components';
import { cn } from '@repo/ui/lib';
import { resumeOnboarding } from '@/app/(onboarding)/onboarding/actions';
import { setChecklistHidden } from '@/app/(protected)/settings/onboarding/actions';
import { ROUTES } from '@/config/routes';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import type { Onboarding, OnboardingChecklistStep } from '@/lib/api/onboarding';

const ITEMS: Record<OnboardingChecklistStep, { href: string; icon: LucideIcon }> = {
  BRAND: { href: ROUTES.accountSettings, icon: PaletteIcon },
  CATALOG_UPLOAD: { href: ROUTES.catalogSettings, icon: FileSpreadsheetIcon },
  TEAM: { href: ROUTES.userSettings, icon: UsersIcon },
};

interface OnboardingChecklistProps {
  onboarding: Onboarding;
  /* The home card offers "No mostrar más"; the settings page offers bringing a hidden card back. */
  placement: 'home' | 'settings';
  className?: string;
}

/*
 * What is left of the initial setup once the wizard is closed. Every item links to the settings
 * screen that does the work, because a step done there counts exactly like one done in the wizard.
 */
export function OnboardingChecklist({
  onboarding,
  placement,
  className,
}: OnboardingChecklistProps) {
  const router = useRouter();
  const t = useTranslations('onboarding.checklist');
  const message = useApiErrorMessage('onboarding');
  const [hideOpen, setHideOpen] = useState(false);
  const [resuming, startResume] = useTransition();
  const [toggling, startToggle] = useTransition();

  const done = onboarding.checklist.filter((item) => item.done).length;
  const total = onboarding.checklist.length;
  const hidden = onboarding.checklistHiddenAt !== null;

  function resume() {
    startResume(async () => {
      const result = await resumeOnboarding();
      if (!result.ok) {
        toast.error(message(result.error));
        return;
      }
      router.push(ROUTES.onboarding);
    });
  }

  function toggle(nextHidden: boolean) {
    startToggle(async () => {
      const result = await setChecklistHidden(nextHidden);
      if (!result.ok) {
        toast.error(message(result.error));
        return;
      }
      setHideOpen(false);
      router.refresh();
    });
  }

  return (
    <Card className={className}>
      <CardHeader>
        <CardTitle className="text-heading-5">{t('title')}</CardTitle>
        <CardDescription>{t('description')}</CardDescription>
        <div className="flex items-center pt-2 gap-x-3">
          <Progress value={(done / total) * 100} label={t('progressLabel')} />
          <span className="shrink-0 text-paragraph-xs-medium text-foreground-muted tabular-nums">
            {t('progress', { done, total })}
          </span>
        </div>
      </CardHeader>

      <CardContent>
        <ul className="flex flex-col gap-y-2">
          {onboarding.checklist.map(({ step, done: itemDone }) => {
            const { href, icon: Icon } = ITEMS[step];
            return (
              <li
                key={step}
                className="flex items-center p-3 gap-x-3 border border-border rounded-lg"
              >
                <span
                  aria-hidden="true"
                  className={cn(
                    'grid size-8 shrink-0 place-items-center rounded-full',
                    itemDone
                      ? 'bg-success-subtle text-success-foreground'
                      : 'bg-muted text-foreground-muted',
                  )}
                >
                  {itemDone ? <CheckIcon className="size-4" /> : <Icon className="size-4" />}
                </span>
                <div className="flex min-w-0 flex-1 flex-col gap-y-0.5">
                  <span className="text-paragraph-sm-medium text-foreground">
                    {t(`items.${step}.title`)}
                  </span>
                  <span className="text-paragraph-xs text-foreground-muted">
                    {t(`items.${step}.description`)}
                  </span>
                </div>
                {itemDone ? (
                  <Badge tone="success" size="sm">
                    {t('done')}
                  </Badge>
                ) : (
                  <Button asChild variant="outline" size="sm">
                    <Link href={href}>{t('configure')}</Link>
                  </Button>
                )}
              </li>
            );
          })}
        </ul>
      </CardContent>

      <CardFooter className="flex-wrap justify-end gap-2 empty:hidden">
        {placement === 'home' ? (
          <Button variant="ghost" size="sm" onClick={() => setHideOpen(true)}>
            {t('hide.action')}
          </Button>
        ) : hidden ? (
          <PendingButton
            variant="ghost"
            size="sm"
            pending={toggling}
            pendingLabel={t('show.pending')}
            onClick={() => toggle(false)}
          >
            {t('show.action')}
          </PendingButton>
        ) : null}
        {onboarding.status === 'DISMISSED' ? (
          <PendingButton
            variant="outline"
            size="sm"
            pending={resuming}
            pendingLabel={t('resume.pending')}
            onClick={resume}
          >
            {t('resume.action')}
          </PendingButton>
        ) : null}
      </CardFooter>

      <ConfirmDialog
        open={hideOpen}
        onOpenChange={setHideOpen}
        entity={placement === 'home' ? onboarding : null}
        title={t('hide.title')}
        description={() => t('hide.description')}
        onConfirm={() => toggle(true)}
        pending={toggling}
        labels={{
          confirm: t('hide.confirm'),
          pending: t('hide.pending'),
          cancel: t('hide.cancel'),
        }}
      />
    </Card>
  );
}
