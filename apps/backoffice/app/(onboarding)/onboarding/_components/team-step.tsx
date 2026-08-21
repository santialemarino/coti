'use client';

import { useState } from 'react';
import { useRouter } from 'next/navigation';
import { PlusIcon, UsersIcon } from 'lucide-react';
import { AnimatePresence, motion, useReducedMotion } from 'motion/react';
import { useTranslations } from 'next-intl';
import { toast } from 'sonner';

import { Badge, Button, Callout, Card } from '@repo/ui/components';
import { MOTION } from '@repo/ui/lib';
import type { OnboardingActionResult } from '@/app/(onboarding)/onboarding/actions';
import type { UserValues } from '@/app/(protected)/settings/users/form-schema';
import { UserFormDialog } from '@/components/user-form-dialog';
import type { Branch } from '@/lib/api/branches';
import type { AccountUser } from '@/lib/api/users';

const NO_BRANCHES: Branch[] = [];

interface TeamStepProps {
  branches: Branch[];
  currentUserId: string;
  users: AccountUser[];
  onCreate: (values: UserValues) => Promise<OnboardingActionResult>;
}

export function TeamStep({ branches, currentUserId, users, onCreate }: TeamStepProps) {
  const router = useRouter();
  const t = useTranslations('onboarding.team');
  const tCommon = useTranslations('common');
  const reduced = useReducedMotion();
  const [open, setOpen] = useState(false);
  const teammates = users.filter((user) => user.id !== currentUserId && user.isActive);

  async function onSubmit(values: UserValues): Promise<OnboardingActionResult> {
    const result = await onCreate(values);
    if (result.ok) {
      toast.success(t('created', { name: values.name }));
      setOpen(false);
      router.refresh();
    }
    return result;
  }

  return (
    <div className="flex flex-col gap-y-6">
      <Callout tone="info">{t('passwordNotice')}</Callout>

      <AnimatePresence mode="wait" initial={false}>
        <motion.div
          key={teammates.length > 0 ? 'members' : 'empty'}
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          exit={{ opacity: 0 }}
          transition={{ duration: reduced ? 0 : MOTION.default }}
        >
          {teammates.length > 0 ? (
            <div className="grid gap-3 md:grid-cols-2">
              {teammates.map((user) => (
                <Card
                  key={user.id}
                  className="flex-row items-center justify-between p-4 gap-y-0 gap-x-3 rounded-lg shadow-e1"
                >
                  <div className="flex min-w-0 items-center gap-x-3">
                    <span className="flex size-9 shrink-0 items-center justify-center bg-accent rounded-full text-accent-foreground">
                      <UsersIcon aria-hidden="true" className="size-4" />
                    </span>
                    <div className="min-w-0">
                      <p className="truncate text-paragraph-sm-medium">{user.name}</p>
                      <p className="truncate text-paragraph-xs text-foreground-muted">
                        {user.email}
                      </p>
                    </div>
                  </div>
                  <Badge tone="neutral">{tCommon(`roles.${user.role}`)}</Badge>
                </Card>
              ))}
            </div>
          ) : (
            <Card className="items-center justify-center p-8 gap-y-3 bg-muted text-center">
              <UsersIcon aria-hidden="true" className="size-8 text-foreground-subtle" />
              <p className="text-paragraph-medium">{t('empty.title')}</p>
              <p className="max-w-md text-paragraph-sm text-foreground-muted">
                {t('empty.description')}
              </p>
            </Card>
          )}
        </motion.div>
      </AnimatePresence>

      <Button
        type="button"
        variant="outline"
        className="w-full sm:self-start sm:w-auto"
        onClick={() => setOpen(true)}
      >
        <PlusIcon aria-hidden="true" />
        {t('add')}
      </Button>

      <UserFormDialog
        open={open}
        onOpenChange={setOpen}
        mode="create"
        user={null}
        assigned={NO_BRANCHES}
        branches={branches}
        isSelf={false}
        onSubmit={onSubmit}
      />
    </div>
  );
}
