import { isValidElement, type ReactElement, type ReactNode } from 'react';
import { beforeEach, expect, it, vi } from 'vitest';

import { ContactEmailConfirmation } from '@/app/verify-contact/_components/contact-email-confirmation';
import { PendingVerification } from '@/app/verify-contact/_components/pending-verification';
import Page from '@/app/verify-contact/page';
import { getContactVerification, getCustomerProfile } from '@/lib/api/customer-profile';
import type { CustomerProfile } from '@/lib/api/customer-profile-model';
import { requireCustomer } from '@/lib/auth/customer-flow';

vi.mock('@/app/verify-contact/_components/contact-email-confirmation', () => ({
  ContactEmailConfirmation: () => null,
}));
vi.mock('@/app/verify-contact/_components/pending-verification', () => ({
  PendingVerification: () => null,
}));
vi.mock('@/components/customer-auth-shell', () => ({
  CustomerAuthShell: ({ children }: { children: ReactNode }) => children,
}));
vi.mock('@/lib/api/customer-auth-options', () => ({
  customerPageMetadata: vi.fn(),
  getCustomerAuthOptions: async () => ({ supplierName: 'Corralón San Martín' }),
}));
vi.mock('@/lib/api/customer-profile', () => ({
  getContactVerification: vi.fn(),
  getCustomerProfile: vi.fn(),
}));
vi.mock('@/lib/auth/customer-flow', async (original) => ({
  ...(await original<typeof import('@/lib/auth/customer-flow')>()),
  requireCustomer: vi.fn(),
}));
vi.mock('@/lib/portal/server', () => ({
  customerRedirect: (path: string) => {
    throw new Error('redirect:' + path);
  },
}));

const unverified: CustomerProfile = {
  clientId: 'profile',
  taxId: '20123456786',
  name: 'Obra Nueva',
  ownershipUnproved: false,
  contacts: [
    { kind: 'EMAIL', value: 'obra@cliente.test', primary: true },
    { kind: 'WHATSAPP', value: '+5491155550000', primary: false },
  ],
};

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getCustomerProfile).mockResolvedValue(unverified);
});

function rendered(page: ReactNode, component: unknown): Record<string, unknown> {
  const child = (page as ReactElement<{ children: ReactNode }>).props.children;
  const element = [child].flat().find((node) => isValidElement(node) && node.type === component);
  if (!isValidElement(element)) throw new Error('component not rendered');
  return element.props as Record<string, unknown>;
}

it.each([
  [null, 'invalid'],
  [{ kind: 'EMAIL', value: 'a@b.test', status: 'EXPIRED', next: null }, 'invalid'],
  [{ kind: 'WHATSAPP', value: '+5491155550000', status: 'PENDING', next: null }, 'invalid'],
  [{ kind: 'EMAIL', value: 'a@b.test', status: 'VERIFIED', next: '/quotes/tok-1' }, 'verified'],
  [{ kind: 'EMAIL', value: 'a@b.test', status: 'PENDING', next: '/quotes/tok-1' }, 'pending'],
] as const)('maps the emailed proof %j to the %s screen', async (verification, state) => {
  vi.mocked(getContactVerification).mockResolvedValue(verification);
  const page = await Page({
    searchParams: Promise.resolve({ challenge: 'c-1', token: 'secret' }),
  });
  const props = rendered(page, ContactEmailConfirmation);
  expect(props.initialState).toBe(state);
  expect(props.next).toBe(verification?.next ?? undefined);
});

it('asks for a sign-in that returns to the same link', async () => {
  vi.mocked(getContactVerification).mockResolvedValue(null);
  await Page({ searchParams: Promise.resolve({ challenge: 'c-1', token: 'secret' }) });
  expect(requireCustomer).toHaveBeenCalledWith('/verify-contact?challenge=c-1&token=secret');
});

it('continues to the quote once a contact is proved', async () => {
  vi.mocked(getCustomerProfile).mockResolvedValue({
    ...unverified,
    contacts: [{ kind: 'EMAIL', value: 'obra@cliente.test', primary: true, verified: true }],
  });
  await expect(Page({ searchParams: Promise.resolve({ next: '/quotes/tok-1' }) })).rejects.toThrow(
    'redirect:/quotes/tok-1',
  );
});

it('keeps an earlier profile owner on the guidance even with a verified contact', async () => {
  vi.mocked(getCustomerProfile).mockResolvedValue({
    ...unverified,
    ownershipUnproved: true,
    contacts: [{ kind: 'EMAIL', value: 'obra@cliente.test', primary: true, verified: true }],
  });
  const page = await Page({ searchParams: Promise.resolve({ next: '/quotes/tok-1' }) });
  const props = rendered(page, PendingVerification);
  expect(props.ownershipUnproved).toBe(true);
  expect(props.contacts).toEqual([]);
});

it('says only the proofs sign-up really sent are on their way', async () => {
  vi.mocked(getContactVerification).mockImplementation(async (id: string) =>
    id === 'e-1'
      ? { kind: 'EMAIL', value: 'obra@cliente.test', status: 'PENDING', next: '/quotes/tok-1' }
      : { kind: 'WHATSAPP', value: '+5491155550000', status: 'PENDING', next: '/quotes/tok-1' },
  );
  const page = await Page({
    searchParams: Promise.resolve({ next: '/quotes/tok-1', email: 'e-1', phone: 'p-1' }),
  });
  const props = rendered(page, PendingVerification);
  expect(props.sentEmail).toBe('obra@cliente.test');
  expect(props.sentCode).toEqual({ value: '+5491155550000', challenge: 'p-1' });

  vi.mocked(getContactVerification).mockResolvedValue(null);
  const plain = await Page({ searchParams: Promise.resolve({ next: '/quotes/tok-1' }) });
  const none = rendered(plain, PendingVerification);
  expect(none.sentEmail).toBeUndefined();
  expect(none.sentCode).toBeUndefined();
});
