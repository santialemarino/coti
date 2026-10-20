'use client';

import { useState } from 'react';
import Image from 'next/image';
import { Building2Icon } from 'lucide-react';
import { AnimatePresence, motion, useReducedMotion } from 'motion/react';
import { useTranslations } from 'next-intl';

import {
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  Combobox,
  Label,
} from '@repo/ui/components';
import { EASE, MOTION } from '@repo/ui/lib';
import { PublicRfqForm } from '@/app/rfq/[channelId]/_components/public-rfq-form';
import type { PublicRfqSite } from '@/lib/api/public-rfqs';

interface PublicRfqSiteFormProps {
  site: PublicRfqSite;
  taxId?: string;
}

export function PublicRfqSiteForm({ site, taxId }: PublicRfqSiteFormProps) {
  const t = useTranslations('rfq');
  const reduced = useReducedMotion();
  const [channelId, setChannelId] = useState('');
  const [confirmed, setConfirmed] = useState(false);
  const logoUrl = site.branches[0]?.destination.logoUrl;
  const branch = site.branches.find((item) => item.channelId === channelId);

  return (
    <AnimatePresence initial={false} mode="wait">
      <motion.div
        key={confirmed ? 'request' : 'branch'}
        className="w-full"
        initial={{ opacity: 0 }}
        animate={{ opacity: 1 }}
        exit={{ opacity: 0 }}
        transition={{ duration: reduced ? 0 : MOTION.fast, ease: EASE.outSoft }}
      >
        {confirmed && branch ? (
          <PublicRfqForm
            key={channelId}
            channelId={channelId}
            destination={branch.destination}
            taxId={taxId}
          />
        ) : (
          <main className="flex flex-col min-h-dvh items-center px-4 py-8 gap-y-8 sm:px-6 sm:py-12">
            <div className="flex w-full max-w-xl flex-col gap-y-8">
              <header className="flex items-center gap-x-3">
                {logoUrl ? (
                  <Image
                    src={logoUrl}
                    alt=""
                    width={48}
                    height={48}
                    unoptimized
                    className="size-12 rounded-lg object-contain"
                  />
                ) : (
                  <span className="flex size-12 shrink-0 items-center justify-center bg-accent rounded-lg text-primary">
                    <Building2Icon className="size-6" aria-hidden="true" />
                  </span>
                )}
                <h1 className="break-words text-heading-5">{site.supplierName}</h1>
              </header>
              <Card>
                <CardHeader>
                  <CardTitle>{t('branch.title')}</CardTitle>
                </CardHeader>
                <CardContent className="flex flex-col gap-y-6">
                  <div className="flex flex-col gap-y-2">
                    <Label htmlFor="public-branch">{t('branch.label')}</Label>
                    <Combobox
                      id="public-branch"
                      value={channelId}
                      onValueChange={setChannelId}
                      placeholder={t('branch.placeholder')}
                      options={site.branches.map((item) => ({
                        value: item.channelId,
                        label: item.destination.branchName,
                      }))}
                    />
                  </div>
                  <Button disabled={!branch} onClick={() => setConfirmed(true)}>
                    {t('branch.continue')}
                  </Button>
                </CardContent>
              </Card>
            </div>
          </main>
        )}
      </motion.div>
    </AnimatePresence>
  );
}
