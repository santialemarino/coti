// The role that reaches the whole account. The API is still the authority — it answers 403
// on every admin route — so this only decides what the interface offers.
export const ADMIN_ROLE = 'ADMIN';

// A mirror of the API's AUTH_PASSWORD_MIN_LENGTH, kept only so a form can reject a
// short password before the round trip. The API stays the authority: it applies its
// own floor on all three password paths and answers 422 when this one drifts low.
export const PASSWORD_MIN_LENGTH = 8;

// bcrypt hashes the first 72 bytes and ignores the rest, so the API rejects anything longer
// than that instead of silently accepting a password whose tail never mattered.
export const PASSWORD_MAX_LENGTH = 72;

// The shape a schema factory takes, so a zod message is a catalog key the form
// resolves rather than a string baked into the schema.
export type MessageFor = (key: string) => string;

export const rawKey: MessageFor = (key) => key;
