// Runtime settings read from the environment, with the defaults in one place.

const DEFAULT_API_URL = 'http://localhost:8000';

export const API_URL = process.env.NEXT_PUBLIC_API_URL ?? DEFAULT_API_URL;

const configuredProxyHops = Number.parseInt(process.env.WEB_TRUSTED_PROXY_HOPS ?? '0', 10);
export const TRUSTED_PROXY_HOPS =
  Number.isFinite(configuredProxyHops) && configuredProxyHops > 0 ? configuredProxyHops : 0;

const DEFAULT_PORTAL_BASE_URL = 'http://localhost:3001';
const DEFAULT_LANDING_URL = 'http://localhost:3000';

// The shared origin every /c/<label> portal lives under; must match the API's CUSTOMER_PORTAL_BASE_URL.
export const PORTAL_BASE_URL = new URL(process.env.WEB_PORTAL_BASE_URL || DEFAULT_PORTAL_BASE_URL)
  .origin;
export const PORTAL_BASE_HOSTNAME = new URL(PORTAL_BASE_URL).hostname;

// Where the portal root and every path outside /c/<label> send a visitor.
export const LANDING_URL = process.env.WEB_LANDING_URL || DEFAULT_LANDING_URL;

export const GOOGLE_FLOW_TTL_SECONDS = Number(process.env.WEB_GOOGLE_FLOW_TTL_SECONDS || 600);
if (!Number.isSafeInteger(GOOGLE_FLOW_TTL_SECONDS) || GOOGLE_FLOW_TTL_SECONDS <= 0)
  throw new Error('WEB_GOOGLE_FLOW_TTL_SECONDS must be a positive integer');
