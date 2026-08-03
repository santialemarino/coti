// A mirror of the API's AUTH_PASSWORD_MIN_LENGTH, kept only so a form can reject a
// short password before the round trip. The API stays the authority: it applies its
// own floor on all three password paths and answers 422 when this one drifts low.
export const PASSWORD_MIN_LENGTH = 8;

// The shape a schema factory takes, so a zod message is a catalog key the form
// resolves rather than a string baked into the schema.
export type MessageFor = (key: string) => string;

export const rawKey: MessageFor = (key) => key;
