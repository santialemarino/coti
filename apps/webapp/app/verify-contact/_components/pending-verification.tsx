'use client';

import { useState } from 'react';
import { MailIcon, MessageCircleIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { toast } from 'sonner';

import {
  Button,
  Callout,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  InlineLink,
  PendingButton,
} from '@repo/ui/components';
import { ContactVerification } from '@/app/profile/_components/contact-verification';
import { requestContactVerification } from '@/app/profile/actions';
import { PortalLink as Link } from '@/components/portal-link';
import { ROUTES } from '@/config/routes';
import type { WhatsAppProof } from '@/lib/api/customer-profile';
import type { CustomerContact } from '@/lib/api/customer-profile-model';

interface PendingVerificationProps {
  contacts: CustomerContact[];
  next: string;
  supplierName: string;
  /* The CUIT already had a profile at the supplier: only a contact that profile held opens it. */
  ownershipUnproved: boolean;
  /* An email link this very flow sent, so the screen can say it is on its way. */
  sentEmail?: string;
  /* The WhatsApp proof sign-up started, by its id, so its message shows at once. */
  sentProof?: WhatsAppProof & { id: string };
}

export function PendingVerification({
  contacts,
  next,
  supplierName,
  ownershipUnproved,
  sentEmail,
  sentProof,
}: PendingVerificationProps) {
  const t = useTranslations('customerVerification');
  const forQuote = next.startsWith('/quotes/');
  const emails = contacts.filter((contact) => contact.kind === 'EMAIL');
  // One proof covers WhatsApp: the number the customer writes from is the one confirmed.
  const phones = contacts.filter((contact) => contact.kind === 'WHATSAPP');

  return (
    <Card>
      <CardHeader className="text-center">
        <CardTitle>
          <h1>{t('pending.title')}</h1>
        </CardTitle>
        <CardDescription>
          {t(forQuote ? 'pending.descriptionQuote' : 'pending.description', {
            supplier: supplierName,
          })}
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-y-5">
        {ownershipUnproved ? (
          <Callout
            tone="info"
            action={
              <Button asChild variant="outline" size="sm">
                <Link href={ROUTES.profile}>{t('pending.earlierProfileAction')}</Link>
              </Button>
            }
          >
            {t('pending.earlierProfile', { supplier: supplierName })}
          </Callout>
        ) : null}
        <ul className="flex flex-col gap-y-3">
          {emails.map((contact) => (
            <li
              key={contact.value}
              className="flex flex-col p-4 gap-y-3 border border-border rounded-lg"
            >
              <ContactLabel kind="EMAIL" label={contact.value} />
              <EmailStep
                value={contact.value}
                next={next}
                initiallySent={sentEmail === contact.value}
              />
            </li>
          ))}
          <li className="flex flex-col p-4 gap-y-3 border border-border rounded-lg">
            <ContactLabel
              kind="WHATSAPP"
              label={
                phones.length > 0
                  ? phones.map((phone) => phone.value).join(' · ')
                  : t('pending.whatsappAny')
              }
            />
            <div className="flex flex-col gap-y-2">
              <p className="text-paragraph-sm text-foreground-muted">
                {t(sentProof ? 'pending.phoneSent' : 'pending.phone')}
              </p>
              <ContactVerification
                kind="WHATSAPP"
                value={phones[0]?.value ?? ''}
                verified={false}
                next={next}
                initialProof={sentProof}
              />
            </div>
          </li>
        </ul>
        <InlineLink asChild tone="muted" className="self-center">
          <Link href={ROUTES.profile}>{t('pending.profile')}</Link>
        </InlineLink>
      </CardContent>
    </Card>
  );
}

function ContactLabel({ kind, label }: { kind: 'EMAIL' | 'WHATSAPP'; label: string }) {
  const Icon = kind === 'EMAIL' ? MailIcon : MessageCircleIcon;
  return (
    <div className="flex min-w-0 items-center gap-x-3">
      <Icon className="size-4 shrink-0 text-foreground-muted" aria-hidden="true" />
      <p className="min-w-0 text-paragraph-sm-medium [overflow-wrap:anywhere]">{label}</p>
    </div>
  );
}

function EmailStep({
  value,
  next,
  initiallySent,
}: {
  value: string;
  next: string;
  initiallySent: boolean;
}) {
  const t = useTranslations('customerVerification');
  const [sending, setSending] = useState(false);
  const [sent, setSent] = useState(initiallySent);

  async function send() {
    setSending(true);
    try {
      const result = await requestContactVerification('EMAIL', value, next);
      if (result.error) {
        toast.error(result.error);
        return;
      }
      toast.success(t('pending.emailSentToast', { email: value }));
      setSent(true);
    } finally {
      setSending(false);
    }
  }

  return (
    <div className="flex flex-col gap-y-2">
      <p className="text-paragraph-sm text-foreground-muted">
        {t(sent ? 'pending.emailSent' : 'pending.email')}
      </p>
      <PendingButton
        variant="outline"
        size="sm"
        className="self-start"
        pending={sending}
        pendingLabel={t('pending.sending')}
        onClick={send}
      >
        {t(sent ? 'pending.resend' : 'pending.send')}
      </PendingButton>
    </div>
  );
}
