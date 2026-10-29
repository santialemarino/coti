'use client';

import { useState } from 'react';
import Link from 'next/link';
import { AnimatePresence, motion, useReducedMotion } from 'motion/react';
import { useTranslations } from 'next-intl';

import {
  AnimatedHeight,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  InlineLink,
} from '@repo/ui/components';
import { MOTION } from '@repo/ui/lib';
import { RecoveryCodeStep } from '@/app/forgot-password/_components/recovery-code-step';
import { RecoveryContactStep } from '@/app/forgot-password/_components/recovery-contact-step';
import { RecoveryIdentifyStep } from '@/app/forgot-password/_components/recovery-identify-step';
import type { RecoveryContact } from '@/app/forgot-password/actions';
import { ROUTES } from '@/config/routes';

interface AccountRecoveryProps {
  minimum: number;
  next: string;
}

type Stage =
  | { name: 'identify'; returned?: boolean }
  | { name: 'contact'; taxId: string; contacts: RecoveryContact[] }
  | {
      name: 'code';
      taxId: string;
      contacts: RecoveryContact[];
      challengeId: string;
      contact: RecoveryContact;
    };

/*
 * CUIT/CUIL → a verified contact, shown masked → the code it received and a new password. Each
 * stage owns everything it shows, so a stage change crossfades the whole body while the card travels
 * to the new stage's height.
 */
export function AccountRecovery({ minimum, next }: AccountRecoveryProps) {
  const t = useTranslations('customerRecovery');
  const reduced = useReducedMotion();
  const [stage, setStage] = useState<Stage>({ name: 'identify' });

  return (
    <AnimatedHeight trigger={stage.name}>
      <Card>
        <CardHeader className="text-center">
          <CardTitle>
            <h1>{t('title')}</h1>
          </CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-y-5">
          <AnimatePresence mode="wait" initial={false}>
            <motion.div
              key={stage.name}
              initial={{ opacity: 0 }}
              animate={{ opacity: 1 }}
              exit={{ opacity: 0 }}
              transition={{ duration: reduced ? 0 : MOTION.default }}
            >
              {stage.name === 'identify' ? (
                <RecoveryIdentifyStep
                  autoFocus={stage.returned}
                  onFound={(taxId, contacts) => setStage({ name: 'contact', taxId, contacts })}
                />
              ) : stage.name === 'contact' ? (
                <RecoveryContactStep
                  taxId={stage.taxId}
                  contacts={stage.contacts}
                  onSent={(challengeId, contact) =>
                    setStage({ ...stage, name: 'code', challengeId, contact })
                  }
                  onBack={() => setStage({ name: 'identify', returned: true })}
                />
              ) : (
                <RecoveryCodeStep
                  challengeId={stage.challengeId}
                  contact={stage.contact}
                  minimum={minimum}
                  next={next}
                  onResend={() =>
                    setStage({ name: 'contact', taxId: stage.taxId, contacts: stage.contacts })
                  }
                />
              )}
            </motion.div>
          </AnimatePresence>
          <InlineLink asChild tone="muted" className="self-center">
            <Link href={`${ROUTES.login}?next=${encodeURIComponent(next)}`}>
              {t('backToLogin')}
            </Link>
          </InlineLink>
        </CardContent>
      </Card>
    </AnimatedHeight>
  );
}
