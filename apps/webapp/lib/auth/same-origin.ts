import type { NextRequest } from 'next/server';

export function isSameOrigin(request: NextRequest): boolean {
  const origin = request.headers.get('origin');
  if (!origin || request.headers.get('sec-fetch-site') === 'cross-site') return false;
  try {
    const parsed = new URL(origin);
    return (
      ['https:', 'http:'].includes(parsed.protocol) &&
      !parsed.username &&
      !parsed.password &&
      parsed.host === (request.headers.get('host') || request.nextUrl.host)
    );
  } catch {
    return false;
  }
}
