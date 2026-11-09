import { afterEach, expect, it, vi } from 'vitest';

import {
  customerRequestOrigin,
  localGoogleReturnOrigin,
  localGoogleState,
} from '@/lib/auth/local-google-flow';

const nonce = 'n'.repeat(43);
afterEach(() => vi.unstubAllEnvs());

it('uses the public HTTPS host behind the production reverse proxy', () => {
  vi.stubEnv('NODE_ENV', 'production');
  expect(
    customerRequestOrigin('http://localhost:3001/callback', 'coti-portal-8zcrm.ondigitalocean.app'),
  ).toBe('https://coti-portal-8zcrm.ondigitalocean.app');
});

it.each([
  'evil.com/path',
  'user@evil.com',
  'evil.com,other.com',
  'evil.com#fragment',
  'evil.com:3001',
])('does not use a malformed public host %s', (host) => {
  vi.stubEnv('NODE_ENV', 'production');
  expect(customerRequestOrigin('https://supplier.test/callback', host)).toBe(
    'https://supplier.test',
  );
});

it('routes the development callback back to its starting supplier and port', () => {
  const state = localGoogleState(nonce, 'http://sanmartin.localhost:3001');
  expect(localGoogleReturnOrigin(state, 'http://localhost:3001')).toBe(
    'http://sanmartin.localhost:3001',
  );
  expect(localGoogleReturnOrigin(state, 'http://localhost:3118')).toBeNull();
  expect(localGoogleReturnOrigin(state, 'http://otro.localhost:3001')).toBeNull();
  expect(localGoogleState(nonce, 'https://supplier.example.com')).toBe(nonce);
});

it.each([
  'https://sanmartin.localhost:3001',
  'http://sanmartin.localhost.evil.com:3001',
  'http://evil.com:3001',
  'http://sanmartin.localhost:3001/path',
  'http://user:password@sanmartin.localhost:3001',
  'http://sanmartin.localhost:3001#fragment',
])('rejects a callback relay to %s', (origin) => {
  const state = `${nonce}.${Buffer.from(origin).toString('base64url')}`;
  expect(localGoogleReturnOrigin(state, 'http://localhost:3001')).toBeNull();
});

it('disables local relaying in production', () => {
  const state = localGoogleState(nonce, 'http://sanmartin.localhost:3001');
  vi.stubEnv('NODE_ENV', 'production');
  expect(localGoogleReturnOrigin(state, 'http://localhost:3001')).toBeNull();
  expect(localGoogleState(nonce, 'http://sanmartin.localhost:3001')).toBe(nonce);
});

it('retains the supplier host when the development server normalizes its request URL', () => {
  expect(customerRequestOrigin('http://localhost:3001/callback', 'sanmartin.localhost:3001')).toBe(
    'http://sanmartin.localhost:3001',
  );
  expect(customerRequestOrigin('http://localhost:3001/callback', 'evil.com')).toBe(
    'http://localhost:3001',
  );
  vi.stubEnv('NODE_ENV', 'production');
  expect(
    customerRequestOrigin('https://supplier.example.com/callback', 'sanmartin.localhost:3001'),
  ).toBe('https://supplier.example.com');
});
