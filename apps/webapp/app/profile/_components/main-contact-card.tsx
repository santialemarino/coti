'use client';

import { useId, useState, useTransition } from 'react';
import { useRouter } from 'next/navigation';
import { useTranslations } from 'next-intl';
import { toast } from 'sonner';

import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  Label,
  RadioGroup,
  RadioGroupItem,
} from '@repo/ui/components';
import { chooseMainContact } from '@/app/profile/actions';
import type { CustomerContact } from '@/lib/api/customer-profile-model';

interface MainContactCardProps {
  contacts: CustomerContact[];
}

const keyOf = (contact: CustomerContact) => `${contact.kind}:${contact.value}`;

export function MainContactCard({ contacts }: MainContactCardProps) {
  const router = useRouter();
  const t = useTranslations('customerProfile.main');
  const id = useId();
  const verified = contacts.filter((contact) => contact.verified);
  const saved = verified.find((contact) => contact.main);
  const [chosen, setChosen] = useState(saved ? keyOf(saved) : '');
  const [pending, startTransition] = useTransition();

  function choose(key: string) {
    const contact = verified.find((candidate) => keyOf(candidate) === key);
    if (!contact) return;
    const previous = chosen;
    setChosen(key);
    startTransition(async () => {
      const result = await chooseMainContact(contact.kind, contact.value);
      if (result.error) {
        setChosen(previous);
        toast.error(result.error);
        return;
      }
      toast.success(t('saved', { contact: contact.value }));
      router.refresh();
    });
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>
          <h2 className="text-heading-5">{t('title')}</h2>
        </CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-y-4">
        <p className="text-paragraph-sm text-foreground-muted">{t('description')}</p>
        {verified.length === 0 ? (
          <p className="text-paragraph-sm text-foreground-muted">{t('empty')}</p>
        ) : (
          <RadioGroup
            aria-label={t('title')}
            className="gap-y-3"
            value={chosen}
            onValueChange={choose}
            disabled={pending}
          >
            {verified.map((contact) => (
              <div key={keyOf(contact)} className="flex items-center gap-x-2.5">
                <RadioGroupItem id={`${id}-${keyOf(contact)}`} value={keyOf(contact)} />
                <Label htmlFor={`${id}-${keyOf(contact)}`} className="cursor-pointer wrap-anywhere">
                  {t(contact.kind === 'WHATSAPP' ? 'whatsapp' : 'email', {
                    contact: contact.value,
                  })}
                </Label>
              </div>
            ))}
          </RadioGroup>
        )}
      </CardContent>
    </Card>
  );
}
