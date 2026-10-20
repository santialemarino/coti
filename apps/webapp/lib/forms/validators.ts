import { z } from 'zod';

export type MessageFor = (key: string, values?: Record<string, string | number>) => string;
export interface SchemaText {
  field: MessageFor;
  shared: MessageFor;
}
export const rawText: SchemaText = { field: (key) => key, shared: (key) => key };

export function requiredText(t: SchemaText, requiredKey: string, max = 255) {
  return z.string().trim().min(1, t.field(requiredKey)).max(max, t.shared('tooLong', { max }));
}

export function optionalText(t: SchemaText, max = 255) {
  return z.string().trim().max(max, t.shared('tooLong', { max }));
}

export function emailAddress(t: SchemaText, requiredKey: string) {
  return requiredText(t, requiredKey).pipe(z.email(t.shared('invalidEmail')));
}
