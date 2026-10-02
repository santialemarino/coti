import { TIME_ZONE } from '@/lib/i18n/locales';
import messages from '@/translations/es.json';

type TranslationValues = Record<string, string | number>;

/*
 * The public-quote components translate through next-intl's request-bound functions, which throw
 * without a request context. This module stands in for next-intl/server in tests: the real es.json
 * and the static zone, so what renders is exactly what a real request would produce.
 */
function resolveMessage(namespace: string, key: string): string {
  let node: unknown = (messages as Record<string, unknown>)[namespace];
  for (const part of key.split('.')) {
    if (node && typeof node === 'object' && part in node) {
      node = (node as Record<string, unknown>)[part];
    } else {
      return key;
    }
  }
  return typeof node === 'string' ? node : key;
}

export async function getTranslations(namespace: string) {
  return (key: string, values: TranslationValues = {}) => {
    const template = resolveMessage(namespace, key);
    return Object.entries(values).reduce(
      (translated, [name, value]) => translated.replaceAll(`{${name}}`, String(value)),
      template,
    );
  };
}

export async function getLocale() {
  return 'es';
}

export async function getTimeZone() {
  return TIME_ZONE;
}
