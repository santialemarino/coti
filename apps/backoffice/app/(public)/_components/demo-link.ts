import { LANDING_DEMO_EMAIL, LANDING_DEMO_WHATSAPP } from '@/lib/config';

const WHATSAPP_CHAT_URL = 'https://wa.me/';

interface DemoCopy {
  message: string;
  subject: string;
}

// The way «Pedí una demo» opens a conversation with Coti, or null while none is configured.
// WhatsApp goes first because it is where a corralón already talks to its customers.
export function demoContactHref(
  copy: DemoCopy,
  whatsapp = LANDING_DEMO_WHATSAPP,
  email = LANDING_DEMO_EMAIL,
): string | null {
  const digits = whatsapp?.replace(/\D/g, '');
  if (digits) return `${WHATSAPP_CHAT_URL}${digits}?text=${encodeURIComponent(copy.message)}`;
  if (email) return `mailto:${email}?subject=${encodeURIComponent(copy.subject)}`;
  return null;
}
