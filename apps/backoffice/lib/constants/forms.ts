// The cap the API puts on every free-text field it stores, mirrored so a form rejects an
// over-long value inline instead of turning it into a 400 with nothing to point at.
export const TEXT_FIELD_MAX_LENGTH = 255;
