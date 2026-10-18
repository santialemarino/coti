// Runtime settings read from the environment, with the defaults in one place.

const DEFAULT_API_URL = 'http://localhost:8000';

export const API_URL = process.env.NEXT_PUBLIC_API_URL ?? DEFAULT_API_URL;

const configuredProxyHops = Number.parseInt(process.env.WEB_TRUSTED_PROXY_HOPS ?? '0', 10);
export const TRUSTED_PROXY_HOPS =
  Number.isFinite(configuredProxyHops) && configuredProxyHops > 0 ? configuredProxyHops : 0;
