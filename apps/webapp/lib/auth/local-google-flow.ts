const LOCAL_SUPPLIER = /^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.localhost$/;

export function localGoogleState(state: string, origin: string): string {
  const url = new URL(origin);
  if (
    process.env.NODE_ENV === 'production' ||
    url.protocol !== 'http:' ||
    !LOCAL_SUPPLIER.test(url.hostname)
  )
    return state;
  return `${state}.${Buffer.from(url.origin).toString('base64url')}`;
}

export function localGoogleReturnOrigin(state: string, callbackOrigin: string): string | null {
  if (process.env.NODE_ENV === 'production' || state.length > 512) return null;
  const callback = new URL(callbackOrigin);
  if (callback.protocol !== 'http:' || !['localhost', '127.0.0.1'].includes(callback.hostname))
    return null;
  const match = /^([A-Za-z0-9_-]{43})\.([A-Za-z0-9_-]+)$/.exec(state);
  if (!match?.[2]) return null;
  try {
    const encoded = Buffer.from(match[2], 'base64url').toString('utf8');
    const target = new URL(encoded);
    if (
      target.origin !== encoded ||
      target.protocol !== 'http:' ||
      target.port !== callback.port ||
      !LOCAL_SUPPLIER.test(target.hostname)
    )
      return null;
    return target.origin;
  } catch {
    return null;
  }
}

export function customerRequestOrigin(url: string, host: string | null): string {
  const origin = new URL(url);
  // The public Host also scopes customer API calls; Next's URL can contain the container address.
  if (
    process.env.NODE_ENV === 'production' &&
    host &&
    /^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}(?::443)?$/i.test(host) &&
    !host.toLowerCase().replace(/:443$/, '').endsWith('.localhost')
  ) {
    return new URL('https://' + host).origin;
  }
  if (
    process.env.NODE_ENV !== 'production' &&
    origin.protocol === 'http:' &&
    host &&
    /^(?:localhost|127\.0\.0\.1|[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.localhost)(?::[0-9]+)?$/i.test(
      host,
    )
  ) {
    return new URL('http://' + host).origin;
  }
  return origin.origin;
}
