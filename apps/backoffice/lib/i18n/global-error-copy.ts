/*
 * The global error page's copy. That page replaces the root layout, so no translation provider sits
 * above it, and importing the catalog there would ship all of it to every page. These three strings
 * are copied from `translations/es.json`, and a test fails the moment they drift from it.
 */
export const GLOBAL_ERROR_COPY = {
  appName: 'Coti',
  title: 'Ocurrió un error inesperado. Intentá de nuevo.',
  retry: 'Reintentar',
} as const;
