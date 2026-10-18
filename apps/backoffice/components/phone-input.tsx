'use client';

import { PhoneInput as SharedPhoneInput, type PhoneInputProps } from '@repo/ui/components';
import { useFormatters } from '@/lib/i18n/formatters';

export function PhoneInput(props: Omit<PhoneInputProps, 'formatters'>) {
  const formatters = useFormatters();
  return <SharedPhoneInput {...props} formatters={formatters} />;
}
