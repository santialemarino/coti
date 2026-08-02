// A mirror of the API's AUTH_PASSWORD_MIN_LENGTH, kept only so a form can reject a
// short password before the round trip. The API stays the authority: it applies its
// own floor on all three password paths and answers 422 when this one drifts low.
export const PASSWORD_MIN_LENGTH = 8;
