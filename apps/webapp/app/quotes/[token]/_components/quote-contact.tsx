import { MailIcon, MessageCircleIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';

import { InlineLink } from '@repo/ui/components';
import type { Branch } from '@/lib/api/public-quotes';

const WHATSAPP_CHAT_URL = 'https://wa.me/';

interface QuoteContactProps {
  branch: Branch;
}

// wa.me needs the full international number as digits; anything stored without the country code
// is shown but not linked, because the link would open a chat with someone else.
function whatsAppURL(number: string): string | null {
  if (!number.trim().startsWith('+')) return null;
  const digits = number.replace(/\D/g, '');
  return digits ? WHATSAPP_CHAT_URL + digits : null;
}

export function QuoteContact({ branch }: QuoteContactProps) {
  const t = useTranslations('quote.contact');
  const whatsapp = branch.whatsapp?.trim();
  const email = branch.email?.trim();

  if (!whatsapp && !email) return null;
  const chatURL = whatsapp ? whatsAppURL(whatsapp) : null;
  return (
    <footer className="flex flex-col pt-2 gap-y-2 sm:flex-row sm:items-center sm:gap-x-6">
      <p className="text-paragraph-sm text-foreground-muted">{t('title')}</p>
      {whatsapp ? (
        <p className="flex items-center gap-x-2 text-paragraph-sm">
          <MessageCircleIcon aria-hidden className="size-4 text-foreground-muted" />
          {chatURL ? (
            <InlineLink asChild>
              <a
                href={chatURL}
                target="_blank"
                rel="noopener noreferrer"
                aria-label={t('whatsappLabel', { number: whatsapp })}
              >
                {whatsapp}
              </a>
            </InlineLink>
          ) : (
            <span>{whatsapp}</span>
          )}
        </p>
      ) : null}
      {email ? (
        <p className="flex items-center gap-x-2 text-paragraph-sm">
          <MailIcon aria-hidden className="size-4 text-foreground-muted" />
          <InlineLink asChild>
            <a href={`mailto:${email}`} aria-label={t('emailLabel', { email })}>
              {email}
            </a>
          </InlineLink>
        </p>
      ) : null}
    </footer>
  );
}
