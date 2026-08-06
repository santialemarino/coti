import { z } from 'zod';

import { TEXT_FIELD_MAX_LENGTH } from '@/lib/constants/forms';
import {
  hasEveryCharacterClass,
  PASSWORD_MAX_BYTES,
  PASSWORD_MIN_LENGTH,
  passwordByteLength,
  SECRET_MAX_LENGTH,
} from '@/lib/constants/password';

/* A schema message is a catalog key the form resolves, so no copy is baked into a schema. */
export type MessageFor = (key: string, values?: Record<string, string | number>) => string;

export const rawKey: MessageFor = (key) => key;

/*
 * Two translators, because a validation message is one of two kinds: it either names this field
 * ("Ingresá el nombre del corralón.") or it says the same thing in every form of the product
 * ("Máximo 255 caracteres."). `field` is the flow's own namespace; `shared` is `common.form.errors`.
 */
export interface SchemaText {
  field: MessageFor;
  shared: MessageFor;
}

/* The default for a server-side re-validation, where the messages are never rendered. */
export const rawText: SchemaText = { field: rawKey, shared: rawKey };

/*
 * Empty and malformed are different rejections and get different messages, which is why every
 * validator below checks presence first and the format second. Text is trimmed before either, so a
 * value of nothing but spaces is refused rather than stored as a blank no screen can render.
 */
export function requiredText(t: SchemaText, requiredKey: string, max = TEXT_FIELD_MAX_LENGTH) {
  return z.string().trim().min(1, t.field(requiredKey)).max(max, t.shared('tooLong', { max }));
}

export function optionalText(t: SchemaText, max = TEXT_FIELD_MAX_LENGTH) {
  return z.string().trim().max(max, t.shared('tooLong', { max }));
}

// The cap mirrors what the API stores; a longer address is refused here rather than as a 400.
export function emailAddress(t: SchemaText, requiredKey: string) {
  return z
    .string()
    .trim()
    .min(1, t.field(requiredKey))
    .pipe(
      z
        .email(t.shared('invalidEmail'))
        .max(TEXT_FIELD_MAX_LENGTH, t.shared('tooLong', { max: TEXT_FIELD_MAX_LENGTH })),
    );
}

/* Presented, never chosen: no policy, because one added later would lock out an older password. */
export function currentSecret(t: SchemaText, requiredKey: string) {
  return z
    .string()
    .min(1, t.field(requiredKey))
    .max(SECRET_MAX_LENGTH, t.shared('passwordTooLong', { max: SECRET_MAX_LENGTH }));
}

/*
 * A password being set, against the same policy the API applies. Piped rather than chained because
 * chained checks all run, and "obligatorio" plus "mínimo 12 caracteres" on one blank field is two
 * answers to one question. The cap is counted in bytes, which is the unit bcrypt stops at.
 */
export function newPassword(t: SchemaText, requiredKey: string) {
  return z
    .string()
    .min(1, t.field(requiredKey))
    .pipe(
      z
        .string()
        .min(PASSWORD_MIN_LENGTH, t.shared('passwordTooShort', { min: PASSWORD_MIN_LENGTH }))
        .refine(
          (value) => passwordByteLength(value) <= PASSWORD_MAX_BYTES,
          t.shared('passwordTooLong', { max: PASSWORD_MAX_BYTES }),
        )
        .refine(hasEveryCharacterClass, t.shared('passwordRequirements')),
    );
}

/*
 * The repeat of a password being set. Only presence is checked here — whether the two agree is a
 * property of the pair, so it belongs to the object's refinement with `passwordMismatch`.
 */
export function passwordConfirmation(t: SchemaText, requiredKey: string) {
  return z.string().min(1, t.field(requiredKey));
}
