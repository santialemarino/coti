import { afterEach, expect, it, vi } from 'vitest';

import { getCustomerHostname } from '@/lib/auth/customer-session';

const request = vi.hoisted(() => ({ host: 'sanmartin.localhost:3001' }));
vi.mock('next/headers', () => ({
  headers: async () => new Headers({ host: request.host, 'x-forwarded-host': 'other.localhost' }),
}));
vi.mock('@/lib/config', () => ({
  API_URL: 'http://localhost:8000',
  PUBLIC_SITE_HOSTNAME: 'fixed.test',
}));
afterEach(() => vi.unstubAllEnvs());

it('resolves each supplier from its own host even when an override exists', async () => {
  request.host = 'SanMartin.localhost:3001';
  expect(await getCustomerHostname()).toBe('sanmartin.localhost');
  request.host = 'otro.localhost:3001';
  expect(await getCustomerHostname()).toBe('otro.localhost');
});

it('restricts the optional single-supplier override to development loopback', async () => {
  request.host = 'localhost:3001';
  expect(await getCustomerHostname()).toBe('fixed.test');
  vi.stubEnv('NODE_ENV', 'production');
  expect(await getCustomerHostname()).toBe('localhost');
  request.host = 'supplier.example.com';
  expect(await getCustomerHostname()).toBe('supplier.example.com');
});
